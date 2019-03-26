/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
}

func (cs *ControllerServer) validateVolumeReq(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.V(3).Infof("invalid create volume req: %v", protosanitizer.StripSecrets(req))
		return err
	}
	// Check sanity of request Name, Volume Capabilities
	if len(req.Name) == 0 {
		return status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		return status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}
	options := req.GetParameters()
	if value, ok := options["clusterID"]; !ok || len(value) == 0 {
		return status.Error(codes.InvalidArgument, "Missing or empty cluster ID to provision volume from")
	}
	if value, ok := options["pool"]; !ok || len(value) == 0 {
		return status.Error(codes.InvalidArgument, "Missing or empty pool name to provision volume from")
	}
	return nil
}

func (cs *ControllerServer) genRBDVolFromCreateRequest(req *csi.CreateVolumeRequest) (*rbdVolume, error) {
	// TODO (sbezverk) Last check for not exceeding total storage capacity

	isMultiNode := false
	isBlock := false
	for _, cap := range req.VolumeCapabilities {
		// RO modes need to be handled indepedently (ie right now even if access mode is RO, they'll be RW upon attach)
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
	rbdVol, err := genRBDVolFromVolumeOptions(req.GetParameters(), (isMultiNode && isBlock))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rbdVol.RequestName = req.GetName()

	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(oneGB)
	if req.GetCapacityRange() != nil {
		volSizeBytes = req.GetCapacityRange().GetRequiredBytes()
	}

	rbdVol.VolSize = util.RoundUpToMiB(volSizeBytes)

	// NOTE: rbdVol does not contain VolID and VolName populated, everything
	// else is populated post create request parsing
	return rbdVol, nil
}

// CreateVolume creates the volume in backend
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateVolumeReq(req); err != nil {
		return nil, err
	}

	volumeNameMutex.LockKey(req.GetName())
	defer func() {
		if err := volumeNameMutex.UnlockKey(req.GetName()); err != nil {
			klog.Warningf("failed to unlock mutex volume:%s %v", req.GetName(), err)
		}
	}()

	rbdVol, err := cs.genRBDVolFromCreateRequest(req)
	if err != nil {
		return nil, err
	}

	found, err := checkRBDVolExists(rbdVol, req.GetSecrets())
	if err != nil {
		if _, ok := err.(ErrVolNameConflict); ok {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	if found {
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      rbdVol.VolID,
				CapacityBytes: rbdVol.VolSize,
				VolumeContext: req.GetParameters(),
			},
		}, nil
	}

	err = reserveRBDVol(rbdVol, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			klog.Warningf("creation failed, undoing reservation of volume: %s", req.GetName())
			errDefer := unreserveRBDVol(rbdVol, req.GetSecrets())
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of volume: %s", req.GetName())
			}
		}
	}()

	err = cs.createRBDVol(rbdVol, req, int(rbdVol.VolSize))
	if err != nil {
		return nil, err
	}
	// store volume size in  bytes (snapshot and check existing volume needs volume
	// size in bytes)
	rbdVol.VolSize = rbdVol.VolSize * util.MiB

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      rbdVol.VolID,
			CapacityBytes: rbdVol.VolSize,
			VolumeContext: req.GetParameters(),
		},
	}, nil
}

