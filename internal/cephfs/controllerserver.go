/*
Copyright 2018 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cephfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/cephfs/core"
	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ControllerServer struct of CEPH CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	VolumeLocks *util.VolumeLocks

	// A map storing all snapshots with ongoing operations so that additional operations
	// for that same snapshot (as defined by SnapshotID/snapshot name) return an Aborted error
	SnapshotLocks *util.VolumeLocks

	// A map storing all volumes/snapshots with ongoing operations.
	OperationLocks *util.OperationLock
}

// createBackingVolume creates the backing subvolume and on any error cleans up any created entities.
func (cs *ControllerServer) createBackingVolume(
	ctx context.Context,
	volOptions,
	parentVolOpt *core.VolumeOptions,

	vID,
	pvID *core.VolumeIdentifier,
	sID *core.SnapshotIdentifier) error {
	var err error
	if sID != nil {
		if err = cs.OperationLocks.GetRestoreLock(sID.SnapshotID); err != nil {
			log.ErrorLog(ctx, err.Error())

			return status.Error(codes.Aborted, err.Error())
		}
		defer cs.OperationLocks.ReleaseRestoreLock(sID.SnapshotID)

		err = core.CreateCloneFromSnapshot(ctx, parentVolOpt, volOptions, vID, sID)
		if err != nil {
			log.ErrorLog(ctx, "failed to create clone from snapshot %s: %v", sID.FsSnapshotName, err)

			return err
		}

		return err
	}
	if parentVolOpt != nil {
		if err = cs.OperationLocks.GetCloneLock(pvID.VolumeID); err != nil {
			log.ErrorLog(ctx, err.Error())

			return status.Error(codes.Aborted, err.Error())
		}
		defer cs.OperationLocks.ReleaseCloneLock(pvID.VolumeID)
		err = core.CreateCloneFromSubvolume(
			ctx,
			fsutil.VolumeID(pvID.FsSubvolName),
			fsutil.VolumeID(vID.FsSubvolName),
			volOptions,
			parentVolOpt)
		if err != nil {
			log.ErrorLog(ctx, "failed to create clone from subvolume %s: %v", fsutil.VolumeID(pvID.FsSubvolName), err)

			return err
		}

		return nil
	}

	if err = core.CreateVolume(ctx, volOptions, fsutil.VolumeID(vID.FsSubvolName), volOptions.Size); err != nil {
		log.ErrorLog(ctx, "failed to create volume %s: %v", volOptions.RequestName, err)

		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func checkContentSource(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
	cr *util.Credentials) (*core.VolumeOptions, *core.VolumeIdentifier, *core.SnapshotIdentifier, error) {
	if req.VolumeContentSource == nil {
		return nil, nil, nil, nil
	}
	volumeSource := req.VolumeContentSource
	switch volumeSource.Type.(type) {
	case *csi.VolumeContentSource_Snapshot:
		snapshotID := req.VolumeContentSource.GetSnapshot().GetSnapshotId()
		volOpt, _, sid, err := core.NewSnapshotOptionsFromID(ctx, snapshotID, cr)
		if err != nil {
			if errors.Is(err, cerrors.ErrSnapNotFound) {
				return nil, nil, nil, status.Error(codes.NotFound, err.Error())
			}

			return nil, nil, nil, status.Error(codes.Internal, err.Error())
		}

		return volOpt, nil, sid, nil
	case *csi.VolumeContentSource_Volume:
		// Find the volume using the provided VolumeID
		volID := req.VolumeContentSource.GetVolume().GetVolumeId()
		parentVol, pvID, err := core.NewVolumeOptionsFromVolID(ctx, volID, nil, req.Secrets)
		if err != nil {
			if !errors.Is(err, cerrors.ErrVolumeNotFound) {
				return nil, nil, nil, status.Error(codes.NotFound, err.Error())
			}

			return nil, nil, nil, status.Error(codes.Internal, err.Error())
		}

		return parentVol, pvID, nil, nil
	}

	return nil, nil, nil, status.Errorf(codes.InvalidArgument, "not a proper volume source %v", volumeSource)
}

// CreateVolume creates a reservation and the volume in backend, if it is not already present.
// nolint:gocognit,gocyclo,nestif,cyclop // TODO: reduce complexity
func (cs *ControllerServer) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(req); err != nil {
		log.ErrorLog(ctx, "CreateVolumeRequest validation failed: %v", err)

		return nil, err
	}

	// Configuration
	secret := req.GetSecrets()
	requestName := req.GetName()

	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		log.ErrorLog(ctx, "failed to retrieve admin credentials: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	// Existence and conflict checks
	if acquired := cs.VolumeLocks.TryAcquire(requestName); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, requestName)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, requestName)
	}
	defer cs.VolumeLocks.Release(requestName)

	volOptions, err := core.NewVolumeOptions(ctx, requestName, req, cr)
	if err != nil {
		log.ErrorLog(ctx, "validation and extraction of volume options failed: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer volOptions.Destroy()

	if req.GetCapacityRange() != nil {
		volOptions.Size = util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())
	}

	parentVol, pvID, sID, err := checkContentSource(ctx, req, cr)
	if err != nil {
		return nil, err
	}
	if parentVol != nil {
		defer parentVol.Destroy()
	}

	vID, err := core.CheckVolExists(ctx, volOptions, parentVol, pvID, sID, cr)
	if err != nil {
		if cerrors.IsCloneRetryError(err) {
			return nil, status.Error(codes.Aborted, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	// TODO return error message if requested vol size greater than found volume return error

	if vID != nil {
		if sID != nil || pvID != nil {
			// while cloning the volume the size is not populated properly to the new volume now.
			// it will be fixed in cephfs soon with the parentvolume size. Till then by below
			// resize we are making sure we return or satisfy the requested size by setting the size
			// explicitly
			err = volOptions.ResizeVolume(ctx, fsutil.VolumeID(vID.FsSubvolName), volOptions.Size)
			if err != nil {
				purgeErr := volOptions.PurgeVolume(ctx, fsutil.VolumeID(vID.FsSubvolName), false)
				if purgeErr != nil {
					log.ErrorLog(ctx, "failed to delete volume %s: %v", requestName, purgeErr)
					// All errors other than ErrVolumeNotFound should return an error back to the caller
					if !errors.Is(purgeErr, cerrors.ErrVolumeNotFound) {
						return nil, status.Error(codes.Internal, purgeErr.Error())
					}
				}
				errUndo := core.UndoVolReservation(ctx, volOptions, *vID, secret)
				if errUndo != nil {
					log.WarningLog(ctx, "failed undoing reservation of volume: %s (%s)",
						requestName, errUndo)
				}
				log.ErrorLog(ctx, "failed to expand volume %s: %v", fsutil.VolumeID(vID.FsSubvolName), err)

				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		volumeContext := req.GetParameters()
		volumeContext["subvolumeName"] = vID.FsSubvolName
		volumeContext["subvolumePath"] = volOptions.RootPath
		volume := &csi.Volume{
			VolumeId:      vID.VolumeID,
			CapacityBytes: volOptions.Size,
			ContentSource: req.GetVolumeContentSource(),
			VolumeContext: volumeContext,
		}
		if volOptions.Topology != nil {
			volume.AccessibleTopology =
				[]*csi.Topology{
					{
						Segments: volOptions.Topology,
					},
				}
		}

		return &csi.CreateVolumeResponse{Volume: volume}, nil
	}

	// Reservation
	vID, err = core.ReserveVol(ctx, volOptions, secret)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	defer func() {
		if err != nil {
			if !cerrors.IsCloneRetryError(err) {
				errDefer := core.UndoVolReservation(ctx, volOptions, *vID, secret)
				if errDefer != nil {
					log.WarningLog(ctx, "failed undoing reservation of volume: %s (%s)",
						requestName, errDefer)
				}
			}
		}
	}()

	// Create a volume
	err = cs.createBackingVolume(ctx, volOptions, parentVol, vID, pvID, sID)
	if err != nil {
		if cerrors.IsCloneRetryError(err) {
			return nil, status.Error(codes.Aborted, err.Error())
		}

		return nil, err
	}

	volOptions.RootPath, err = volOptions.GetVolumeRootPathCeph(ctx, fsutil.VolumeID(vID.FsSubvolName))
	if err != nil {
		purgeErr := volOptions.PurgeVolume(ctx, fsutil.VolumeID(vID.FsSubvolName), true)
		if purgeErr != nil {
			log.ErrorLog(ctx, "failed to delete volume %s: %v", vID.FsSubvolName, purgeErr)
			// All errors other than ErrVolumeNotFound should return an error back to the caller
			if !errors.Is(purgeErr, cerrors.ErrVolumeNotFound) {
				// If the subvolume deletion is failed, we should not cleanup
				// the OMAP entry it will stale subvolume in cluster.
				// set err=nil so that when we get the request again we can get
				// the subvolume info.
				err = nil

				return nil, status.Error(codes.Internal, purgeErr.Error())
			}
		}
		log.ErrorLog(ctx, "failed to get subvolume path %s: %v", vID.FsSubvolName, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully created backing volume named %s for request name %s",
		vID.FsSubvolName, requestName)
	volumeContext := req.GetParameters()
	volumeContext["subvolumeName"] = vID.FsSubvolName
	volumeContext["subvolumePath"] = volOptions.RootPath
	volume := &csi.Volume{
		VolumeId:      vID.VolumeID,
		CapacityBytes: volOptions.Size,
		ContentSource: req.GetVolumeContentSource(),
		VolumeContext: volumeContext,
	}
	if volOptions.Topology != nil {
		volume.AccessibleTopology =
			[]*csi.Topology{
				{
					Segments: volOptions.Topology,
				},
			}
	}

	return &csi.CreateVolumeResponse{Volume: volume}, nil
}

// DeleteVolume deletes the volume in backend and its reservation.
func (cs *ControllerServer) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(); err != nil {
		log.ErrorLog(ctx, "DeleteVolumeRequest validation failed: %v", err)

		return nil, err
	}

	volID := fsutil.VolumeID(req.GetVolumeId())
	secrets := req.GetSecrets()

	// lock out parallel delete operations
	if acquired := cs.VolumeLocks.TryAcquire(string(volID)); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, string(volID))
	}
	defer cs.VolumeLocks.Release(string(volID))

	// lock out volumeID for clone and expand operation
	if err := cs.OperationLocks.GetDeleteLock(req.GetVolumeId()); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseDeleteLock(req.GetVolumeId())

	// Find the volume using the provided VolumeID
	volOptions, vID, err := core.NewVolumeOptionsFromVolID(ctx, string(volID), nil, secrets)
	if err != nil {
		// if error is ErrPoolNotFound, the pool is already deleted we dont
		// need to worry about deleting subvolume or omap data, return success
		if errors.Is(err, util.ErrPoolNotFound) {
			log.WarningLog(ctx, "failed to get backend volume for %s: %v", string(volID), err)

			return &csi.DeleteVolumeResponse{}, nil
		}
		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (subvolume and imageOMap are garbage collected already), hence
		// return success as deletion is complete
		if errors.Is(err, util.ErrKeyNotFound) {
			return &csi.DeleteVolumeResponse{}, nil
		}

		log.ErrorLog(ctx, "Error returned from newVolumeOptionsFromVolID: %v", err)

		// All errors other than ErrVolumeNotFound should return an error back to the caller
		if !errors.Is(err, cerrors.ErrVolumeNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If error is ErrImageNotFound then we failed to find the subvolume, but found the imageOMap
		// to lead us to the image, hence the imageOMap needs to be garbage collected, by calling
		// unreserve for the same
		if acquired := cs.VolumeLocks.TryAcquire(volOptions.RequestName); !acquired {
			return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volOptions.RequestName)
		}
		defer cs.VolumeLocks.Release(volOptions.RequestName)

		if err = core.UndoVolReservation(ctx, volOptions, *vID, secrets); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.DeleteVolumeResponse{}, nil
	}
	defer volOptions.Destroy()

	// lock out parallel delete and create requests against the same volume name as we
	// cleanup the subvolume and associated omaps for the same
	if acquired := cs.VolumeLocks.TryAcquire(volOptions.RequestName); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volOptions.RequestName)
	}
	defer cs.VolumeLocks.Release(volOptions.RequestName)

	// Deleting a volume requires admin credentials
	cr, err := util.NewAdminCredentials(secrets)
	if err != nil {
		log.ErrorLog(ctx, "failed to retrieve admin credentials: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	if err = volOptions.PurgeVolume(ctx, fsutil.VolumeID(vID.FsSubvolName), false); err != nil {
		log.ErrorLog(ctx, "failed to delete volume %s: %v", volID, err)
		if errors.Is(err, cerrors.ErrVolumeHasSnapshots) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}

		if !errors.Is(err, cerrors.ErrVolumeNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err := core.UndoVolReservation(ctx, volOptions, *vID, secrets); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully deleted volume %s", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// Cephfs doesn't support Block volume
	for _, capability := range req.VolumeCapabilities {
		if capability.GetBlock() != nil {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

// ControllerExpandVolume expands CephFS Volumes on demand based on resizer request.
func (cs *ControllerServer) ControllerExpandVolume(
	ctx context.Context,
	req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if err := cs.validateExpandVolumeRequest(req); err != nil {
		log.ErrorLog(ctx, "ControllerExpandVolumeRequest validation failed: %v", err)

		return nil, err
	}

	volID := req.GetVolumeId()
	secret := req.GetSecrets()

	// lock out parallel delete operations
	if acquired := cs.VolumeLocks.TryAcquire(volID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer cs.VolumeLocks.Release(volID)

	// lock out volumeID for clone and delete operation
	if err := cs.OperationLocks.GetExpandLock(volID); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseExpandLock(volID)

	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	volOptions, volIdentifier, err := core.NewVolumeOptionsFromVolID(ctx, volID, nil, secret)
	if err != nil {
		log.ErrorLog(ctx, "validation and extraction of volume options failed: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer volOptions.Destroy()

	RoundOffSize := util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())

	if err = volOptions.ResizeVolume(ctx, fsutil.VolumeID(volIdentifier.FsSubvolName), RoundOffSize); err != nil {
		log.ErrorLog(ctx, "failed to expand volume %s: %v", fsutil.VolumeID(volIdentifier.FsSubvolName), err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         RoundOffSize,
		NodeExpansionRequired: false,
	}, nil
}

// CreateSnapshot creates the snapshot in backend and stores metadata
// in store
// nolint:gocyclo,cyclop // golangci-lint did not catch this earlier, needs to get fixed late
func (cs *ControllerServer) CreateSnapshot(
	ctx context.Context,
	req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if err := cs.validateSnapshotReq(ctx, req); err != nil {
		return nil, err
	}
	cr, err := util.NewAdminCredentials(req.GetSecrets())
	if err != nil {
		return nil, err
	}
	defer cr.DeleteCredentials()

	clusterData, err := core.GetClusterInformation(req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	requestName := req.GetName()
	sourceVolID := req.GetSourceVolumeId()
	// Existence and conflict checks
	if acquired := cs.SnapshotLocks.TryAcquire(requestName); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, requestName)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, requestName)
	}
	defer cs.SnapshotLocks.Release(requestName)

	if err = cs.OperationLocks.GetSnapshotCreateLock(sourceVolID); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}

	defer cs.OperationLocks.ReleaseSnapshotCreateLock(sourceVolID)

	// Find the volume using the provided VolumeID
	parentVolOptions, vid, err := core.NewVolumeOptionsFromVolID(ctx, sourceVolID, nil, req.GetSecrets())
	if err != nil {
		if errors.Is(err, util.ErrPoolNotFound) {
			log.WarningLog(ctx, "failed to get backend volume for %s: %v", sourceVolID, err)

			return nil, status.Error(codes.NotFound, err.Error())
		}

		if errors.Is(err, cerrors.ErrVolumeNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	defer parentVolOptions.Destroy()

	if clusterData.ClusterID != parentVolOptions.ClusterID {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"requested cluster id %s not matching subvolume cluster id %s",
			clusterData.ClusterID,
			parentVolOptions.ClusterID)
	}

	cephfsSnap, genSnapErr := core.GenSnapFromOptions(ctx, req)
	if genSnapErr != nil {
		return nil, status.Error(codes.Internal, genSnapErr.Error())
	}

	// lock out parallel snapshot create operations
	if acquired := cs.VolumeLocks.TryAcquire(sourceVolID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, sourceVolID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, sourceVolID)
	}
	defer cs.VolumeLocks.Release(sourceVolID)
	snapName := req.GetName()
	sid, snapInfo, err := core.CheckSnapExists(ctx, parentVolOptions, vid.FsSubvolName, cephfsSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// check are we able to retrieve the size of parent
	// ceph fs subvolume info command got added in 14.2.10 and 15.+
	// as we are not able to retrieve the parent size we are rejecting the
	// request to create snapshot.
	// TODO: For this purpose we could make use of cached clusterAdditionalInfo too.
	info, err := parentVolOptions.GetSubVolumeInfo(ctx, fsutil.VolumeID(vid.FsSubvolName))
	if err != nil {
		// Check error code value against ErrInvalidCommand to understand the cluster
		// support it or not, It's safe to evaluate as the filtering
		// is already done from GetSubVolumeInfo() and send out the error here.
		if errors.Is(err, cerrors.ErrInvalidCommand) {
			return nil, status.Error(
				codes.FailedPrecondition,
				"subvolume info command not supported in current ceph cluster")
		}
		if sid != nil {
			errDefer := core.UndoSnapReservation(ctx, parentVolOptions, *sid, snapName, cr)
			if errDefer != nil {
				log.WarningLog(ctx, "failed undoing reservation of snapshot: %s (%s)",
					requestName, errDefer)
			}
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	if sid != nil {
		// check snapshot is protected
		protected := true
		if !(snapInfo.Protected == core.SnapshotIsProtected) {
			err = parentVolOptions.ProtectSnapshot(ctx, fsutil.VolumeID(sid.FsSnapshotName), fsutil.VolumeID(vid.FsSubvolName))
			if err != nil {
				protected = false
				log.WarningLog(ctx, "failed to protect snapshot of snapshot: %s (%s)",
					sid.FsSnapshotName, err)
			}
		}

		return &csi.CreateSnapshotResponse{
			Snapshot: &csi.Snapshot{
				SizeBytes:      info.BytesQuota,
				SnapshotId:     sid.SnapshotID,
				SourceVolumeId: req.GetSourceVolumeId(),
				CreationTime:   sid.CreationTime,
				ReadyToUse:     protected,
			},
		}, nil
	}

	// Reservation
	sID, err := core.ReserveSnap(ctx, parentVolOptions, vid.FsSubvolName, cephfsSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := core.UndoSnapReservation(ctx, parentVolOptions, *sID, snapName, cr)
			if errDefer != nil {
				log.WarningLog(ctx, "failed undoing reservation of snapshot: %s (%s)",
					requestName, errDefer)
			}
		}
	}()
	snap, err := doSnapshot(ctx, parentVolOptions, vid.FsSubvolName, sID.FsSnapshotName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      info.BytesQuota,
			SnapshotId:     sID.SnapshotID,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   snap.CreationTime,
			ReadyToUse:     true,
		},
	}, nil
}

func doSnapshot(
	ctx context.Context,
	volOpt *core.VolumeOptions,
	subvolumeName,
	snapshotName string) (core.SnapshotInfo, error) {
	volID := fsutil.VolumeID(subvolumeName)
	snapID := fsutil.VolumeID(snapshotName)
	snap := core.SnapshotInfo{}
	err := volOpt.CreateSnapshot(ctx, snapID, volID)
	if err != nil {
		log.ErrorLog(ctx, "failed to create snapshot %s %v", snapID, err)

		return snap, err
	}
	defer func() {
		if err != nil {
			dErr := volOpt.DeleteSnapshot(ctx, snapID, volID)
			if dErr != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapID, err)
			}
		}
	}()
	snap, err = volOpt.GetSnapshotInfo(ctx, snapID, volID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get snapshot info %s %v", snapID, err)

		return snap, fmt.Errorf("failed to get snapshot info for snapshot:%s", snapID)
	}
	var t *timestamp.Timestamp
	t, err = fsutil.ParseTime(ctx, snap.CreatedAt)
	if err != nil {
		return snap, err
	}
	snap.CreationTime = t
	err = volOpt.ProtectSnapshot(ctx, snapID, volID)
	if err != nil {
		log.ErrorLog(ctx, "failed to protect snapshot %s %v", snapID, err)
	}

	return snap, err
}

func (cs *ControllerServer) validateSnapshotReq(ctx context.Context, req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		log.ErrorLog(ctx, "invalid create snapshot req: %v", protosanitizer.StripSecrets(req))

		return err
	}

	// Check sanity of request Snapshot Name, Source Volume Id
	if req.Name == "" {
		return status.Error(codes.NotFound, "snapshot Name cannot be empty")
	}
	if req.SourceVolumeId == "" {
		return status.Error(codes.NotFound, "source Volume ID cannot be empty")
	}

	return nil
}

// DeleteSnapshot deletes the snapshot in backend and removes the
// snapshot metadata from store.
func (cs *ControllerServer) DeleteSnapshot(
	ctx context.Context,
	req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		log.ErrorLog(ctx, "invalid delete snapshot req: %v", protosanitizer.StripSecrets(req))

		return nil, err
	}

	cr, err := util.NewAdminCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()
	snapshotID := req.GetSnapshotId()
	if snapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID cannot be empty")
	}

	if acquired := cs.SnapshotLocks.TryAcquire(snapshotID); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, snapshotID)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, snapshotID)
	}
	defer cs.SnapshotLocks.Release(snapshotID)

	// lock out snapshotID for restore operation
	if err = cs.OperationLocks.GetDeleteLock(snapshotID); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseDeleteLock(snapshotID)

	volOpt, snapInfo, sid, err := core.NewSnapshotOptionsFromID(ctx, snapshotID, cr)
	if err != nil {
		switch {
		case errors.Is(err, util.ErrPoolNotFound):
			// if error is ErrPoolNotFound, the pool is already deleted we dont
			// need to worry about deleting snapshot or omap data, return success
			log.WarningLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)

			return &csi.DeleteSnapshotResponse{}, nil
		case errors.Is(err, util.ErrKeyNotFound):
			// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
			// or partially complete (snap and snapOMap are garbage collected already), hence return
			// success as deletion is complete
			return &csi.DeleteSnapshotResponse{}, nil
		case errors.Is(err, cerrors.ErrSnapNotFound):
			err = core.UndoSnapReservation(ctx, volOpt, *sid, sid.FsSnapshotName, cr)
			if err != nil {
				log.ErrorLog(ctx, "failed to remove reservation for snapname (%s) with backing snap (%s) (%s)",
					sid.FsSubvolName, sid.FsSnapshotName, err)

				return nil, status.Error(codes.Internal, err.Error())
			}

			return &csi.DeleteSnapshotResponse{}, nil
		case errors.Is(err, cerrors.ErrVolumeNotFound):
			// if the error is ErrVolumeNotFound, the subvolume is already deleted
			// from backend, Hence undo the omap entries and return success
			log.ErrorLog(ctx, "Volume not present")
			err = core.UndoSnapReservation(ctx, volOpt, *sid, sid.FsSnapshotName, cr)
			if err != nil {
				log.ErrorLog(ctx, "failed to remove reservation for snapname (%s) with backing snap (%s) (%s)",
					sid.FsSubvolName, sid.FsSnapshotName, err)

				return nil, status.Error(codes.Internal, err.Error())
			}

			return &csi.DeleteSnapshotResponse{}, nil
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	defer volOpt.Destroy()

	// safeguard against parallel create or delete requests against the same
	// name
	if acquired := cs.SnapshotLocks.TryAcquire(sid.RequestName); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, sid.RequestName)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, sid.RequestName)
	}
	defer cs.SnapshotLocks.Release(sid.RequestName)

	if snapInfo.HasPendingClones == "yes" {
		return nil, status.Errorf(codes.FailedPrecondition, "snapshot %s has pending clones", snapshotID)
	}
	if snapInfo.Protected == core.SnapshotIsProtected {
		err = volOpt.UnprotectSnapshot(ctx, fsutil.VolumeID(sid.FsSnapshotName), fsutil.VolumeID(sid.FsSubvolName))
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	err = volOpt.DeleteSnapshot(ctx, fsutil.VolumeID(sid.FsSnapshotName), fsutil.VolumeID(sid.FsSubvolName))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	err = core.UndoSnapReservation(ctx, volOpt, *sid, sid.FsSnapshotName, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to remove reservation for snapname (%s) with backing snap (%s) (%s)",
			sid.RequestName, sid.FsSnapshotName, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteSnapshotResponse{}, nil
}
