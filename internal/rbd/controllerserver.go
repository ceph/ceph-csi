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
	"fmt"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	oneGB = 1073741824
)

// ControllerServer struct of rbd CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	MetadataStore util.CachePersister
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	VolumeLocks *util.VolumeLocks

	// A map storing all volumes with ongoing operations so that additional operations
	// for that same snapshot (as defined by SnapshotID/snapshot name) return an Aborted error
	SnapshotLocks *util.VolumeLocks
}

func (cs *ControllerServer) validateVolumeReq(ctx context.Context, req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Errorf(util.Log(ctx, "invalid create volume req: %v"), protosanitizer.StripSecrets(req))
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
	if value, ok := options["volumeNamePrefix"]; ok && value == "" {
		return status.Error(codes.InvalidArgument, "empty volume name prefix to provision volume from")
	}
	return nil
}

func (cs *ControllerServer) parseVolCreateRequest(ctx context.Context, req *csi.CreateVolumeRequest) (*rbdVolume, error) {
	// TODO (sbezverk) Last check for not exceeding total storage capacity

	isMultiNode := false
	isBlock := false
	for _, cap := range req.VolumeCapabilities {
		// RO modes need to be handled independently (ie right now even if access mode is RO, they'll be RW upon attach)
		if cap.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			isMultiNode = true
		}
		if cap.GetBlock() != nil {
			isBlock = true
		}
	}

	// We want to fail early if the user is trying to create a RWX on a non-block type device
	if isMultiNode && !isBlock {
		return nil, status.Error(codes.InvalidArgument, "multi node access modes are only supported on rbd `block` type volumes")
	}

	// if it's NOT SINGLE_NODE_WRITER and it's BLOCK we'll set the parameter to ignore the in-use checks
	rbdVol, err := genVolFromVolumeOptions(ctx, req.GetParameters(), req.GetSecrets(), (isMultiNode && isBlock), false)
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
			klog.Error(util.Log(ctx, err.Error()))
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	volumeContext := req.GetParameters()
	volumeContext["pool"] = rbdVol.Pool
	volumeContext["journalPool"] = rbdVol.JournalPool
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

// CreateVolume creates the volume in backend
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
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), req.GetName())
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.VolumeLocks.Release(req.GetName())

	err = rbdVol.Connect(cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to connect to volume %v: %v"), rbdVol.RbdImageName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	found, err := rbdVol.Exists(ctx)
	if err != nil {
		if _, ok := err.(ErrVolNameConflict); ok {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	if found {
		return buildCreateVolumeResponse(ctx, req, rbdVol)
	}

	rbdSnap, err := cs.checkSnapshotSource(ctx, req, cr)
	if err != nil {
		return nil, err
	}

	err = reserveVol(ctx, rbdVol, rbdSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoVolReservation(ctx, rbdVol, cr)
			if errDefer != nil {
				klog.Warningf(util.Log(ctx, "failed undoing reservation of volume: %s (%s)"), req.GetName(), errDefer)
			}
		}
	}()

	err = createBackingImage(ctx, cr, rbdVol, rbdSnap)
	if err != nil {
		return nil, err
	}

	if rbdVol.Encrypted {
		err = rbdVol.ensureEncryptionMetadataSet(rbdImageRequiresEncryption)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to save encryption status, deleting image %s: %s"),
				rbdVol, err)
			if deleteErr := deleteImage(ctx, rbdVol, cr); deleteErr != nil {
				klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s with error: %v"),
					rbdVol, deleteErr)
				return nil, deleteErr
			}
			return nil, err
		}
	}

	volumeContext := req.GetParameters()
	volumeContext["pool"] = rbdVol.Pool
	volumeContext["journalPool"] = rbdVol.JournalPool
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

