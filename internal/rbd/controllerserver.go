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
	"fmt"
	"strconv"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

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
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		log.ErrorLog(ctx, "invalid create volume req: %v", protosanitizer.StripSecrets(req))

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

func (cs *ControllerServer) parseVolCreateRequest(
	ctx context.Context,
	req *csi.CreateVolumeRequest) (*rbdVolume, error) {
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
		return nil, status.Error(
			codes.InvalidArgument,
			"multi node access modes are only supported on rbd `block` type volumes")
	}

	if imageFeatures, ok := req.GetParameters()["imageFeatures"]; checkImageFeatures(imageFeatures, ok, true) {
		return nil, status.Error(codes.InvalidArgument, "missing required parameter imageFeatures")
	}

	// if it's NOT SINGLE_NODE_WRITER and it's BLOCK we'll set the parameter to ignore the in-use checks
	rbdVol, err := genVolFromVolumeOptions(
		ctx,
		req.GetParameters(), req.GetSecrets(),
		(isMultiNode && isBlock), false)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rbdVol.ThickProvision = isThickProvisionRequest(req.GetParameters())

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

func buildCreateVolumeResponse(req *csi.CreateVolumeRequest, rbdVol *rbdVolume) *csi.CreateVolumeResponse {
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

	return &csi.CreateVolumeResponse{Volume: volume}
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
			return status.Errorf(
				codes.InvalidArgument,
				"size mismatches, requested volume size %d and source snapshot size %d",
				rbdVol.VolSize,
				vol.VolSize)
		}
	}
	if parentVol != nil {
		if rbdVol.VolSize != parentVol.VolSize {
			return status.Errorf(
				codes.InvalidArgument,
				"size mismatches, requested volume size %d and source volume size %d",
				rbdVol.VolSize,
				parentVol.VolSize)
		}
	}

	return nil
}

func checkValidCreateVolumeRequest(rbdVol, parentVol *rbdVolume, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	err := validateRequestedVolumeSize(rbdVol, parentVol, rbdSnap, cr)
	if err != nil {
		return err
	}

	switch {
	case rbdSnap != nil:
		err = rbdSnap.isCompatibleEncryption(&rbdVol.rbdImage)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "cannot restore from snapshot %s: %s", rbdSnap, err.Error())
		}

		err = rbdSnap.isCompatibleThickProvision(rbdVol)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "cannot restore from snapshot %s: %s", rbdSnap, err.Error())
		}
	case parentVol != nil:
		err = parentVol.isCompatibleEncryption(&rbdVol.rbdImage)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "cannot clone from volume %s: %s", parentVol, err.Error())
		}

		err = parentVol.isCompatibleThickProvision(rbdVol)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "cannot clone from volume %s: %s", parentVol, err.Error())
		}
	}

	return nil
}

// CreateVolume creates the volume in backend.
func (cs *ControllerServer) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
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
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, req.GetName())

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.VolumeLocks.Release(req.GetName())

	err = rbdVol.Connect(cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to connect to volume %v: %v", rbdVol.RbdImageName, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	parentVol, rbdSnap, err := checkContentSource(ctx, req, cr)
	if err != nil {
		return nil, err
	}

	found, err := rbdVol.Exists(ctx, parentVol)
	if err != nil {
		return nil, getGRPCErrorForCreateVolume(err)
	} else if found {
		return cs.repairExistingVolume(ctx, req, cr, rbdVol, parentVol, rbdSnap)
	}

	err = checkValidCreateVolumeRequest(rbdVol, parentVol, rbdSnap, cr)
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
					log.WarningLog(ctx, "failed undoing reservation of volume: %s (%s)", req.GetName(), errDefer)
				}
			}
		}
	}()

	err = cs.createBackingImage(ctx, cr, req.GetSecrets(), rbdVol, parentVol, rbdSnap)
	if err != nil {
		if errors.Is(err, ErrFlattenInProgress) {
			return nil, status.Error(codes.Aborted, err.Error())
		}

		return nil, err
	}

	return buildCreateVolumeResponse(req, rbdVol), nil
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

