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
	"fmt"

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
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
	return nil
}

func (cs *ControllerServer) parseVolCreateRequest(req *csi.CreateVolumeRequest) (*rbdVolume, error) {
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
	rbdVol, err := genVolFromVolumeOptions(req.GetParameters(), (isMultiNode && isBlock))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rbdVol.RequestName = req.GetName()

	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(oneGB)
	if req.GetCapacityRange() != nil {
		volSizeBytes = req.GetCapacityRange().GetRequiredBytes()
	}

	// always round up the request size in bytes to the nearest MiB
	rbdVol.VolSize = util.MiB * util.RoundUpToMiB(volSizeBytes)

	// NOTE: rbdVol does not contain VolID and RbdImageName populated, everything
	// else is populated post create request parsing
	return rbdVol, nil
}

// CreateVolume creates the volume in backend
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	if err := cs.validateVolumeReq(req); err != nil {
		return nil, err
	}

	cr, err := util.GetUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	rbdVol, err := cs.parseVolCreateRequest(req)
	if err != nil {
		return nil, err
	}

	idLk := volumeNameLocker.Lock(req.GetName())
	defer volumeNameLocker.Unlock(idLk, req.GetName())

	found, err := checkVolExists(rbdVol, cr)
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

	err = reserveVol(rbdVol, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoVolReservation(rbdVol, cr)
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of volume: %s (%s)", req.GetName(), errDefer)
			}
		}
	}()

	err = cs.createBackingImage(rbdVol, req, util.RoundUpToMiB(rbdVol.VolSize))
	if err != nil {
		return nil, err
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      rbdVol.VolID,
			CapacityBytes: rbdVol.VolSize,
			VolumeContext: req.GetParameters(),
		},
	}, nil
}

