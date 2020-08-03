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

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
)

// ControllerServer struct of CEPH CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	VolumeLocks *util.VolumeLocks
}

// createBackingVolume creates the backing subvolume and on any error cleans up any created entities.
func (cs *ControllerServer) createBackingVolume(ctx context.Context, volOptions *volumeOptions, vID *volumeIdentifier, secret map[string]string) error {
	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	if err = createVolume(ctx, volOptions, cr, volumeID(vID.FsSubvolName), volOptions.Size); err != nil {
		klog.Errorf(util.Log(ctx, "failed to create volume %s: %v"), volOptions.RequestName, err)
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// CreateVolume creates a reservation and the volume in backend, if it is not already present.
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(req); err != nil {
		klog.Errorf(util.Log(ctx, "CreateVolumeRequest validation failed: %v"), err)
		return nil, err
	}

	// Configuration
	secret := req.GetSecrets()
	requestName := req.GetName()

	// Existence and conflict checks
	if acquired := cs.VolumeLocks.TryAcquire(requestName); !acquired {
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), requestName)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, requestName)
	}
	defer cs.VolumeLocks.Release(requestName)

	volOptions, err := newVolumeOptions(ctx, requestName, req, secret)
	if err != nil {
		klog.Errorf(util.Log(ctx, "validation and extraction of volume options failed: %v"), err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.GetCapacityRange() != nil {
		volOptions.Size = util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())
	}
	// TODO need to add check for 0 volume size

	vID, err := checkVolExists(ctx, volOptions, secret)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// TODO return error message if requested vol size greater than found volume return error

	if vID != nil {
		volumeContext := req.GetParameters()
		volumeContext["subvolumeName"] = vID.FsSubvolName
		volume := &csi.Volume{
			VolumeId:      vID.VolumeID,
			CapacityBytes: volOptions.Size,
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
	vID, err = reserveVol(ctx, volOptions, secret)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoVolReservation(ctx, volOptions, *vID, secret)
			if errDefer != nil {
				klog.Warningf(util.Log(ctx, "failed undoing reservation of volume: %s (%s)"),
					requestName, errDefer)
			}
		}
	}()

	// Create a volume
	err = cs.createBackingVolume(ctx, volOptions, vID, secret)
	if err != nil {
		return nil, err
	}

	util.DebugLog(ctx, "cephfs: successfully created backing volume named %s for request name %s",
		vID.FsSubvolName, requestName)
	volumeContext := req.GetParameters()
	volumeContext["subvolumeName"] = vID.FsSubvolName
	volume := &csi.Volume{
		VolumeId:      vID.VolumeID,
		CapacityBytes: volOptions.Size,
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
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(); err != nil {
		klog.Errorf(util.Log(ctx, "DeleteVolumeRequest validation failed: %v"), err)
		return nil, err
	}

	volID := volumeID(req.GetVolumeId())
	secrets := req.GetSecrets()

	// lock out parallel delete operations
	if acquired := cs.VolumeLocks.TryAcquire(string(volID)); !acquired {
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, string(volID))
	}
	defer cs.VolumeLocks.Release(string(volID))

	// Find the volume using the provided VolumeID
	volOptions, vID, err := newVolumeOptionsFromVolID(ctx, string(volID), nil, secrets)
	if err != nil {
		// if error is ErrPoolNotFound, the pool is already deleted we dont
		// need to worry about deleting subvolume or omap data, return success
		if errors.Is(err, util.ErrPoolNotFound) {
			klog.Warningf(util.Log(ctx, "failed to get backend volume for %s: %v"), string(volID), err)
			return &csi.DeleteVolumeResponse{}, nil
		}
		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (subvolume and imageOMap are garbage collected already), hence
		// return success as deletion is complete
		if errors.Is(err, util.ErrKeyNotFound) {
			return &csi.DeleteVolumeResponse{}, nil
		}

		// All errors other than ErrVolumeNotFound should return an error back to the caller
		if !errors.Is(err, ErrVolumeNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If error is ErrImageNotFound then we failed to find the subvolume, but found the imageOMap
		// to lead us to the image, hence the imageOMap needs to be garbage collected, by calling
		// unreserve for the same
		if acquired := cs.VolumeLocks.TryAcquire(volOptions.RequestName); !acquired {
			return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volOptions.RequestName)
		}
		defer cs.VolumeLocks.Release(volOptions.RequestName)

		if err = undoVolReservation(ctx, volOptions, *vID, secrets); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.DeleteVolumeResponse{}, nil
	}

	// lock out parallel delete and create requests against the same volume name as we
	// cleanup the subvolume and associated omaps for the same
	if acquired := cs.VolumeLocks.TryAcquire(volOptions.RequestName); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volOptions.RequestName)
	}
	defer cs.VolumeLocks.Release(string(volID))

	// Deleting a volume requires admin credentials
	cr, err := util.NewAdminCredentials(secrets)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to retrieve admin credentials: %v"), err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	if err = purgeVolume(ctx, volumeID(vID.FsSubvolName), cr, volOptions); err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete volume %s: %v"), volID, err)
		// All errors other than ErrVolumeNotFound should return an error back to the caller
		if !errors.Is(err, ErrVolumeNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err := undoVolReservation(ctx, volOptions, *vID, secrets); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	util.DebugLog(ctx, "cephfs: successfully deleted volume %s", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// Cephfs doesn't support Block volume
	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
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
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if err := cs.validateExpandVolumeRequest(req); err != nil {
		klog.Errorf(util.Log(ctx, "ControllerExpandVolumeRequest validation failed: %v"), err)
		return nil, err
	}

	volID := req.GetVolumeId()
	secret := req.GetSecrets()

	// lock out parallel delete operations
	if acquired := cs.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Errorf(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer cs.VolumeLocks.Release(volID)

	// lock out volumeID for clone and delete operation
	if err := cs.OperationLocks.GetExpandLock(volID); err != nil {
		klog.Error(util.Log(ctx, err.Error()))
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer cs.OperationLocks.ReleaseExpandLock(volID)

	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	volOptions, volIdentifier, err := newVolumeOptionsFromVolID(ctx, volID, nil, secret)

	if err != nil {
		klog.Errorf(util.Log(ctx, "validation and extraction of volume options failed: %v"), err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	RoundOffSize := util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())

	if err = resizeVolume(ctx, volOptions, cr, volumeID(volIdentifier.FsSubvolName), RoundOffSize); err != nil {
		klog.Errorf(util.Log(ctx, "failed to expand volume %s: %v"), volumeID(volIdentifier.FsSubvolName), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         RoundOffSize,
		NodeExpansionRequired: false,
	}, nil
}