// repairExistingVolume checks the existing volume or snapshot and makes sure
// that the state is corrected to what was requested. It is needed to call this
// when the process of creating a volume was interrupted.
func (cs *ControllerServer) repairExistingVolume(ctx context.Context, req *csi.CreateVolumeRequest,
	cr *util.Credentials, rbdVol, parentVol *rbdVolume, rbdSnap *rbdSnapshot) (*csi.CreateVolumeResponse, error) {
	vcs := req.GetVolumeContentSource()

	switch {
	// normal CreateVolume without VolumeContentSource
	case vcs == nil:
		// continue/restart allocating the volume in case it
		// should be thick-provisioned
		if isThickProvisionRequest(req.GetParameters()) {
			err := rbdVol.RepairThickProvision()
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

	// rbdVol is a restore from snapshot, rbdSnap is passed
	case vcs.GetSnapshot() != nil:
		// When restoring of a thick-provisioned volume was happening,
		// the image should be marked as thick-provisioned, unless it
		// was aborted in flight. In order to restart the
		// thick-restoring, delete the volume and let the caller retry
		// from the start.
		if isThickProvisionRequest(req.GetParameters()) {
			thick, err := rbdVol.isThickProvisioned()
			if err != nil {
				return nil, status.Errorf(
					codes.Aborted,
					"failed to verify thick-provisioned volume %q: %s",
					rbdVol,
					err)
			} else if !thick {
				err = deleteImage(ctx, rbdVol, cr)
				if err != nil {
					return nil, status.Errorf(codes.Aborted, "failed to remove partially cloned volume %q: %s", rbdVol, err)
				}
				err = undoVolReservation(ctx, rbdVol, cr)
				if err != nil {
					return nil, status.Errorf(codes.Aborted, "failed to remove volume %q from journal: %s", rbdVol, err)
				}

				return nil, status.Errorf(
					codes.Aborted,
					"restoring thick-provisioned volume %q has been interrupted, please retry", rbdVol)
			}
		}
		// restore from snapshot implies rbdSnap != nil
		// check if image depth is reached limit and requires flatten
		err := checkFlatten(ctx, rbdVol, cr)
		if err != nil {
			return nil, err
		}

		err = rbdSnap.repairEncryptionConfig(&rbdVol.rbdImage)
		if err != nil {
			return nil, err
		}

	// rbdVol is a clone from parentVol
	case vcs.GetVolume() != nil:
		// When cloning into a thick-provisioned volume was happening,
		// the image should be marked as thick-provisioned, unless it
		// was aborted in flight. In order to restart the
		// thick-cloning, delete the volume and undo the reservation in
		// the journal to let the caller retry from the start.
		if isThickProvisionRequest(req.GetParameters()) {
			thick, err := rbdVol.isThickProvisioned()
			if err != nil {
				return nil, status.Errorf(
					codes.Internal,
					"failed to verify thick-provisioned volume %q: %s",
					rbdVol,
					err)
			} else if !thick {
				err = cleanUpSnapshot(ctx, parentVol, rbdSnap, rbdVol, cr)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "failed to remove partially cloned volume %q: %s", rbdVol, err)
				}
				err = undoVolReservation(ctx, rbdVol, cr)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "failed to remove volume %q from journal: %s", rbdVol, err)
				}

				return nil, status.Errorf(
					codes.Internal,
					"cloning thick-provisioned volume %q has been interrupted, please retry", rbdVol)
			}
		}
	}

	return buildCreateVolumeResponse(req, rbdVol), nil
}