func (cs *ControllerServer) createBackingImage(rbdVol *rbdVolume, req *csi.CreateVolumeRequest, volSizeMiB int64) error {
	var err error

	// if VolumeContentSource is not nil, this request is for snapshot
	if req.VolumeContentSource != nil {
		if err = cs.checkSnapshot(req, rbdVol); err != nil {
			return err
		}
	} else {
		cr, err := util.GetUserCredentials(req.GetSecrets())
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}

		err = createImage(rbdVol, volSizeMiB, cr)
		if err != nil {
			klog.Warningf("failed to create volume: %v", err)
			return status.Error(codes.Internal, err.Error())
		}

		klog.V(4).Infof("created image %s", rbdVol.RbdImageName)
	}

	return nil
}
func (cs *ControllerServer) checkSnapshot(req *csi.CreateVolumeRequest, rbdVol *rbdVolume) error {
	snapshot := req.VolumeContentSource.GetSnapshot()
	if snapshot == nil {
		return status.Error(codes.InvalidArgument, "volume Snapshot cannot be empty")
	}

	snapshotID := snapshot.GetSnapshotId()
	if snapshotID == "" {
		return status.Error(codes.InvalidArgument, "volume Snapshot ID cannot be empty")
	}

	cr, err := util.GetUserCredentials(req.GetSecrets())
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	rbdSnap := &rbdSnapshot{}
	if err = genSnapFromSnapID(rbdSnap, snapshotID, cr); err != nil {
		if _, ok := err.(ErrSnapNotFound); !ok {
			return status.Error(codes.Internal, err.Error())
		}
		return status.Error(codes.InvalidArgument, "missing requested Snapshot ID")
	}

	err = restoreSnapshot(rbdVol, rbdSnap, cr)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof("create volume %s from snapshot %s", req.GetName(), rbdSnap.RbdSnapName)
	return nil
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Warningf("invalid delete volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	cr, err := util.GetUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// For now the image get unconditionally deleted, but here retention policy can be checked
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	rbdVol := &rbdVolume{}
	if err := genVolFromVolID(rbdVol, volumeID, cr); err != nil {
		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (image and imageOMap are garbage collected already), hence return
		// success as deletion is complete
		if _, ok := err.(util.ErrKeyNotFound); ok {
			return &csi.DeleteVolumeResponse{}, nil
		}

		// All errors other than ErrImageNotFound should return an error back to the caller
		if _, ok := err.(ErrImageNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If error is ErrImageNotFound then we failed to find the image, but found the imageOMap
		// to lead us to the image, hence the imageOMap needs to be garbage collected, by calling
		// unreserve for the same
		idLk := volumeNameLocker.Lock(rbdVol.RequestName)
		defer volumeNameLocker.Unlock(idLk, rbdVol.RequestName)

		if err := undoVolReservation(rbdVol, cr); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteVolumeResponse{}, nil
	}

	// lock out parallel create requests against the same volume name as we
	// cleanup the image and associated omaps for the same
	idLk := volumeNameLocker.Lock(rbdVol.RequestName)
	defer volumeNameLocker.Unlock(idLk, rbdVol.RequestName)

	// Deleting rbd image
	klog.V(4).Infof("deleting image %s", rbdVol.RbdImageName)
	if err := deleteImage(rbdVol, cr); err != nil {
		klog.Errorf("failed to delete rbd image: %s/%s with error: %v",
			rbdVol.Pool, rbdVol.RbdImageName, err)
		return nil, status.Error(codes.Internal, err.Error())
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
	if err := cs.validateSnapshotReq(req); err != nil {
		return nil, err
	}

	cr, err := util.GetUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Fetch source volume information
	rbdVol := new(rbdVolume)
	err = genVolFromVolID(rbdVol, req.GetSourceVolumeId(), cr)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); ok {
			return nil, status.Errorf(codes.NotFound, "source Volume ID %s not found", req.GetSourceVolumeId())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// Check if source volume was created with required image features for snaps
	if !hasSnapshotFeature(rbdVol.ImageFeatures) {
		return nil, status.Errorf(codes.InvalidArgument, "volume(%s) has not snapshot feature(layering)", req.GetSourceVolumeId())
	}

	// Create snap volume
	rbdSnap := genSnapFromOptions(rbdVol, req.GetParameters())
	rbdSnap.RbdImageName = rbdVol.RbdImageName
	rbdSnap.SizeBytes = rbdVol.VolSize
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.RequestName = req.GetName()

	idLk := snapshotNameLocker.Lock(req.GetName())
	defer snapshotNameLocker.Unlock(idLk, req.GetName())

	// Need to check for already existing snapshot name, and if found
	// check for the requested source volume id and already allocated source volume id
	found, err := checkSnapExists(rbdSnap, cr)
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

	err = reserveSnap(rbdSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoSnapReservation(rbdSnap, cr)
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of snapshot: %s %v", req.GetName(), errDefer)
			}
		}
	}()

	err = cs.doSnapshot(rbdSnap, cr)
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

func (cs *ControllerServer) validateSnapshotReq(req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Warningf("invalid create snapshot req: %v", protosanitizer.StripSecrets(req))
		return err
	}

	// Check sanity of request Snapshot Name, Source Volume Id
	if req.Name == "" {
		return status.Error(codes.InvalidArgument, "snapshot Name cannot be empty")
	}
	if req.SourceVolumeId == "" {
		return status.Error(codes.InvalidArgument, "source Volume ID cannot be empty")
	}

	return nil
}

func (cs *ControllerServer) doSnapshot(rbdSnap *rbdSnapshot, cr *util.Credentials) (err error) {
	err = createSnapshot(rbdSnap, cr)
	// If snap creation fails, even due to snapname already used, fail, next attempt will get a new
	// uuid for use as the snap name
	if err != nil {
		klog.Errorf("failed to create snapshot: %v", err)
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := deleteSnapshot(rbdSnap, cr)
			if errDefer != nil {
				klog.Errorf("failed to delete snapshot: %v", errDefer)
				err = fmt.Errorf("snapshot created but failed to delete snapshot due to"+
					" other failures: %v", err)
			}
			err = status.Error(codes.Internal, err.Error())
		}
	}()

	err = protectSnapshot(rbdSnap, cr)
	if err != nil {
		klog.Errorf("failed to protect snapshot: %v", err)
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := unprotectSnapshot(rbdSnap, cr)
			if errDefer != nil {
				klog.Errorf("failed to unprotect snapshot: %v", errDefer)
				err = fmt.Errorf("snapshot created but failed to unprotect snapshot due to"+
					" other failures: %v", err)
			}
			err = status.Error(codes.Internal, err.Error())
		}
	}()

	err = getSnapshotMetadata(rbdSnap, cr)
	if err != nil {
		klog.Errorf("failed to fetch snapshot metadata: %v", err)
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// DeleteSnapshot deletes the snapshot in backend and removes the
// snapshot metadata from store
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Warningf("invalid delete snapshot req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	cr, err := util.GetUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	snapshotID := req.GetSnapshotId()
	if snapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID cannot be empty")
	}

	rbdSnap := &rbdSnapshot{}
	if err = genSnapFromSnapID(rbdSnap, snapshotID, cr); err != nil {
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
		// safeguarding against parallel create or delete requests against the same name.
		idLk := snapshotNameLocker.Lock(rbdSnap.RequestName)
		defer snapshotNameLocker.Unlock(idLk, rbdSnap.RequestName)

		if err = undoSnapReservation(rbdSnap, cr); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteSnapshotResponse{}, nil
	}

	// safeguard against parallel create or delete requests against the same name
	idLk := snapshotNameLocker.Lock(rbdSnap.RequestName)
	defer snapshotNameLocker.Unlock(idLk, rbdSnap.RequestName)

	// Unprotect snapshot
	err = unprotectSnapshot(rbdSnap, cr)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to unprotect snapshot: %s/%s with error: %v",
			rbdSnap.Pool, rbdSnap.RbdSnapName, err)
	}

	// Deleting snapshot
	klog.V(4).Infof("deleting Snaphot %s", rbdSnap.RbdSnapName)
	if err := deleteSnapshot(rbdSnap, cr); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to delete snapshot: %s/%s with error: %v",
			rbdSnap.Pool, rbdSnap.RbdSnapName, err)
	}

	return &csi.DeleteSnapshotResponse{}, nil
}