func (cs *ControllerServer) createRBDVol(rbdVol *rbdVolume, req *csi.CreateVolumeRequest, volSizeMiB int) error {
	var err error

	// if VolumeContentSource is not nil, this request is for snapshot
	if req.VolumeContentSource != nil {
		if err = cs.checkSnapshot(req, rbdVol); err != nil {
			return err
		}
	} else {
		err = createRBDImage(rbdVol, volSizeMiB, rbdVol.AdminID, req.GetSecrets())
		if err != nil {
			klog.Warningf("failed to create volume: %v", err)
			return status.Error(codes.Internal, err.Error())
		}

		klog.V(4).Infof("create volume %s", rbdVol.VolName)
	}

	return nil
}
func (cs *ControllerServer) checkSnapshot(req *csi.CreateVolumeRequest, rbdVol *rbdVolume) error {
	snapshot := req.VolumeContentSource.GetSnapshot()
	if snapshot == nil {
		return status.Error(codes.InvalidArgument, "Volume Snapshot cannot be empty")
	}

	snapshotID := snapshot.GetSnapshotId()
	if len(snapshotID) == 0 {
		return status.Error(codes.InvalidArgument, "Volume Snapshot ID cannot be empty")
	}

	rbdSnap := &rbdSnapshot{}
	if err := genRBDSnapFromSnapID(rbdSnap, snapshotID, req.GetSecrets()); err != nil {
		if _, ok := err.(util.ErrSnapNotFound); !ok {
			return status.Error(codes.Internal, err.Error())
		}
		return status.Error(codes.InvalidArgument, "Missing requested Snapshot ID")
	}

	err := restoreSnapshot(rbdVol, rbdSnap, rbdVol.AdminID, req.GetSecrets())
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof("create volume %s from snapshot %s", req.GetName(), rbdSnap.SnapName)
	return nil
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Warningf("invalid delete volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}
	// For now the image get unconditionally deleted, but here retention policy can be checked
	volumeID := req.GetVolumeId()
	volumeIDMutex.LockKey(volumeID)
	defer func() {
		if err := volumeIDMutex.UnlockKey(volumeID); err != nil {
			klog.Warningf("failed to unlock mutex volume:%s %v", volumeID, err)
		}
	}()

	rbdVol := &rbdVolume{}
	if err := genRBDVolFromVolID(rbdVol, volumeID, req.GetSecrets()); err != nil {
		// If image key is missing, there can be no unreserve as request name is unknown, and also
		// means there is no image, hence return success
		if _, ok := err.(util.ErrKeyNotFound); !ok {
			return &csi.DeleteVolumeResponse{}, nil
		}

		if _, ok := err.(util.ErrImageNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If image is missing, then there was a key to point to it, it means we need to cleanup
		// the keys and return success
		volumeNameMutex.LockKey(rbdVol.RequestName)
		defer func() {
			if err := volumeNameMutex.UnlockKey(rbdVol.RequestName); err != nil {
				klog.Warningf("failed to unlock mutex volume:%s %v", rbdVol.RequestName, err)
			}
		}()

		if err := unreserveRBDVol(rbdVol, req.GetSecrets()); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteVolumeResponse{}, nil
	}

	// lock out parallel create requests against the same volume name as we
	// cleanup the image and associated omaps for the same
	volumeNameMutex.LockKey(rbdVol.RequestName)
	defer func() {
		if err := volumeNameMutex.UnlockKey(rbdVol.RequestName); err != nil {
			klog.Warningf("failed to unlock mutex volume:%s %v", rbdVol.RequestName, err)
		}
	}()

	// Deleting rbd image
	klog.V(4).Infof("deleting volume %s", rbdVol.VolName)
	if err := deleteRBDImage(rbdVol, rbdVol.AdminID, req.GetSecrets()); err != nil {
		// TODO: can we detect "already deleted" situations here and proceed?
		klog.V(3).Infof("failed to delete rbd image: %s/%s with error: %v", rbdVol.Pool, rbdVol.VolName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
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

// ControllerUnpublishVolume returns success response
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ControllerPublishVolume returns success response
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

// CreateSnapshot creates the snapshot in backend and stores metadata
// in store
// nolint: gocyclo
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	var rbdVol *rbdVolume

	if err := cs.validateSnapshotReq(req); err != nil {
		return nil, err
	}

	snapshotNameMutex.LockKey(req.GetName())
	defer func() {
		if err := snapshotNameMutex.UnlockKey(req.GetName()); err != nil {
			klog.Warningf("failed to unlock mutex snapshot:%s %v", req.GetName(), err)
		}
	}()

	// Fetch source volume information
	rbdVol = new(rbdVolume)
	err := genRBDVolFromVolID(rbdVol, req.GetSourceVolumeId(), req.GetSecrets())
	if err != nil {
		if _, ok := err.(util.ErrImageNotFound); ok {
			return nil, status.Errorf(codes.NotFound, "Source Volume ID %s cannot found", req.GetSourceVolumeId())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// Check if source volume was created with required image features for snaps
	if !hasSnapshotFeature(rbdVol.ImageFeatures) {
		return nil, status.Errorf(codes.InvalidArgument, "volume(%s) has not snapshot feature(layering)", req.GetSourceVolumeId())
	}

	// Create snap volume
	rbdSnap, err := genRBDSnapFromOptions(req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	rbdSnap.VolName = rbdVol.VolName
	rbdSnap.SizeBytes = rbdVol.VolSize
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.RequestName = req.GetName()

	// Need to check for already existing snapshot name, and if found
	// check for the requested source volume id and already allocated source volume id
	found, err := checkRBDSnapExists(rbdSnap, req.GetSecrets())
	if err != nil {
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

	err = reserveRBDSnap(rbdSnap, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			klog.Warningf("snapshot failed, undoing reservation of snap name: %s", req.GetName())
			errDefer := unreserveRBDSnap(rbdSnap, req.GetSecrets())
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of snapshot: %s", req.GetName())
			}
		}
	}()

	err = cs.doSnapshot(rbdSnap, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
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

func (cs *ControllerServer) validateSnapshotReq(req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Warningf("invalid create snapshot req: %v", protosanitizer.StripSecrets(req))
		return err
	}

	// Check sanity of request Snapshot Name, Source Volume Id
	if len(req.Name) == 0 {
		return status.Error(codes.InvalidArgument, "Snapshot Name cannot be empty")
	}
	if len(req.SourceVolumeId) == 0 {
		return status.Error(codes.InvalidArgument, "Source Volume ID cannot be empty")
	}
	options := req.GetParameters()
	if value, ok := options["clusterID"]; !ok || len(value) == 0 {
		return status.Error(codes.InvalidArgument, "Missing or empty cluster ID to snapshot volume from")
	}
	if value, ok := options["pool"]; !ok || len(value) == 0 {
		return status.Error(codes.InvalidArgument, "Missing or empty pool name to snapshot volume from")
	}
	return nil
}

func (cs *ControllerServer) doSnapshot(rbdSnap *rbdSnapshot, secret map[string]string) (err error) {
	err = createSnapshot(rbdSnap, rbdSnap.AdminID, secret)
	// If snap creation fails, even due to snapname already used, fail, next attempt will get a new
	// uuid for use as the snap name
	if err != nil {
		klog.Warningf("failed to create snapshot: %v", err)
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := deleteSnapshot(rbdSnap, rbdSnap.AdminID, secret)
			if errDefer != nil {
				err = fmt.Errorf("snapshot created but failed to delete snapshot due to"+
					" other failures: %v", err)
			}
			err = status.Error(codes.Internal, err.Error())
		}
	}()

	err = protectSnapshot(rbdSnap, rbdSnap.AdminID, secret)
	if err != nil {
		return errors.New("snapshot created but failed to protect snapshot")
	}
	defer func() {
		if err != nil {
			errDefer := unprotectSnapshot(rbdSnap, rbdSnap.AdminID, secret)
			if errDefer != nil {
				err = fmt.Errorf("snapshot created but failed to unprotect and delete snapshot"+
					" due to other failures: %v", err)
			}
		}
	}()

	err = getSnapshotMetadata(rbdSnap, rbdSnap.AdminID, secret)
	if err != nil {
		return err
	}

	return nil
}

// DeleteSnapshot deletes the snapshot in backend and removes the
//snapshot metadata from store
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Warningf("invalid delete snapshot req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot ID cannot be empty")
	}

	snapshotIDMutex.LockKey(snapshotID)
	defer func() {
		if err := snapshotIDMutex.UnlockKey(snapshotID); err != nil {
			klog.Warningf("failed to unlock mutex snapshot:%s %v", snapshotID, err)
		}
	}()

	rbdSnap := &rbdSnapshot{}
	if err := genRBDSnapFromSnapID(rbdSnap, snapshotID, req.GetSecrets()); err != nil {
		// Consider missing snap as already deleted, and proceed to remove the omap values
		if _, ok := err.(util.ErrSnapNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if err := unreserveRBDSnap(rbdSnap, req.GetSecrets()); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteSnapshotResponse{}, nil
	}

	// lock out parallel create requests against the same snap name as we
	// cleanup the image and associated omaps for the same
	snapshotNameMutex.LockKey(rbdSnap.RequestName)
	defer func() {
		if err := snapshotNameMutex.UnlockKey(rbdSnap.RequestName); err != nil {
			klog.Warningf("failed to unlock mutex snapshot:%s %v", rbdSnap.RequestName, err)
		}
	}()

	// Unprotect snapshot
	err := unprotectSnapshot(rbdSnap, rbdSnap.AdminID, req.GetSecrets())
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to unprotect snapshot: %s/%s with error: %v",
			rbdSnap.Pool, rbdSnap.SnapName, err)
	}

	// Deleting snapshot
	klog.V(4).Infof("deleting Snaphot %s", rbdSnap.SnapName)
	if err := deleteSnapshot(rbdSnap, rbdSnap.AdminID, req.GetSecrets()); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to delete snapshot: %s/%s with error: %v",
			rbdSnap.Pool, rbdSnap.SnapName, err)
	}

	return &csi.DeleteSnapshotResponse{}, nil
}