// check snapshots on the rbd image, as we have limit from krbd that an image
// cannot have more than 510 snapshot at a given point of time. If the
// snapshots are more than the `maxSnapshotsOnImage` Add a task to flatten all
// the temporary cloned images and return ABORT error message. If the snapshots
// are more than the `minSnapshotOnImage` Add a task to flatten all the
// temporary cloned images.
func flattenTemporaryClonedImages(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	if rbdVol.ThickProvision {
		// thick-provisioned images do not need flattening
		return nil
	}

	snaps, err := rbdVol.listSnapshots()
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			return status.Error(codes.InvalidArgument, err.Error())
		}

		return status.Error(codes.Internal, err.Error())
	}

	if len(snaps) > int(maxSnapshotsOnImage) {
		log.DebugLog(
			ctx,
			"snapshots count %d on image: %s reached configured hard limit %d",
			len(snaps),
			rbdVol,
			maxSnapshotsOnImage)
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
		log.DebugLog(
			ctx,
			"snapshots count %d on image: %s reached configured soft limit %d",
			len(snaps),
			rbdVol,
			minSnapshotsOnImageToStartFlatten)
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

// checkFlatten ensures that the image chain depth is not reached
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
			log.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v", rbdVol, errDefer)

			return status.Error(codes.Internal, err.Error())
		}
		errDefer := undoVolReservation(ctx, rbdVol, cr)
		if errDefer != nil {
			log.WarningLog(ctx, "failed undoing reservation of volume: %s (%s)", rbdVol.RequestName, errDefer)
		}

		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (cs *ControllerServer) createVolumeFromSnapshot(
	ctx context.Context,
	cr *util.Credentials,
	secrets map[string]string,
	rbdVol *rbdVolume,
	snapshotID string) error {
	rbdSnap := &rbdSnapshot{}
	if acquired := cs.SnapshotLocks.TryAcquire(snapshotID); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, snapshotID)

		return status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, snapshotID)
	}
	defer cs.SnapshotLocks.Release(snapshotID)

	err := genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr, secrets)
	if err != nil {
		if errors.Is(err, util.ErrPoolNotFound) {
			log.ErrorLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)

			return status.Error(codes.InvalidArgument, err.Error())
		}

		return status.Error(codes.Internal, err.Error())
	}

	// update parent name(rbd image name in snapshot)
	rbdSnap.RbdImageName = rbdSnap.RbdSnapName
	parentVol := generateVolFromSnap(rbdSnap)
	// as we are operating on single cluster reuse the connection
	parentVol.conn = rbdVol.conn.Copy()

	if rbdVol.ThickProvision {
		err = parentVol.DeepCopy(rbdVol)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to deep copy %q into %q: %v", parentVol, rbdVol, err)
		}
		err = rbdVol.setThickProvisioned()
		if err != nil {
			return status.Errorf(codes.Internal, "failed to mark %q thick-provisioned: %s", rbdVol, err)
		}
		err = parentVol.copyEncryptionConfig(&rbdVol.rbdImage, true)
		if err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
	} else {
		// create clone image and delete snapshot
		err = rbdVol.cloneRbdImageFromSnapshot(ctx, rbdSnap, parentVol)
		if err != nil {
			log.ErrorLog(ctx, "failed to clone rbd image %s from snapshot %s: %v", rbdVol, rbdSnap, err)

			return err
		}
	}

	log.DebugLog(ctx, "create volume %s from snapshot %s", rbdVol.RequestName, rbdSnap.RbdSnapName)

	return nil
}