func createBackingImage(ctx context.Context, cr *util.Credentials, rbdVol *rbdVolume, rbdSnap *rbdSnapshot) error {
	var err error

	if rbdSnap != nil {
		err = restoreSnapshot(ctx, rbdVol, rbdSnap, cr)
		if err != nil {
			return err
		}

		klog.V(4).Infof(util.Log(ctx, "created volume %s from snapshot %s"), rbdVol.RequestName, rbdSnap.RbdSnapName)
		return nil
	}

	err = createImage(ctx, rbdVol, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create volume: %v"), err)
		return status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infof(util.Log(ctx, "created volume %s backed by image %s"), rbdVol.RequestName, rbdVol.RbdImageName)

	return nil
}

func (cs *ControllerServer) checkSnapshotSource(ctx context.Context, req *csi.CreateVolumeRequest,
	cr *util.Credentials) (*rbdSnapshot, error) {
	if req.VolumeContentSource == nil {
		return nil, nil
	}

	snapshot := req.VolumeContentSource.GetSnapshot()
	if snapshot == nil {
		return nil, status.Error(codes.InvalidArgument, "volume Snapshot cannot be empty")
	}

	snapshotID := snapshot.GetSnapshotId()
	if snapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume Snapshot ID cannot be empty")
	}

	rbdSnap := &rbdSnapshot{}
	if err := genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr); err != nil {
		if _, ok := err.(ErrSnapNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if _, ok := err.(util.ErrPoolNotFound); ok {
			klog.Errorf(util.Log(ctx, "failed to get backend snapshot for %s: %v"), snapshotID, err)
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		return nil, status.Error(codes.InvalidArgument, "missing requested Snapshot ID")
	}

	return rbdSnap, nil
}

// DeleteLegacyVolume deletes a volume provisioned using version 1.0.0 of the plugin
func (cs *ControllerServer) DeleteLegacyVolume(ctx context.Context, req *csi.DeleteVolumeRequest, cr *util.Credentials) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()

	if cs.MetadataStore == nil {
		return nil, status.Errorf(codes.InvalidArgument, "missing metadata store configuration to"+
			" proceed with deleting legacy volume ID (%s)", volumeID)
	}

	if acquired := cs.VolumeLocks.TryAcquire(volumeID); !acquired {
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volumeID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.VolumeLocks.Release(volumeID)

	rbdVol := &rbdVolume{}
	defer rbdVol.Destroy()
	if err := cs.MetadataStore.Get(volumeID, rbdVol); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			klog.Warningf(util.Log(ctx, "metadata for legacy volume %s not found, assuming the volume to be already deleted (%v)"), volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	// Fill up Monitors
	if err := updateMons(rbdVol, nil, req.GetSecrets()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Update rbdImageName as the VolName when dealing with version 1 volumes
	rbdVol.RbdImageName = rbdVol.VolName

	klog.V(4).Infof(util.Log(ctx, "deleting legacy volume %s"), rbdVol.VolName)
	if err := deleteImage(ctx, rbdVol, cr); err != nil {
		// TODO: can we detect "already deleted" situations here and proceed?
		klog.Errorf(util.Log(ctx, "failed to delete legacy rbd image: %s/%s with error: %v"), rbdVol.Pool, rbdVol.VolName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := cs.MetadataStore.Delete(volumeID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Errorf(util.Log(ctx, "invalid delete volume req: %v"), protosanitizer.StripSecrets(req))
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
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volumeID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.VolumeLocks.Release(volumeID)

	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	if err != nil {
		switch err.(type) {
		case util.ErrPoolNotFound:
			klog.Warningf(util.Log(ctx, "failed to get backend volume for %s: %v"), volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil

		// If error is ErrInvalidVolID it could be a version 1.0.0 or lower volume, attempt
		// to process it as such
		case ErrInvalidVolID:
			if isLegacyVolumeID(volumeID) {
				klog.V(2).Infof(util.Log(ctx, "attempting deletion of potential legacy volume (%s)"), volumeID)
				return cs.DeleteLegacyVolume(ctx, req, cr)
			}

			// Consider unknown volumeID as a successfully deleted volume
			return &csi.DeleteVolumeResponse{}, nil

		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (image and imageOMap are garbage collected already), hence return
		// success as deletion is complete
		case util.ErrKeyNotFound:
			klog.Warningf(util.Log(ctx, "Failed to volume options for %s: %v"), volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil

		// All errors other than ErrImageNotFound should return an error back to the caller
		case ErrImageNotFound:
			break
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If error is ErrImageNotFound then we failed to find the image, but found the imageOMap
		// to lead us to the image, hence the imageOMap needs to be garbage collected, by calling
		// unreserve for the same
		if acquired := cs.VolumeLocks.TryAcquire(rbdVol.RequestName); !acquired {
			klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), rbdVol.RequestName)
			return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
		}
		defer cs.VolumeLocks.Release(rbdVol.RequestName)

		if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteVolumeResponse{}, nil
	}
	defer rbdVol.Destroy()

	// lock out parallel create requests against the same volume name as we
	// cleanup the image and associated omaps for the same
	if acquired := cs.VolumeLocks.TryAcquire(rbdVol.RequestName); !acquired {
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), rbdVol.RequestName)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdVol.RequestName)
	}
	defer cs.VolumeLocks.Release(rbdVol.RequestName)

	// Deleting rbd image
	klog.V(4).Infof(util.Log(ctx, "deleting image %s"), rbdVol.RbdImageName)
	if err = deleteImage(ctx, rbdVol, cr); err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s with error: %v"),
			rbdVol, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = undoVolReservation(ctx, rbdVol, cr); err != nil {
		klog.Errorf(util.Log(ctx, "failed to remove reservation for volume (%s) with backing image (%s) (%s)"),
			rbdVol.RequestName, rbdVol.RbdImageName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if rbdVol.Encrypted {
		if err = rbdVol.KMS.DeletePassphrase(rbdVol.VolID); err != nil {
			klog.Warningf(util.Log(ctx, "failed to clean the passphrase for volume %s: %s"), rbdVol.VolID, err)
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

	for _, cap := range req.VolumeCapabilities {
		if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
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
// nolint: gocyclo
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
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
	if err != nil {
		switch err.(type) {
		case ErrImageNotFound:
			err = status.Errorf(codes.NotFound, "source Volume ID %s not found", req.GetSourceVolumeId())
		case util.ErrPoolNotFound:
			klog.Errorf(util.Log(ctx, "failed to get backend volume for %s: %v"), req.GetSourceVolumeId(), err)
			err = status.Errorf(codes.NotFound, err.Error())
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}
		return nil, err
	}
	defer rbdVol.Destroy()

	// TODO: re-encrypt snapshot with a new passphrase
	if rbdVol.Encrypted {
		return nil, status.Errorf(codes.Unimplemented, "source Volume %s is encrypted, "+
			"snapshotting is not supported currently", rbdVol.VolID)
	}

	// Check if source volume was created with required image features for snaps
	if !hasSnapshotFeature(rbdVol.ImageFeatures) {
		return nil, status.Errorf(codes.InvalidArgument, "volume(%s) has not snapshot feature(layering)", req.GetSourceVolumeId())
	}

	// Create snap volume
	rbdSnap := genSnapFromOptions(ctx, rbdVol, req.GetParameters())
	rbdSnap.RbdImageName = rbdVol.RbdImageName
	rbdSnap.SizeBytes = rbdVol.VolSize
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.RequestName = req.GetName()

	if acquired := cs.SnapshotLocks.TryAcquire(req.GetName()); !acquired {
		klog.Errorf(util.Log(ctx, util.SnapshotOperationAlreadyExistsFmt), req.GetName())
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.SnapshotLocks.Release(req.GetName())

	// Need to check for already existing snapshot name, and if found
	// check for the requested source volume id and already allocated source volume id
	found, err := checkSnapExists(ctx, rbdSnap, cr)
	if err != nil {
		if _, ok := err.(util.ErrSnapNameConflict); ok {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}

		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if found {
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

	err = reserveSnap(ctx, rbdSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoSnapReservation(ctx, rbdSnap, cr)
			if errDefer != nil {
				klog.Warningf(util.Log(ctx, "failed undoing reservation of snapshot: %s %v"), req.GetName(), errDefer)
			}
		}
	}()

	err = cs.doSnapshot(ctx, rbdSnap, cr)
	if err != nil {
		return nil, err
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      rbdSnap.SizeBytes,
			SnapshotId:     rbdSnap.SnapID,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   rbdSnap.CreatedAt,
			ReadyToUse:     true,
		},
	}, nil
}

func (cs *ControllerServer) validateSnapshotReq(ctx context.Context, req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Errorf(util.Log(ctx, "invalid create snapshot req: %v"), protosanitizer.StripSecrets(req))
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

func (cs *ControllerServer) doSnapshot(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) (err error) {
	err = createSnapshot(ctx, rbdSnap, cr)
	// If snap creation fails, even due to snapname already used, fail, next attempt will get a new
	// uuid for use as the snap name
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create snapshot: %v"), err)
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := deleteSnapshot(ctx, rbdSnap, cr)
			if errDefer != nil {
				klog.Errorf(util.Log(ctx, "failed to delete snapshot: %v"), errDefer)
				err = fmt.Errorf("snapshot created but failed to delete snapshot due to"+
					" other failures: %v", err)
			}
			err = status.Error(codes.Internal, err.Error())
		}
	}()
	err = protectSnapshot(ctx, rbdSnap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to protect snapshot: %v"), err)
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := unprotectSnapshot(ctx, rbdSnap, cr)
			if errDefer != nil {
				klog.Errorf(util.Log(ctx, "failed to unprotect snapshot: %v"), errDefer)
				err = fmt.Errorf("snapshot created but failed to unprotect snapshot due to"+
					" other failures: %v", err)
			}
			err = status.Error(codes.Internal, err.Error())
		}
	}()

	err = getSnapshotMetadata(ctx, rbdSnap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to fetch snapshot metadata: %v"), err)
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// DeleteSnapshot deletes the snapshot in backend and removes the
// snapshot metadata from store
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Errorf(util.Log(ctx, "invalid delete snapshot req: %v"), protosanitizer.StripSecrets(req))
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
		klog.Errorf(util.Log(ctx, util.SnapshotOperationAlreadyExistsFmt), snapshotID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, snapshotID)
	}
	defer cs.SnapshotLocks.Release(snapshotID)

	rbdSnap := &rbdSnapshot{}
	if err = genSnapFromSnapID(ctx, rbdSnap, snapshotID, cr); err != nil {
		// if error is ErrPoolNotFound, the pool is already deleted we dont
		// need to worry about deleting snapshot or omap data, return success
		if _, ok := err.(util.ErrPoolNotFound); ok {
			klog.Warningf(util.Log(ctx, "failed to get backend snapshot for %s: %v"), snapshotID, err)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (snap and snapOMap are garbage collected already), hence return
		// success as deletion is complete
		if _, ok := err.(util.ErrKeyNotFound); ok {
			return &csi.DeleteSnapshotResponse{}, nil
		}

		// All errors other than ErrSnapNotFound should return an error back to the caller
		if _, ok := err.(ErrSnapNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Consider missing snap as already deleted, and proceed to remove the omap values,
		// safeguarding against parallel create or delete requests against the
		// same name.
		if acquired := cs.SnapshotLocks.TryAcquire(rbdSnap.RequestName); !acquired {
			klog.Errorf(util.Log(ctx, util.SnapshotOperationAlreadyExistsFmt), rbdSnap.RequestName)
			return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdSnap.RequestName)
		}
		defer cs.SnapshotLocks.Release(rbdSnap.RequestName)

		if err = undoSnapReservation(ctx, rbdSnap, cr); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteSnapshotResponse{}, nil
	}

	// safeguard against parallel create or delete requests against the same
	// name
	if acquired := cs.SnapshotLocks.TryAcquire(rbdSnap.RequestName); !acquired {
		klog.Errorf(util.Log(ctx, util.SnapshotOperationAlreadyExistsFmt), rbdSnap.RequestName)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, rbdSnap.RequestName)
	}
	defer cs.SnapshotLocks.Release(rbdSnap.RequestName)

	// Unprotect snapshot
	err = unprotectSnapshot(ctx, rbdSnap, cr)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to unprotect snapshot: %s with error: %v",
			rbdSnap, err)
	}

	// Deleting snapshot
	klog.V(4).Infof(util.Log(ctx, "deleting Snaphot %s"), rbdSnap)
	if err := deleteSnapshot(ctx, rbdSnap, cr); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to delete snapshot: %s with error: %v", rbdSnap, err)
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

// ControllerExpandVolume expand RBD Volumes on demand based on resizer request
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		klog.Errorf(util.Log(ctx, "invalid expand volume req: %v"), protosanitizer.StripSecrets(req))
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
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer cs.VolumeLocks.Release(volID)

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	rbdVol, err := genVolFromVolID(ctx, volID, cr, req.GetSecrets())
	if err != nil {
		switch err.(type) {
		case ErrImageNotFound:
			err = status.Errorf(codes.NotFound, "volume ID %s not found", volID)
		case util.ErrPoolNotFound:
			klog.Errorf(util.Log(ctx, "failed to get backend volume for %s: %v"), volID, err)
			err = status.Errorf(codes.NotFound, err.Error())
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}
		return nil, err
	}
	defer rbdVol.Destroy()

	if rbdVol.Encrypted {
		return nil, status.Errorf(codes.InvalidArgument, "encrypted volumes do not support resize (%s)",
			rbdVol)
	}

	// always round up the request size in bytes to the nearest MiB/GiB
	volSize := util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())

	// resize volume if required
	nodeExpansion := false
	if rbdVol.VolSize < volSize {
		klog.V(4).Infof(util.Log(ctx, "rbd volume %s size is %v,resizing to %v"), rbdVol, rbdVol.VolSize, volSize)
		rbdVol.VolSize = volSize
		nodeExpansion = true
		err = resizeRBDImage(rbdVol, cr)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to resize rbd image: %s with error: %v"), rbdVol, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         rbdVol.VolSize,
		NodeExpansionRequired: nodeExpansion,
	}, nil
}
