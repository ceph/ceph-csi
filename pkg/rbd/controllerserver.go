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
	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"
	"github.com/pkg/errors"

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
	MetadataStore util.CachePersister
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
	rbdVol, err := genVolFromVolumeOptions(req.GetParameters(), nil, (isMultiNode && isBlock), false)
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

	// do we need to call this in go routine to make it background job
	err = flattenRbdImage(rbdVol, rbdMaxCloneDepth, cr)
	if err != nil {
		klog.Errorf("failed to  flatten image %v", err)
	}
	return nil
}

// DeleteLegacyVolume deletes a volume provisioned using version 1.0.0 of the plugin
func (cs *ControllerServer) DeleteLegacyVolume(req *csi.DeleteVolumeRequest, cr *util.Credentials) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()

	if cs.MetadataStore == nil {
		return nil, status.Errorf(codes.InvalidArgument, "missing metadata store configuration to"+
			" proceed with deleting legacy volume ID (%s)", volumeID)
	}

	idLk := legacyVolumeIDLocker.Lock(volumeID)
	defer legacyVolumeIDLocker.Unlock(idLk, volumeID)

	rbdVol := &rbdVolume{}
	if err := cs.MetadataStore.Get(volumeID, rbdVol); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			klog.V(3).Infof("metadata for legacy volume %s not found, assuming the volume to be already deleted (%v)", volumeID, err)
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

	klog.V(4).Infof("deleting legacy volume %s", rbdVol.VolName)
	if err := deleteImage(rbdVol, cr); err != nil {
		// TODO: can we detect "already deleted" situations here and proceed?
		klog.V(3).Infof("failed to delete legacy rbd image: %s/%s with error: %v", rbdVol.Pool, rbdVol.VolName, err)
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
		// If error is ErrInvalidVolID it could be a version 1.0.0 or lower volume, attempt
		// to process it as such
		if _, ok := err.(ErrInvalidVolID); ok {
			if isLegacyVolumeID(volumeID) {
				klog.V(2).Infof("attempting deletion of potential legacy volume (%s)", volumeID)
				return cs.DeleteLegacyVolume(req, cr)
			}

			// Consider unknown volumeID as a successfully deleted volume
			return &csi.DeleteVolumeResponse{}, nil
		}

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

	if err := undoVolReservation(rbdVol, cr); err != nil {
		klog.Errorf("failed to remove reservation for volume (%s) with backing image (%s) (%s)",
			rbdVol.RequestName, rbdVol.RbdImageName, err)
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
	rbdVol, err := validateVolHasSnapFeature(req.GetSourceVolumeId(), cr)
	if err != nil {
		return nil, err
	}
	// Create snap volume
	rbdSnap := genSnapFromOptions(rbdVol, req.GetParameters())
	rbdSnap.RbdImageName = rbdVol.RbdImageName
	rbdSnap.SizeBytes = rbdVol.VolSize
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.RequestName = req.GetName()

	idLk := snapshotNameLocker.Lock(req.GetName())
	defer snapshotNameLocker.Unlock(idLk, req.GetName())

	// updating the rbdImage name to point to temparory cloned image name, this
	// is to check if the snapshot is already present or not
	img, err := getCloneImageName(rbdSnap, cr)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		rbdSnap.RbdImageName = img
	}

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

	// this will be used to unreserve snapshot if we failed to get snapshot
	// information from the ceph backend
	updatedRbdImage := ""
	// update the rbd image name to clone from parent volume
	rbdSnap.RbdImageName = rbdVol.RbdImageName
	err = reserveSnap(rbdSnap, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			if updatedRbdImage != "" {
				rbdSnap.RbdImageName = updatedRbdImage
			}
			errDefer := undoSnapReservation(rbdSnap, cr)
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of snapshot: %s %v", req.GetName(), errDefer)
			}
		}
	}()

	// generate temp snap struct and vol struct to create a temprory snapshot
	// and to create a new volume which will be used to take new snapshot
	tmpSnap := &rbdSnapshot{}
	copySnapStruct(tmpSnap, rbdSnap)

	// using request name as the temp snapshot request name. this will help us to
	// identify if the temp snap is already created.
	// this will look like `csi-tmp-snap-1-PVC-XXXX-XXX-XXXX`
	tmpSnap.RequestName = snapJournal.GetTmpNamePrefix(req.GetName(), true, true)
	// generate tempVolume struct
	tmpVol := generateVolFromSnap(rbdSnap)
	tmpVol.ImageFeatures = "layering,deep-flatten"
	// using request name as the volume request name. this will help us to
	// identify if the temp snap is already created.
	// this will look like `csi-tmp-clone-PVC-XXXX-XXX-XXXX`
	tmpVol.RequestName = volJournal.GetTmpNamePrefix(req.GetName(), true, false)
	err = cs.createSnapFromClone(tmpVol, tmpSnap, cr)
	if err != nil {
		return nil, err
	}
	// update the parent image to create snapshot
	rbdSnap.RbdImageName = tmpVol.RbdImageName
	err = createSnapshot(rbdSnap, cr)
	if err != nil {
		return nil, err
	}
	err = updateReservedSnap(rbdSnap, cr)
	if err != nil {
		klog.Errorf("failed to update snapshot parent name: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	updatedRbdImage = rbdSnap.RbdImageName

	err = getSnapshotMetadata(rbdSnap, cr)
	if err != nil {
		klog.Errorf("failed to fetch snapshot metadata: %v", err)
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

func copySnapStruct(tmpSnap, rbdSnap *rbdSnapshot) {
	tmpSnap.ClusterID = rbdSnap.ClusterID
	tmpSnap.Monitors = rbdSnap.Monitors
	tmpSnap.Pool = rbdSnap.Pool
	tmpSnap.RbdImageName = rbdSnap.RbdImageName
	tmpSnap.RequestName = rbdSnap.RequestName
	tmpSnap.SizeBytes = rbdSnap.SizeBytes
	tmpSnap.SourceVolumeID = rbdSnap.SourceVolumeID
}

func getCloneImageName(rbdSnap *rbdSnapshot, cr *util.Credentials) (string, error) {
	tmpVol := &rbdVolume{}
	tmpVol.Monitors = rbdSnap.Monitors
	tmpVol.Pool = rbdSnap.Pool
	tmpVol.ClusterID = rbdSnap.ClusterID
	tmpVol.VolSize = rbdSnap.SizeBytes
	tmpVol.RequestName = volJournal.GetTmpNamePrefix(rbdSnap.RequestName, true, false)
	found, err := checkVolExists(tmpVol, cr)
	if err != nil {
		return "", err
	}
	if found {
		return tmpVol.RbdImageName, nil
	}
	return "", ErrImageNotFound{tmpVol.RequestName, errors.New("failed to get clone image name")}
}

func validateVolHasSnapFeature(parentVolID string, cr *util.Credentials) (*rbdVolume, error) {
	// validate parent volume
	rbdVol := new(rbdVolume)
	err := genVolFromVolID(rbdVol, parentVolID, cr)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); ok {
			return nil, status.Errorf(codes.NotFound, "source Volume ID %s not found", parentVolID)
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// Check if source volume was created with required image features for snaps
	if !hasSnapshotFeature(rbdVol.ImageFeatures) {
		return nil, status.Errorf(codes.InvalidArgument, "volume(%s) has not snapshot feature(layering)", parentVolID)
	}
	return rbdVol, nil
}

func (cs *ControllerServer) createSnapFromClone(rbdVol *rbdVolume, rbdSnap *rbdSnapshot, cr *util.Credentials) error {

	var (
		err     error
		snapErr error
		volErr  error
	)
	found, err := checkSnapExists(rbdSnap, cr)
	if err != nil {
		if _, ok := err.(util.ErrSnapNameConflict); ok {
			return status.Error(codes.AlreadyExists, err.Error())
		}
		return status.Errorf(codes.Internal, err.Error())
	}

	if found {
		klog.Infof("found temp snapshot image %s ", rbdSnap.RequestName)
		// check clone volume Exist
		_, err = checkVolExists(rbdVol, cr)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		klog.Infof("found temp clone image %s for snapshot %s", rbdVol.RequestName, rbdSnap.RequestName)
		goto deleteSnap
	}
	err = reserveSnap(rbdSnap, cr)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	// createsnapshot
	snapErr = createSnapshot(rbdSnap, cr)

	if snapErr != nil {
		errDefer := undoSnapReservation(rbdSnap, cr)
		if errDefer != nil {
			klog.Warningf("failed undoing reservation of snapshot: %s (%s)", rbdSnap.RequestName, errDefer)
		}
		return snapErr
	}
	err = reserveVol(rbdVol, cr)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	volErr = restoreSnapshot(rbdVol, rbdSnap, cr)
	if volErr != nil {
		errDefer := undoVolReservation(rbdVol, cr)
		if errDefer != nil {
			klog.Warningf("failed undoing reservation of volume: %s (%s)", rbdVol.RequestName, errDefer)
		}
		return volErr
	}
deleteSnap:
	// delete snapshot
	if err := deleteSnapshot(rbdSnap, cr); err != nil {
		return status.Errorf(codes.Internal,
			"failed to delete snapshot: %s/%s with error: %v",
			rbdSnap.Pool, rbdSnap.RbdSnapName, err)
	}
	return nil
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

	// generate tempVolume struct
	rbdVol := generateVolFromSnap(rbdSnap)
	rbdVol.RequestName = volJournal.GetTmpNamePrefix(rbdSnap.RequestName, true, false)
	found, err := checkVolExists(rbdVol, cr)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	// Deleting snapshot
	klog.V(4).Infof("deleting Snaphot %s", rbdSnap.RbdSnapName)
	if err := deleteSnapshot(rbdSnap, cr); err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to delete snapshot: %s/%s with error: %v",
			rbdSnap.Pool, rbdSnap.RbdSnapName, err)
	}

	if found {
		// TODO need to delete stale volumes
		// Deleting rbd image
		klog.V(4).Infof("deleting image %s", rbdVol.RbdImageName)
		if err := deleteImage(rbdVol, cr); err != nil {
			klog.Errorf("failed to delete rbd image: %s/%s with error: %v",
				rbdVol.Pool, rbdVol.RbdImageName, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func generateVolFromSnap(snap *rbdSnapshot) *rbdVolume {
	vol := &rbdVolume{
		Pool:      snap.Pool,
		Monitors:  snap.Monitors,
		VolSize:   snap.SizeBytes,
		ClusterID: snap.ClusterID,
	}
	return vol
}