func (cs *ControllerServer) createBackingImage(
	ctx context.Context,
	cr *util.Credentials,
	secrets map[string]string,
	rbdVol, parentVol *rbdVolume,
	rbdSnap *rbdSnapshot) error {
	var err error

	j, err := volJournal.Connect(rbdVol.Monitors, rbdVol.RadosNamespace, cr)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer j.Destroy()

	switch {
	case rbdSnap != nil:
		if err = cs.OperationLocks.GetRestoreLock(rbdSnap.VolID); err != nil {
			log.ErrorLog(ctx, err.Error())

			return status.Error(codes.Aborted, err.Error())
		}
		defer cs.OperationLocks.ReleaseRestoreLock(rbdSnap.VolID)

		err = cs.createVolumeFromSnapshot(ctx, cr, secrets, rbdVol, rbdSnap.VolID)
		if err != nil {
			return err
		}
		log.DebugLog(ctx, "created volume %s from snapshot %s", rbdVol.RequestName, rbdSnap.RbdSnapName)
	case parentVol != nil:
		if err = cs.OperationLocks.GetCloneLock(parentVol.VolID); err != nil {
			log.ErrorLog(ctx, err.Error())

			return status.Error(codes.Aborted, err.Error())
		}
		defer cs.OperationLocks.ReleaseCloneLock(parentVol.VolID)

		return rbdVol.createCloneFromImage(ctx, parentVol)
	default:
		err = createImage(ctx, rbdVol, cr)
		if err != nil {
			log.ErrorLog(ctx, "failed to create volume: %v", err)

			return status.Error(codes.Internal, err.Error())
		}
	}

	log.DebugLog(ctx, "created volume %s backed by image %s", rbdVol.RequestName, rbdVol.RbdImageName)

	defer func() {
		if err != nil {
			if !errors.Is(err, ErrFlattenInProgress) {
				if deleteErr := deleteImage(ctx, rbdVol, cr); deleteErr != nil {
					log.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v", rbdVol, deleteErr)
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
			log.ErrorLog(ctx, "failed to flatten image %s: %v", rbdVol, err)

			return err
		}
	}

	return nil
}

func checkContentSource(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
	cr *util.Credentials) (*rbdVolume, *rbdSnapshot, error) {
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
		if err := genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr, req.GetSecrets()); err != nil {
			log.ErrorLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)
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
		rbdvol, err := genVolFromVolID(ctx, volID, cr, req.GetSecrets())
		if err != nil {
			log.ErrorLog(ctx, "failed to get backend image for %s: %v", volID, err)
			if !errors.Is(err, ErrImageNotFound) {
				return nil, nil, status.Error(codes.Internal, err.Error())
			}

			return nil, nil, status.Errorf(codes.NotFound, "%s image does not exist", volID)
		}

		return rbdvol, nil, nil
	}

	return nil, nil, status.Errorf(codes.InvalidArgument, "not a proper volume source")
}

// checkErrAndUndoReserve work on error from genVolFromVolID() and undo omap reserve.
// Even-though volumeID is part of rbdVolume struct we take it as an arg here, the main reason
// being, the volume id is getting filled from `genVolFromVolID->generateVolumeFromVolumeID` call path,
// and this function is operating on the error case/scenario of above call chain, so we can not rely
// on the 'rbdvol->rbdimage->voldID' field.

func (cs *ControllerServer) checkErrAndUndoReserve(
	ctx context.Context,
	err error,
	volumeID string,
	rbdVol *rbdVolume, cr *util.Credentials) (*csi.DeleteVolumeResponse, error) {
	if errors.Is(err, util.ErrPoolNotFound) {
		log.WarningLog(ctx, "failed to get backend volume for %s: %v", volumeID, err)

		return &csi.DeleteVolumeResponse{}, nil
	}

	// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
	// or partially complete (image and imageOMap are garbage collected already), hence return
	// success as deletion is complete
	if errors.Is(err, util.ErrKeyNotFound) {
		log.WarningLog(ctx, "failed to volume options for %s: %v", volumeID, err)

		return &csi.DeleteVolumeResponse{}, nil
	}

	if errors.Is(err, ErrImageNotFound) {
		err = rbdVol.ensureImageCleanup(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		// All errors other than ErrImageNotFound should return an error back to the caller
		return nil, status.Error(codes.Internal, err.Error())
	}

	// If error is ErrImageNotFound then we failed to find the image, but found the imageOMap
	// to lead us to the image, hence the imageOMap needs to be garbage collected, by calling
	// unreserve for the same
	if acquired := cs.VolumeLocks.TryAcquire(rbdVol.RequestName); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
	}
	defer cs.VolumeLocks.Release(rbdVol.RequestName)

	if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store.
func (cs *ControllerServer) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		log.ErrorLog(ctx, "invalid delete volume req: %v", protosanitizer.StripSecrets(req))

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
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.VolumeLocks.Release(volumeID)

	// lock out volumeID for clone and expand operation
	if err = cs.OperationLocks.GetDeleteLock(volumeID); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseDeleteLock(volumeID)

	if isMigrationVolID(volumeID) {
		log.DebugLog(ctx, "migration volume ID : %s", volumeID)
		err = parseAndDeleteMigratedVolume(ctx, volumeID, cr)
		if err != nil && !errors.Is(err, ErrImageNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.DeleteVolumeResponse{}, nil
	}

	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		return cs.checkErrAndUndoReserve(ctx, err, volumeID, rbdVol, cr)
	}

	// lock out parallel create requests against the same volume name as we
	// clean up the image and associated omaps for the same
	if acquired := cs.VolumeLocks.TryAcquire(rbdVol.RequestName); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
	}
	defer cs.VolumeLocks.Release(rbdVol.RequestName)

	return cleanupRBDImage(ctx, rbdVol, cr)
}

