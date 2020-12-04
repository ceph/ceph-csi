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

package rbd

import (
	"context"
	"errors"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	oneGB = 1073741824
)

// ControllerServer struct of rbd CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	VolumeLocks *util.VolumeLocks

	// A map storing all volumes with ongoing operations so that additional operations
	// for that same snapshot (as defined by SnapshotID/snapshot name) return an Aborted error
	SnapshotLocks *util.VolumeLocks

	// A map storing all volumes/snapshots with ongoing operations.
	OperationLocks *util.OperationLock
}

func (cs *ControllerServer) validateVolumeReq(ctx context.Context, req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		util.ErrorLog(ctx, "invalid create volume req: %v", protosanitizer.StripSecrets(req))
		return err
	}
	// Check sanity of request Name, Volume Capabilities
	if req.Name == "" {
		return status.Error(codes.InvalidArgument, "volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		return status.Error(codes.InvalidArgument, "volume Capabilities cannot be empty")
	}
	options := req.GetParameters()
	if value, ok := options["clusterID"]; !ok || value == "" {
		return status.Error(codes.InvalidArgument, "missing or empty cluster ID to provision volume from")
	}
	if value, ok := options["pool"]; !ok || value == "" {
		return status.Error(codes.InvalidArgument, "missing or empty pool name to provision volume from")
	}

	if value, ok := options["dataPool"]; ok && value == "" {
		return status.Error(codes.InvalidArgument, "empty datapool name to provision volume from")
	}
	if value, ok := options["radosNamespace"]; ok && value == "" {
		return status.Error(codes.InvalidArgument, "empty namespace name to provision volume from")
	}
	if value, ok := options["volumeNamePrefix"]; ok && value == "" {
		return status.Error(codes.InvalidArgument, "empty volume name prefix to provision volume from")
	}

	// Allow readonly access mode for volume with content source
	err := util.CheckReadOnlyManyIsSupported(req)
	if err != nil {
		return err
	}

	return nil
}

func (cs *ControllerServer) parseVolCreateRequest(ctx context.Context, req *csi.CreateVolumeRequest) (*rbdVolume, error) {
	// TODO (sbezverk) Last check for not exceeding total storage capacity

	isMultiNode := false
	isBlock := false
	for _, capability := range req.VolumeCapabilities {
		// RO modes need to be handled independently (ie right now even if access mode is RO, they'll be RW upon attach)
		if capability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			isMultiNode = true
		}
		if capability.GetBlock() != nil {
			isBlock = true
		}
	}

	// We want to fail early if the user is trying to create a RWX on a non-block type device
	if isMultiNode && !isBlock {
		return nil, status.Error(codes.InvalidArgument, "multi node access modes are only supported on rbd `block` type volumes")
	}

	// if it's NOT SINGLE_NODE_WRITER and it's BLOCK we'll set the parameter to ignore the in-use checks
	rbdVol, err := genVolFromVolumeOptions(ctx, req.GetParameters(), req.GetSecrets(), (isMultiNode && isBlock))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rbdVol.RequestName = req.GetName()

	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(oneGB)
	if req.GetCapacityRange() != nil {
		volSizeBytes = req.GetCapacityRange().GetRequiredBytes()
	}

	// always round up the request size in bytes to the nearest MiB/GiB
	rbdVol.VolSize = util.RoundOffBytes(volSizeBytes)

	// start with pool the same as journal pool, in case there is a topology
	// based split, pool for the image will be updated subsequently
	rbdVol.JournalPool = rbdVol.Pool

	// store topology information from the request
	rbdVol.TopologyPools, rbdVol.TopologyRequirement, err = util.GetTopologyFromRequest(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// NOTE: rbdVol does not contain VolID and RbdImageName populated, everything
	// else is populated post create request parsing
	return rbdVol, nil
}

func buildCreateVolumeResponse(ctx context.Context, req *csi.CreateVolumeRequest, rbdVol *rbdVolume) (*csi.CreateVolumeResponse, error) {
	if rbdVol.Encrypted {
		err := rbdVol.ensureEncryptionMetadataSet(rbdImageRequiresEncryption)
		if err != nil {
			util.ErrorLog(ctx, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	volumeContext := req.GetParameters()
	volumeContext["pool"] = rbdVol.Pool
	volumeContext["journalPool"] = rbdVol.JournalPool
	volumeContext["imageName"] = rbdVol.RbdImageName
	if rbdVol.RadosNamespace != "" {
		volumeContext["radosNamespace"] = rbdVol.RadosNamespace
	}
	volume := &csi.Volume{
		VolumeId:      rbdVol.VolID,
		CapacityBytes: rbdVol.VolSize,
		VolumeContext: volumeContext,
		ContentSource: req.GetVolumeContentSource(),
	}
	if rbdVol.Topology != nil {
		volume.AccessibleTopology =
			[]*csi.Topology{
				{
					Segments: rbdVol.Topology,
				},
			}
	}
	return &csi.CreateVolumeResponse{Volume: volume}, nil
}

// getGRPCErrorForCreateVolume converts the returns the GRPC errors based on
// the input error types it expected to use only for CreateVolume as we need to
// return different GRPC codes for different functions based on the input.
func getGRPCErrorForCreateVolume(err error) error {
	if errors.Is(err, ErrVolNameConflict) {
		return status.Error(codes.AlreadyExists, err.Error())
	}
	if errors.Is(err, ErrFlattenInProgress) {
		return status.Error(codes.Aborted, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

// validateRequestedVolumeSize validates the request volume size with the
// source snapshot or volume size, if there is a size mismatches it returns an error.
func validateRequestedVolumeSize(rbdVol, parentVol *rbdVolume, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	if rbdSnap != nil {
		vol := generateVolFromSnap(rbdSnap)
		err := vol.Connect(cr)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		defer vol.Destroy()

		err = vol.getImageInfo()
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		if rbdVol.VolSize != vol.VolSize {
			return status.Errorf(codes.InvalidArgument, "size mismatches, requested volume size %d and source snapshot size %d", rbdVol.VolSize, vol.VolSize)
		}
	}
	if parentVol != nil {
		if rbdVol.VolSize != parentVol.VolSize {
			return status.Errorf(codes.InvalidArgument, "size mismatches, requested volume size %d and source volume size %d", rbdVol.VolSize, parentVol.VolSize)
		}
	}
	return nil
}

// CreateVolume creates the volume in backend.
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateVolumeReq(ctx, req); err != nil {
		return nil, err
	}

	// TODO: create/get a connection from the the ConnPool, and do not pass
	// the credentials to any of the utility functions.
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	rbdVol, err := cs.parseVolCreateRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer rbdVol.Destroy()
	// Existence and conflict checks
	if acquired := cs.VolumeLocks.TryAcquire(req.GetName()); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, req.GetName())
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.VolumeLocks.Release(req.GetName())

	err = rbdVol.Connect(cr)
	if err != nil {
		util.ErrorLog(ctx, "failed to connect to volume %v: %v", rbdVol.RbdImageName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	parentVol, rbdSnap, err := checkContentSource(ctx, req, cr)
	if err != nil {
		return nil, err
	}

	found, err := rbdVol.Exists(ctx, parentVol)
	if err != nil {
		return nil, getGRPCErrorForCreateVolume(err)
	}
	if found {
		if rbdSnap != nil {
			// check if image depth is reached limit and requires flatten
			err = checkFlatten(ctx, rbdVol, cr)
			if err != nil {
				return nil, err
			}
		}
		return buildCreateVolumeResponse(ctx, req, rbdVol)
	}

	err = validateRequestedVolumeSize(rbdVol, parentVol, rbdSnap, cr)
	if err != nil {
		return nil, err
	}

	err = flattenParentImage(ctx, parentVol, cr)
	if err != nil {
		return nil, err
	}

	err = reserveVol(ctx, rbdVol, rbdSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			if !errors.Is(err, ErrFlattenInProgress) {
				errDefer := undoVolReservation(ctx, rbdVol, cr)
				if errDefer != nil {
					util.WarningLog(ctx, "failed undoing reservation of volume: %s (%s)", req.GetName(), errDefer)
				}
			}
		}
	}()

	err = cs.createBackingImage(ctx, cr, rbdVol, parentVol, rbdSnap)
	if err != nil {
		if errors.Is(err, ErrFlattenInProgress) {
			return nil, status.Error(codes.Aborted, err.Error())
		}
		return nil, err
	}

	volumeContext := req.GetParameters()
	volumeContext["pool"] = rbdVol.Pool
	volumeContext["journalPool"] = rbdVol.JournalPool
	volumeContext["radosNamespace"] = rbdVol.RadosNamespace
	volumeContext["imageName"] = rbdVol.RbdImageName
	volume := &csi.Volume{
		VolumeId:      rbdVol.VolID,
		CapacityBytes: rbdVol.VolSize,
		VolumeContext: volumeContext,
		ContentSource: req.GetVolumeContentSource(),
	}
	if rbdVol.Topology != nil {
		volume.AccessibleTopology =
			[]*csi.Topology{
				{
					Segments: rbdVol.Topology,
				},
			}
	}
	return &csi.CreateVolumeResponse{Volume: volume}, nil
}

func flattenParentImage(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	if rbdVol != nil {
		// flatten the image or its parent before the reservation to avoid
		// stale entries in post creation if we return ABORT error and the
		// delete volume is not called
		err := rbdVol.flattenCloneImage(ctx)
		if err != nil {
			return getGRPCErrorForCreateVolume(err)
		}

		// flatten cloned images if the snapshot count on the parent image
		// exceeds maxSnapshotsOnImage
		err = flattenTemporaryClonedImages(ctx, rbdVol, cr)
		if err != nil {
			return err
		}
	}
	return nil
}

// check snapshots on the rbd image, as we have limit from krbd that an image
// cannot have more than 510 snapshot at a given point of time. If the
// snapshots are more than the `maxSnapshotsOnImage` Add a task to flatten all
// the temporary cloned images and return ABORT error message. If the snapshots
// are more than the `minSnapshotOnImage` Add a task to flatten all the
// temporary cloned images.
func flattenTemporaryClonedImages(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	snaps, err := rbdVol.listSnapshots()
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		return status.Error(codes.Internal, err.Error())
	}

	if len(snaps) > int(maxSnapshotsOnImage) {
		util.DebugLog(ctx, "snapshots count %d on image: %s reached configured hard limit %d", len(snaps), rbdVol, maxSnapshotsOnImage)
		err = flattenClonedRbdImages(
			ctx,
			snaps,
			rbdVol.Pool,
			rbdVol.Monitors,
			rbdVol.RbdImageName,
			cr)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		return status.Errorf(codes.ResourceExhausted, "rbd image %s has %d snapshots", rbdVol, len(snaps))
	}

	if len(snaps) > int(minSnapshotsOnImageToStartFlatten) {
		util.DebugLog(ctx, "snapshots count %d on image: %s reached configured soft limit %d", len(snaps), rbdVol, minSnapshotsOnImageToStartFlatten)
		// If we start flattening all the snapshots at one shot the volume
		// creation time will be affected,so we will flatten only the extra
		// snapshots.
		snaps = snaps[minSnapshotsOnImageToStartFlatten-1:]
		err = flattenClonedRbdImages(
			ctx,
			snaps,
			rbdVol.Pool,
			rbdVol.Monitors,
			rbdVol.RbdImageName,
			cr)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
	return nil
}

// checkFlatten ensures that that the image chain depth is not reached
// hardlimit or softlimit. if the softlimit is reached it adds a task and
// return success,the hardlimit is reached it starts a task to flatten the
// image and return Aborted.
func checkFlatten(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	err := rbdVol.flattenRbdImage(ctx, cr, false, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
	if err != nil {
		if errors.Is(err, ErrFlattenInProgress) {
			return status.Error(codes.Aborted, err.Error())
		}
		if errDefer := deleteImage(ctx, rbdVol, cr); errDefer != nil {
			util.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v", rbdVol, errDefer)
			return status.Error(codes.Internal, err.Error())
		}
		errDefer := undoVolReservation(ctx, rbdVol, cr)
		if errDefer != nil {
			util.WarningLog(ctx, "failed undoing reservation of volume: %s (%s)", rbdVol.RequestName, errDefer)
		}
		return status.Error(codes.Internal, err.Error())
	}
	return nil
}

func (cs *ControllerServer) createVolumeFromSnapshot(ctx context.Context, cr *util.Credentials, rbdVol *rbdVolume, snapshotID string) error {
	rbdSnap := &rbdSnapshot{}
	if acquired := cs.SnapshotLocks.TryAcquire(snapshotID); !acquired {
		util.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, snapshotID)
		return status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, snapshotID)
	}
	defer cs.SnapshotLocks.Release(snapshotID)

	err := genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr)
	if err != nil {
		if errors.Is(err, util.ErrPoolNotFound) {
			util.ErrorLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)
			return status.Error(codes.InvalidArgument, err.Error())
		}
		return status.Error(codes.Internal, err.Error())
	}

	// update parent name(rbd image name in snapshot)
	rbdSnap.RbdImageName = rbdSnap.RbdSnapName
	// create clone image and delete snapshot
	err = rbdVol.cloneRbdImageFromSnapshot(ctx, rbdSnap)
	if err != nil {
		util.ErrorLog(ctx, "failed to clone rbd image %s from snapshot %s: %v", rbdSnap, err)
		return err
	}

	util.DebugLog(ctx, "create volume %s from snapshot %s", rbdVol.RequestName, rbdSnap.RbdSnapName)
	return nil
}

func (cs *ControllerServer) createBackingImage(ctx context.Context, cr *util.Credentials, rbdVol, parentVol *rbdVolume, rbdSnap *rbdSnapshot) error {
	var err error

	var j = &journal.Connection{}
	j, err = volJournal.Connect(rbdVol.Monitors, rbdVol.RadosNamespace, cr)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer j.Destroy()

	// nolint:gocritic // this ifElseChain can not be rewritten to a switch statement
	if rbdSnap != nil {
		if err = cs.OperationLocks.GetRestoreLock(rbdSnap.SnapID); err != nil {
			util.ErrorLog(ctx, err.Error())
			return status.Error(codes.Aborted, err.Error())
		}
		defer cs.OperationLocks.ReleaseRestoreLock(rbdSnap.SnapID)

		err = cs.createVolumeFromSnapshot(ctx, cr, rbdVol, rbdSnap.SnapID)
		if err != nil {
			return err
		}
		util.DebugLog(ctx, "created volume %s from snapshot %s", rbdVol.RequestName, rbdSnap.RbdSnapName)
	} else if parentVol != nil {
		if err = cs.OperationLocks.GetCloneLock(parentVol.VolID); err != nil {
			util.ErrorLog(ctx, err.Error())
			return status.Error(codes.Aborted, err.Error())
		}
		defer cs.OperationLocks.ReleaseCloneLock(parentVol.VolID)
		return rbdVol.createCloneFromImage(ctx, parentVol)
	} else {
		err = createImage(ctx, rbdVol, cr)
		if err != nil {
			util.ErrorLog(ctx, "failed to create volume: %v", err)
			return status.Error(codes.Internal, err.Error())
		}
	}

	util.DebugLog(ctx, "created volume %s backed by image %s", rbdVol.RequestName, rbdVol.RbdImageName)

	defer func() {
		if err != nil {
			if !errors.Is(err, ErrFlattenInProgress) {
				if deleteErr := deleteImage(ctx, rbdVol, cr); deleteErr != nil {
					util.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v", rbdVol, deleteErr)
				}
			}
		}
	}()
	err = rbdVol.storeImageID(ctx, j)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	if rbdSnap != nil {
		err = rbdVol.flattenRbdImage(ctx, cr, false, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
		if err != nil {
			util.ErrorLog(ctx, "failed to flatten image %s: %v", rbdVol, err)
			return err
		}
	}
	if rbdVol.Encrypted {
		err = rbdVol.ensureEncryptionMetadataSet(rbdImageRequiresEncryption)
		if err != nil {
			util.ErrorLog(ctx, "failed to save encryption status, deleting image %s: %s",
				rbdVol, err)
			return status.Error(codes.Internal, err.Error())
		}
	}
	return nil
}

func checkContentSource(ctx context.Context, req *csi.CreateVolumeRequest, cr *util.Credentials) (*rbdVolume, *rbdSnapshot, error) {
	if req.VolumeContentSource == nil {
		return nil, nil, nil
	}
	volumeSource := req.VolumeContentSource
	switch volumeSource.Type.(type) {
	case *csi.VolumeContentSource_Snapshot:
		snapshot := req.VolumeContentSource.GetSnapshot()
		if snapshot == nil {
			return nil, nil, status.Error(codes.NotFound, "volume Snapshot cannot be empty")
		}
		snapshotID := snapshot.GetSnapshotId()
		if snapshotID == "" {
			return nil, nil, status.Errorf(codes.NotFound, "volume Snapshot ID cannot be empty")
		}
		rbdSnap := &rbdSnapshot{}
		if err := genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr); err != nil {
			util.ErrorLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)
			if !errors.Is(err, ErrSnapNotFound) {
				return nil, nil, status.Error(codes.Internal, err.Error())
			}
			return nil, nil, status.Errorf(codes.NotFound, "%s snapshot does not exist", snapshotID)
		}
		return nil, rbdSnap, nil
	case *csi.VolumeContentSource_Volume:
		vol := req.VolumeContentSource.GetVolume()
		if vol == nil {
			return nil, nil, status.Error(codes.NotFound, "volume cannot be empty")
		}
		volID := vol.GetVolumeId()
		if volID == "" {
			return nil, nil, status.Errorf(codes.NotFound, "volume ID cannot be empty")
		}
		// TODO need to support cloning for encrypted volume
		rbdvol, err := genVolFromVolID(ctx, volID, cr, nil)
		if err != nil {
			util.ErrorLog(ctx, "failed to get backend image for %s: %v", volID, err)
			if !errors.Is(err, ErrImageNotFound) {
				return nil, nil, status.Error(codes.Internal, err.Error())
			}
			return nil, nil, status.Errorf(codes.NotFound, "%s image does not exist", volID)
		}
		return rbdvol, nil, nil
	}
	return nil, nil, status.Errorf(codes.InvalidArgument, "not a proper volume source")
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store
// TODO: make this function less complex
// nolint:gocyclo // golangci-lint did not catch this earlier, needs to get fixed later
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		util.ErrorLog(ctx, "invalid delete volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	// For now the image get unconditionally deleted, but here retention policy can be checked
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	if acquired := cs.VolumeLocks.TryAcquire(volumeID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.VolumeLocks.Release(volumeID)

	// lock out volumeID for clone and expand operation
	if err = cs.OperationLocks.GetDeleteLock(volumeID); err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseDeleteLock(volumeID)

	var rbdVol = &rbdVolume{}
	rbdVol, err = genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		if errors.Is(err, util.ErrPoolNotFound) {
			util.WarningLog(ctx, "failed to get backend volume for %s: %v", volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}

		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (image and imageOMap are garbage collected already), hence return
		// success as deletion is complete
		if errors.Is(err, util.ErrKeyNotFound) {
			util.WarningLog(ctx, "Failed to volume options for %s: %v", volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}

		// All errors other than ErrImageNotFound should return an error back to the caller
		if !errors.Is(err, ErrImageNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If error is ErrImageNotFound then we failed to find the image, but found the imageOMap
		// to lead us to the image, hence the imageOMap needs to be garbage collected, by calling
		// unreserve for the same
		if acquired := cs.VolumeLocks.TryAcquire(rbdVol.RequestName); !acquired {
			util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
			return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
		}
		defer cs.VolumeLocks.Release(rbdVol.RequestName)

		if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteVolumeResponse{}, nil
	}

	// lock out parallel create requests against the same volume name as we
	// cleanup the image and associated omaps for the same
	if acquired := cs.VolumeLocks.TryAcquire(rbdVol.RequestName); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
	}
	defer cs.VolumeLocks.Release(rbdVol.RequestName)

	inUse, err := rbdVol.isInUse()
	if err != nil {
		util.ErrorLog(ctx, "failed getting information for image (%s): (%s)", rbdVol, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if inUse {
		util.ErrorLog(ctx, "rbd %s is still being used", rbdVol)
		return nil, status.Errorf(codes.Internal, "rbd %s is still being used", rbdVol.RbdImageName)
	}

	// delete the temporary rbd image created as part of volume clone during
	// create volume
	tempClone := rbdVol.generateTempClone()
	err = deleteImage(ctx, tempClone, cr)
	if err != nil {
		// return error if it is not ErrImageNotFound
		if !errors.Is(err, ErrImageNotFound) {
			util.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v",
				tempClone, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Deleting rbd image
	util.DebugLog(ctx, "deleting image %s", rbdVol.RbdImageName)
	if err = deleteImage(ctx, rbdVol, cr); err != nil {
		util.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v",
			rbdVol, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
		util.ErrorLog(ctx, "failed to remove reservation for volume (%s) with backing image (%s) (%s)",
			rbdVol.RequestName, rbdVol.RbdImageName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if rbdVol.Encrypted {
		if err = rbdVol.KMS.DeletePassphrase(rbdVol.VolID); err != nil {
			util.WarningLog(ctx, "failed to clean the passphrase for volume %s: %s", rbdVol.VolID, err)
		}
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "empty volume capabilities in request")
	}

	for _, capability := range req.VolumeCapabilities {
		if capability.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

// CreateSnapshot creates the snapshot in backend and stores metadata
// in store
// TODO: make this function less complex
// nolint:gocyclo // complexity needs to be reduced.
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if err := cs.validateSnapshotReq(ctx, req); err != nil {
		return nil, err
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	var rbdVol = &rbdVolume{}
	// Fetch source volume information
	rbdVol, err = genVolFromVolID(ctx, req.GetSourceVolumeId(), cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		// nolint:gocritic // this ifElseChain can not be rewritten to a switch statement
		if errors.Is(err, ErrImageNotFound) {
			err = status.Errorf(codes.NotFound, "source Volume ID %s not found", req.GetSourceVolumeId())
		} else if errors.Is(err, util.ErrPoolNotFound) {
			util.ErrorLog(ctx, "failed to get backend volume for %s: %v", req.GetSourceVolumeId(), err)
			err = status.Errorf(codes.NotFound, err.Error())
		} else {
			err = status.Errorf(codes.Internal, err.Error())
		}
		return nil, err
	}

	// TODO: re-encrypt snapshot with a new passphrase
	if rbdVol.Encrypted {
		return nil, status.Errorf(codes.Unimplemented, "source Volume %s is encrypted, "+
			"snapshotting is not supported currently", rbdVol.VolID)
	}

	// Check if source volume was created with required image features for snaps
	if !rbdVol.hasSnapshotFeature() {
		return nil, status.Errorf(codes.InvalidArgument, "volume(%s) has not snapshot feature(layering)", req.GetSourceVolumeId())
	}

	rbdSnap, err := genSnapFromOptions(ctx, rbdVol, req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	rbdSnap.RbdImageName = rbdVol.RbdImageName
	rbdSnap.SizeBytes = rbdVol.VolSize
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.RequestName = req.GetName()

	if acquired := cs.SnapshotLocks.TryAcquire(req.GetName()); !acquired {
		util.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, req.GetName())
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.SnapshotLocks.Release(req.GetName())

	// Take lock on parent rbd image
	if err = cs.OperationLocks.GetSnapshotCreateLock(rbdSnap.SourceVolumeID); err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseSnapshotCreateLock(rbdSnap.SourceVolumeID)

	// Need to check for already existing snapshot name, and if found
	// check for the requested source volume id and already allocated source volume id
	found, err := checkSnapCloneExists(ctx, rbdVol, rbdSnap, cr)
	if err != nil {
		if errors.Is(err, util.ErrSnapNameConflict) {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if found {
		vol := generateVolFromSnap(rbdSnap)
		err = vol.Connect(cr)
		if err != nil {
			uErr := undoSnapshotCloning(ctx, vol, rbdSnap, vol, cr)
			if uErr != nil {
				util.WarningLog(ctx, "failed undoing reservation of snapshot: %s %v", req.GetName(), uErr)
			}
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		defer vol.Destroy()

		err = vol.flattenRbdImage(ctx, cr, false, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
		if errors.Is(err, ErrFlattenInProgress) {
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      rbdSnap.SizeBytes,
					SnapshotId:     rbdSnap.SnapID,
					SourceVolumeId: rbdSnap.SourceVolumeID,
					CreationTime:   rbdSnap.CreatedAt,
					ReadyToUse:     false,
				},
			}, nil
		}
		if err != nil {
			uErr := undoSnapshotCloning(ctx, vol, rbdSnap, vol, cr)
			if uErr != nil {
				util.WarningLog(ctx, "failed undoing reservation of snapshot: %s %v", req.GetName(), uErr)
			}
			return nil, status.Errorf(codes.Internal, err.Error())
		}

		return &csi.CreateSnapshotResponse{
			Snapshot: &csi.Snapshot{
				SizeBytes:      rbdSnap.SizeBytes,
				SnapshotId:     rbdSnap.SnapID,
				SourceVolumeId: rbdSnap.SourceVolumeID,
				CreationTime:   rbdSnap.CreatedAt,
				ReadyToUse:     true,
			},
		}, nil
	}

	err = flattenTemporaryClonedImages(ctx, rbdVol, cr)
	if err != nil {
		return nil, err
	}

	err = reserveSnap(ctx, rbdSnap, rbdVol, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoSnapReservation(ctx, rbdSnap, cr)
			if errDefer != nil {
				util.WarningLog(ctx, "failed undoing reservation of snapshot: %s %v", req.GetName(), errDefer)
			}
		}
	}()

	ready := false
	var vol = new(rbdVolume)

	ready, vol, err = cs.doSnapshotClone(ctx, rbdVol, rbdSnap, cr)
	if err != nil {
		return nil, err
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      vol.VolSize,
			SnapshotId:     vol.VolID,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   vol.CreatedAt,
			ReadyToUse:     ready,
		},
	}, nil
}

func (cs *ControllerServer) validateSnapshotReq(ctx context.Context, req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		util.ErrorLog(ctx, "invalid create snapshot req: %v", protosanitizer.StripSecrets(req))
		return err
	}

	// Check sanity of request Snapshot Name, Source Volume Id
	if req.Name == "" {
		return status.Error(codes.InvalidArgument, "snapshot Name cannot be empty")
	}
	if req.SourceVolumeId == "" {
		return status.Error(codes.InvalidArgument, "source Volume ID cannot be empty")
	}

	options := req.GetParameters()
	if value, ok := options["snapshotNamePrefix"]; ok && value == "" {
		return status.Error(codes.InvalidArgument, "empty snapshot name prefix to provision snapshot from")
	}

	return nil
}

func (cs *ControllerServer) doSnapshotClone(ctx context.Context, parentVol *rbdVolume, rbdSnap *rbdSnapshot, cr *util.Credentials) (bool, *rbdVolume, error) {
	// generate cloned volume details from snapshot
	cloneRbd := generateVolFromSnap(rbdSnap)
	defer cloneRbd.Destroy()
	// add image feature for cloneRbd
	f := []string{librbd.FeatureNameLayering, librbd.FeatureNameDeepFlatten}
	cloneRbd.imageFeatureSet = librbd.FeatureSetFromNames(f)
	ready := false

	err := cloneRbd.Connect(cr)
	if err != nil {
		return ready, cloneRbd, err
	}

	err = createRBDClone(ctx, parentVol, cloneRbd, rbdSnap, cr)
	if err != nil {
		util.ErrorLog(ctx, "failed to create snapshot: %v", err)
		return ready, cloneRbd, status.Error(codes.Internal, err.Error())
	}

	defer func() {
		if err != nil {
			if !errors.Is(err, ErrFlattenInProgress) {
				// cleanup clone and snapshot
				errCleanUp := cleanUpSnapshot(ctx, cloneRbd, rbdSnap, cloneRbd, cr)
				if errCleanUp != nil {
					util.ErrorLog(ctx, "failed to cleanup snapshot and clone: %v", errCleanUp)
				}
			}
		}
	}()

	err = cloneRbd.createSnapshot(ctx, rbdSnap)
	if err != nil {
		// update rbd image name for logging
		rbdSnap.RbdImageName = cloneRbd.RbdImageName
		util.ErrorLog(ctx, "failed to create snapshot %s: %v", rbdSnap, err)
		return ready, cloneRbd, err
	}

	err = cloneRbd.getImageID()
	if err != nil {
		util.ErrorLog(ctx, "failed to get image id: %v", err)
		return ready, cloneRbd, err
	}
	var j = &journal.Connection{}
	// save image ID
	j, err = snapJournal.Connect(rbdSnap.Monitors, rbdSnap.RadosNamespace, cr)
	if err != nil {
		util.ErrorLog(ctx, "failed to connect to cluster: %v", err)
		return ready, cloneRbd, err
	}
	defer j.Destroy()

	err = j.StoreImageID(ctx, rbdSnap.JournalPool, rbdSnap.ReservedID, cloneRbd.ImageID)
	if err != nil {
		util.ErrorLog(ctx, "failed to reserve volume id: %v", err)
		return ready, cloneRbd, err
	}

	err = cloneRbd.flattenRbdImage(ctx, cr, false, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
	if err != nil {
		if errors.Is(err, ErrFlattenInProgress) {
			return ready, cloneRbd, nil
		}
		return ready, cloneRbd, err
	}
	ready = true
	return ready, cloneRbd, nil
}

// DeleteSnapshot deletes the snapshot in backend and removes the
// snapshot metadata from store.
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		util.ErrorLog(ctx, "invalid delete snapshot req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	snapshotID := req.GetSnapshotId()
	if snapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID cannot be empty")
	}

	if acquired := cs.SnapshotLocks.TryAcquire(snapshotID); !acquired {
		util.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, snapshotID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, snapshotID)
	}
	defer cs.SnapshotLocks.Release(snapshotID)

	// lock out snapshotID for restore operation
	if err = cs.OperationLocks.GetDeleteLock(snapshotID); err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseDeleteLock(snapshotID)

	rbdSnap := &rbdSnapshot{}
	if err = genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr); err != nil {
		// if error is ErrPoolNotFound, the pool is already deleted we dont
		// need to worry about deleting snapshot or omap data, return success
		if errors.Is(err, util.ErrPoolNotFound) {
			util.WarningLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (snap and snapOMap are garbage collected already), hence return
		// success as deletion is complete
		if errors.Is(err, util.ErrKeyNotFound) {
			return &csi.DeleteSnapshotResponse{}, nil
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	// safeguard against parallel create or delete requests against the same
	// name
	if acquired := cs.SnapshotLocks.TryAcquire(rbdSnap.RequestName); !acquired {
		util.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, rbdSnap.RequestName)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdSnap.RequestName)
	}
	defer cs.SnapshotLocks.Release(rbdSnap.RequestName)

	// Deleting snapshot and cloned volume
	util.DebugLog(ctx, "deleting cloned rbd volume %s", rbdSnap.RbdSnapName)

	rbdVol := generateVolFromSnap(rbdSnap)

	err = rbdVol.Connect(cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer rbdVol.Destroy()

	err = rbdVol.getImageInfo()
	if err != nil {
		if !errors.Is(err, ErrImageNotFound) {
			util.ErrorLog(ctx, "failed to delete rbd image: %s/%s with error: %v", rbdVol.Pool, rbdVol.VolName, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		rbdVol.ImageID = rbdSnap.ImageID
		// update parent name to delete the snapshot
		rbdSnap.RbdImageName = rbdVol.RbdImageName
		err = cleanUpSnapshot(ctx, rbdVol, rbdSnap, rbdVol, cr)
		if err != nil {
			util.ErrorLog(ctx, "failed to delete image: %v", err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	err = undoSnapReservation(ctx, rbdSnap, cr)
	if err != nil {
		util.ErrorLog(ctx, "failed to remove reservation for snapname (%s) with backing snap (%s) on image (%s) (%s)",
			rbdSnap.RequestName, rbdSnap.RbdSnapName, rbdSnap.RbdImageName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

// ControllerExpandVolume expand RBD Volumes on demand based on resizer request.
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		util.ErrorLog(ctx, "invalid expand volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	volID := req.GetVolumeId()
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID cannot be empty")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "capacityRange cannot be empty")
	}

	nodeExpansion := false
	// Get the nodeexpansion flag set based on the volume mode
	if req.GetVolumeCapability().GetBlock() == nil {
		nodeExpansion = true
	}

	// lock out parallel requests against the same volume ID
	if acquired := cs.VolumeLocks.TryAcquire(volID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer cs.VolumeLocks.Release(volID)

	// lock out volumeID for clone and delete operation
	if err := cs.OperationLocks.GetExpandLock(volID); err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseExpandLock(volID)

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	var rbdVol = &rbdVolume{}
	rbdVol, err = genVolFromVolID(ctx, volID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		// nolint:gocritic // this ifElseChain can not be rewritten to a switch statement
		if errors.Is(err, ErrImageNotFound) {
			err = status.Errorf(codes.NotFound, "volume ID %s not found", volID)
		} else if errors.Is(err, util.ErrPoolNotFound) {
			util.ErrorLog(ctx, "failed to get backend volume for %s: %v", volID, err)
			err = status.Errorf(codes.NotFound, err.Error())
		} else {
			err = status.Errorf(codes.Internal, err.Error())
		}
		return nil, err
	}

	if rbdVol.Encrypted {
		return nil, status.Errorf(codes.InvalidArgument, "encrypted volumes do not support resize (%s)",
			rbdVol)
	}

	// always round up the request size in bytes to the nearest MiB/GiB
	volSize := util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())

	// resize volume if required
	if rbdVol.VolSize < volSize {
		util.DebugLog(ctx, "rbd volume %s size is %v,resizing to %v", rbdVol, rbdVol.VolSize, volSize)
		err = rbdVol.resize(volSize)
		if err != nil {
			util.ErrorLog(ctx, "failed to resize rbd image: %s with error: %v", rbdVol, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         rbdVol.VolSize,
		NodeExpansionRequired: nodeExpansion,
	}, nil
}