// cleanupRBDImage removes the rbd image and OMAP metadata associated with it.
func cleanupRBDImage(ctx context.Context,
	rbdVol *rbdVolume, cr *util.Credentials) (*csi.DeleteVolumeResponse, error) {
	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}
	// Cleanup only omap data if the following condition is met
	// Mirroring is enabled on the image
	// Local image is secondary
	// Local image is in up+replaying state
	if mirroringInfo.State == librbd.MirrorImageEnabled && !mirroringInfo.Primary {
		// If the image is in a secondary state and its up+replaying means its
		// an healthy secondary and the image is primary somewhere in the
		// remote cluster and the local image is getting replayed. Delete the
		// OMAP data generated as we cannot delete the secondary image. When
		// the image on the primary cluster gets deleted/mirroring disabled,
		// the image on all the remote (secondary) clusters will get
		// auto-deleted. This helps in garbage collecting the OMAP, PVC and PV
		// objects after failback operation.
		localStatus, rErr := rbdVol.getLocalState()
		if rErr != nil {
			return nil, status.Error(codes.Internal, rErr.Error())
		}
		if localStatus.Up && localStatus.State == librbd.MirrorImageStatusStateReplaying {
			if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
				log.ErrorLog(ctx, "failed to remove reservation for volume (%s) with backing image (%s) (%s)",
					rbdVol.RequestName, rbdVol.RbdImageName, err)

				return nil, status.Error(codes.Internal, err.Error())
			}

			return &csi.DeleteVolumeResponse{}, nil
		}
		log.ErrorLog(ctx,
			"secondary image status is up=%t and state=%s",
			localStatus.Up,
			localStatus.State)
	}

	inUse, err := rbdVol.isInUse()
	if err != nil {
		log.ErrorLog(ctx, "failed getting information for image (%s): (%s)", rbdVol, err)

		return nil, status.Error(codes.Internal, err.Error())
	}
	if inUse {
		log.ErrorLog(ctx, "rbd %s is still being used", rbdVol)

		return nil, status.Errorf(codes.Internal, "rbd %s is still being used", rbdVol.RbdImageName)
	}

	// delete the temporary rbd image created as part of volume clone during
	// create volume
	tempClone := rbdVol.generateTempClone()
	err = deleteImage(ctx, tempClone, cr)
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			err = tempClone.ensureImageCleanup(ctx)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			// return error if it is not ErrImageNotFound
			log.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v",
				tempClone, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Deleting rbd image
	log.DebugLog(ctx, "deleting image %s", rbdVol.RbdImageName)
	if err = deleteImage(ctx, rbdVol, cr); err != nil {
		log.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v",
			rbdVol, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
		log.ErrorLog(ctx, "failed to remove reservation for volume (%s) with backing image (%s) (%s)",
			rbdVol.RequestName, rbdVol.RbdImageName, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
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

// CreateSnapshot creates the snapshot in backend and stores metadata in store.
func (cs *ControllerServer) CreateSnapshot(
	ctx context.Context,
	req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if err := cs.validateSnapshotReq(ctx, req); err != nil {
		return nil, err
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	// Fetch source volume information
	rbdVol, err := genVolFromVolID(ctx, req.GetSourceVolumeId(), cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "source Volume ID %s not found", req.GetSourceVolumeId())
		case errors.Is(err, util.ErrPoolNotFound):
			log.ErrorLog(ctx, "failed to get backend volume for %s: %v", req.GetSourceVolumeId(), err)
			err = status.Errorf(codes.NotFound, err.Error())
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}

	// Check if source volume was created with required image features for snaps
	if !rbdVol.hasSnapshotFeature() {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"volume(%s) has not snapshot feature(layering)",
			req.GetSourceVolumeId())
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
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, req.GetName())

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.SnapshotLocks.Release(req.GetName())

	// Take lock on parent rbd image
	if err = cs.OperationLocks.GetSnapshotCreateLock(rbdSnap.SourceVolumeID); err != nil {
		log.ErrorLog(ctx, err.Error())

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
		return cloneFromSnapshot(ctx, rbdVol, rbdSnap, cr)
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
		if err != nil && !errors.Is(err, ErrFlattenInProgress) {
			errDefer := undoSnapReservation(ctx, rbdSnap, cr)
			if errDefer != nil {
				log.WarningLog(ctx, "failed undoing reservation of snapshot: %s %v", req.GetName(), errDefer)
			}
		}
	}()

	vol, err := cs.doSnapshotClone(ctx, rbdVol, rbdSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      vol.VolSize,
			SnapshotId:     vol.VolID,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   vol.CreatedAt,
			ReadyToUse:     true,
		},
	}, nil
}

// cloneFromSnapshot is a helper for CreateSnapshot that continues creating an
// RBD image from an RBD snapshot if the process was interrupted at one point.
func cloneFromSnapshot(
	ctx context.Context,
	rbdVol *rbdVolume,
	rbdSnap *rbdSnapshot,
	cr *util.Credentials) (*csi.CreateSnapshotResponse, error) {
	vol := generateVolFromSnap(rbdSnap)
	err := vol.Connect(cr)
	if err != nil {
		uErr := undoSnapshotCloning(ctx, rbdVol, rbdSnap, vol, cr)
		if uErr != nil {
			log.WarningLog(ctx, "failed undoing reservation of snapshot: %s %v", rbdSnap.RequestName, uErr)
		}

		return nil, status.Errorf(codes.Internal, err.Error())
	}
	defer vol.Destroy()

	if rbdVol.isEncrypted() {
		err = rbdVol.copyEncryptionConfig(&vol.rbdImage, false)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// The clone image created during CreateSnapshot has to be marked as thick.
	// As snapshot and volume both are independent we cannot depend on the
	// parent volume of the clone to check thick provision during CreateVolume
	// from snapshot operation because the parent volume can be deleted anytime
	// after snapshot is created.
	// TODO: copy thick provision config
	thick, err := rbdVol.isThickProvisioned()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed checking thick-provisioning of %q: %s", rbdVol, err)
	}

	if thick {
		// check the thick metadata is already set on the clone image.
		thick, err = vol.isThickProvisioned()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed checking thick-provisioning of %q: %s", vol, err)
		}
		if !thick {
			err = vol.setThickProvisioned()
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed mark %q thick-provisioned: %s", vol, err)
			}
		}
	}

	err = vol.flattenRbdImage(ctx, cr, false, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
	if errors.Is(err, ErrFlattenInProgress) {
		// if flattening is in progress, return error and do not cleanup
		return nil, status.Errorf(codes.Internal, err.Error())
	} else if err != nil {
		uErr := undoSnapshotCloning(ctx, rbdVol, rbdSnap, vol, cr)
		if uErr != nil {
			log.WarningLog(ctx, "failed undoing reservation of snapshot: %s %v", rbdSnap.RequestName, uErr)
		}

		return nil, status.Errorf(codes.Internal, err.Error())
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      rbdSnap.SizeBytes,
			SnapshotId:     rbdSnap.VolID,
			SourceVolumeId: rbdSnap.SourceVolumeID,
			CreationTime:   rbdSnap.CreatedAt,
			ReadyToUse:     true,
		},
	}, nil
}

func (cs *ControllerServer) validateSnapshotReq(ctx context.Context, req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		log.ErrorLog(ctx, "invalid create snapshot req: %v", protosanitizer.StripSecrets(req))

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
	if value, ok := options["pool"]; ok && value == "" {
		return status.Error(codes.InvalidArgument, "empty pool name in which rbd image will be created")
	}

	return nil
}

func (cs *ControllerServer) doSnapshotClone(
	ctx context.Context,
	parentVol *rbdVolume,
	rbdSnap *rbdSnapshot,
	cr *util.Credentials) (*rbdVolume, error) {
	// generate cloned volume details from snapshot
	cloneRbd := generateVolFromSnap(rbdSnap)
	defer cloneRbd.Destroy()
	// add image feature for cloneRbd
	f := []string{librbd.FeatureNameLayering, librbd.FeatureNameDeepFlatten}
	cloneRbd.imageFeatureSet = librbd.FeatureSetFromNames(f)

	err := cloneRbd.Connect(cr)
	if err != nil {
		return cloneRbd, err
	}

	err = createRBDClone(ctx, parentVol, cloneRbd, rbdSnap, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to create snapshot: %v", err)

		return cloneRbd, err
	}

	defer func() {
		if err != nil {
			if !errors.Is(err, ErrFlattenInProgress) {
				// cleanup clone and snapshot
				errCleanUp := cleanUpSnapshot(ctx, cloneRbd, rbdSnap, cloneRbd, cr)
				if errCleanUp != nil {
					log.ErrorLog(ctx, "failed to cleanup snapshot and clone: %v", errCleanUp)
				}
			}
		}
	}()

	if parentVol.isEncrypted() {
		cryptErr := parentVol.copyEncryptionConfig(&cloneRbd.rbdImage, false)
		if cryptErr != nil {
			log.WarningLog(ctx, "failed copy encryption "+
				"config for %q: %v", cloneRbd, cryptErr)

			return nil, err
		}
	}

	// The clone image created during CreateSnapshot has to be marked as thick.
	// As snapshot and volume both are independent we cannot depend on the
	// parent volume of the clone to check thick provision during CreateVolume
	// from snapshot operation because the parent volume can be deleted anytime
	// after snapshot is created.
	thick, err := parentVol.isThickProvisioned()
	if err != nil {
		return nil, fmt.Errorf("failed checking thick-provisioning of %q: %w", parentVol, err)
	}

	if thick {
		err = cloneRbd.setThickProvisioned()
		if err != nil {
			return nil, fmt.Errorf("failed mark %q thick-provisioned: %w", cloneRbd, err)
		}
	} else {
		err = cloneRbd.createSnapshot(ctx, rbdSnap)
		if err != nil {
			// update rbd image name for logging
			rbdSnap.RbdImageName = cloneRbd.RbdImageName
			log.ErrorLog(ctx, "failed to create snapshot %s: %v", rbdSnap, err)

			return cloneRbd, err
		}
	}

	err = cloneRbd.getImageID()
	if err != nil {
		log.ErrorLog(ctx, "failed to get image id: %v", err)

		return cloneRbd, err
	}
	// save image ID
	j, err := snapJournal.Connect(rbdSnap.Monitors, rbdSnap.RadosNamespace, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to connect to cluster: %v", err)

		return cloneRbd, err
	}
	defer j.Destroy()

	err = j.StoreImageID(ctx, rbdSnap.JournalPool, rbdSnap.ReservedID, cloneRbd.ImageID)
	if err != nil {
		log.ErrorLog(ctx, "failed to reserve volume id: %v", err)

		return cloneRbd, err
	}

	err = cloneRbd.flattenRbdImage(ctx, cr, false, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
	if err != nil {
		return cloneRbd, err
	}

	return cloneRbd, nil
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
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, snapshotID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, snapshotID)
	}
	defer cs.SnapshotLocks.Release(snapshotID)

	// lock out snapshotID for restore operation
	if err = cs.OperationLocks.GetDeleteLock(snapshotID); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseDeleteLock(snapshotID)

	rbdSnap := &rbdSnapshot{}
	if err = genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr, req.GetSecrets()); err != nil {
		// if error is ErrPoolNotFound, the pool is already deleted we don't
		// need to worry about deleting snapshot or omap data, return success
		if errors.Is(err, util.ErrPoolNotFound) {
			log.WarningLog(ctx, "failed to get backend snapshot for %s: %v", snapshotID, err)

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
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, rbdSnap.RequestName)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdSnap.RequestName)
	}
	defer cs.SnapshotLocks.Release(rbdSnap.RequestName)

	// Deleting snapshot and cloned volume
	log.DebugLog(ctx, "deleting cloned rbd volume %s", rbdSnap.RbdSnapName)

	rbdVol := generateVolFromSnap(rbdSnap)

	err = rbdVol.Connect(cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer rbdVol.Destroy()

	err = rbdVol.getImageInfo()
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			err = rbdVol.ensureImageCleanup(ctx)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			log.ErrorLog(ctx, "failed to delete rbd image: %s/%s with error: %v", rbdVol.Pool, rbdVol.VolName, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		rbdVol.ImageID = rbdSnap.ImageID
		// update parent name to delete the snapshot
		rbdSnap.RbdImageName = rbdVol.RbdImageName
		err = cleanUpSnapshot(ctx, rbdVol, rbdSnap, rbdVol, cr)
		if err != nil {
			log.ErrorLog(ctx, "failed to delete image: %v", err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	err = undoSnapReservation(ctx, rbdSnap, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to remove reservation for snapname (%s) with backing snap (%s) on image (%s) (%s)",
			rbdSnap.RequestName, rbdSnap.RbdSnapName, rbdSnap.RbdImageName, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

// ControllerExpandVolume expand RBD Volumes on demand based on resizer request.
func (cs *ControllerServer) ControllerExpandVolume(
	ctx context.Context,
	req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		log.ErrorLog(ctx, "invalid expand volume req: %v", protosanitizer.StripSecrets(req))

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

	// lock out parallel requests against the same volume ID
	if acquired := cs.VolumeLocks.TryAcquire(volID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer cs.VolumeLocks.Release(volID)

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	rbdVol, err := genVolFromVolID(ctx, volID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume ID %s not found", volID)
		case errors.Is(err, util.ErrPoolNotFound):
			log.ErrorLog(ctx, "failed to get backend volume for %s: %v", volID, err)
			err = status.Errorf(codes.NotFound, err.Error())
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}

	// NodeExpansion is needed for PersistentVolumes with,
	// 1. Filesystem VolumeMode with & without Encryption and
	// 2. Block VolumeMode with Encryption
	// Hence set nodeExpansion flag based on VolumeMode and Encryption status
	nodeExpansion := true
	if req.GetVolumeCapability().GetBlock() != nil && !rbdVol.isEncrypted() {
		nodeExpansion = false
	}

	// lock out volumeID for clone and delete operation
	if err = cs.OperationLocks.GetExpandLock(volID); err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseExpandLock(volID)

	// always round up the request size in bytes to the nearest MiB/GiB
	volSize := util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())

	// resize volume if required
	if rbdVol.VolSize < volSize {
		log.DebugLog(ctx, "rbd volume %s size is %v,resizing to %v", rbdVol, rbdVol.VolSize, volSize)
		err = rbdVol.resize(volSize)
		if err != nil {
			log.ErrorLog(ctx, "failed to resize rbd image: %s with error: %v", rbdVol, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         rbdVol.VolSize,
		NodeExpansionRequired: nodeExpansion,
	}, nil
}

// isThickProvisionRequest returns true in case the request contains the
// `thickProvision` option set to `true`.
func isThickProvisionRequest(parameters map[string]string) bool {
	tp := "thickProvision"

	thick, ok := parameters[tp]
	if !ok || thick == "" {
		return false
	}

	thickBool, err := strconv.ParseBool(thick)
	if err != nil {
		return false
	}

	return thickBool
}
