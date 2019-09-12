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

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

// ControllerServer struct of CEPH CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	MetadataStore util.CachePersister
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	VolumeLocks *util.VolumeLocks
}

type controllerCacheEntry struct {
	VolOptions volumeOptions
	VolumeID   volumeID
}

// createBackingVolume creates the backing subvolume and on any error cleans up any created entities
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
	defer func() {
		if err != nil {
			if errDefer := purgeVolume(ctx, volumeID(vID.FsSubvolName), cr, volOptions); errDefer != nil {
				klog.Warningf(util.Log(ctx, "failed purging volume: %s (%s)"), volOptions.RequestName, errDefer)
			}
		}
	}()

	return nil
}

// CreateVolume creates a reservation and the volume in backend, if it is not already present
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
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), requestName)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, requestName)
	}
	defer cs.VolumeLocks.Release(requestName)

	volOptions, err := newVolumeOptions(ctx, requestName, req.GetCapacityRange().GetRequiredBytes(),
		req.GetParameters(), secret)
	if err != nil {
		klog.Errorf(util.Log(ctx, "validation and extraction of volume options failed: %v"), err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	vID, err := checkVolExists(ctx, volOptions, secret)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if vID != nil {
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vID.VolumeID,
				CapacityBytes: volOptions.Size,
				VolumeContext: req.GetParameters(),
			},
		}, nil
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

	klog.Infof(util.Log(ctx, "cephfs: successfully created backing volume named %s for request name %s"),
		vID.FsSubvolName, requestName)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vID.VolumeID,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: req.GetParameters(),
		},
	}, nil
}

// deleteVolumeDeprecated is used to delete volumes created using version 1.0.0 of the plugin,
// that have state information stored in files or kubernetes config maps
func (cs *ControllerServer) deleteVolumeDeprecated(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var (
		volID   = volumeID(req.GetVolumeId())
		secrets = req.GetSecrets()
	)

	ce := &controllerCacheEntry{}
	if err := cs.MetadataStore.Get(string(volID), ce); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			klog.Infof(util.Log(ctx, "cephfs: metadata for volume %s not found, assuming the volume to be already deleted (%v)"), volID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	if !ce.VolOptions.ProvisionVolume {
		// DeleteVolume() is forbidden for statically provisioned volumes!

		klog.Warningf(util.Log(ctx, "volume %s is provisioned statically, aborting delete"), volID)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// mons may have changed since create volume,
	// retrieve the latest mons and override old mons
	if mon, secretsErr := util.GetMonValFromSecret(secrets); secretsErr == nil && len(mon) > 0 {
		klog.Infof(util.Log(ctx, "overriding monitors [%q] with [%q] for volume %s"), ce.VolOptions.Monitors, mon, volID)
		ce.VolOptions.Monitors = mon
	}

	// Deleting a volume requires admin credentials

	cr, err := util.NewAdminCredentials(secrets)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to retrieve admin credentials: %v"), err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	if acquired := cs.VolumeLocks.TryAcquire(string(volID)); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, string(volID))
	}
	defer cs.VolumeLocks.Release(string(volID))

	if err = purgeVolumeDeprecated(ctx, volID, cr, &ce.VolOptions); err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete volume %s: %v"), volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = deleteCephUserDeprecated(ctx, &ce.VolOptions, cr, volID); err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete ceph user for volume %s: %v"), volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = cs.MetadataStore.Delete(string(volID)); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof(util.Log(ctx, "cephfs: successfully deleted volume %s"), volID)

	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteVolume deletes the volume in backend and its reservation
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(); err != nil {
		klog.Errorf(util.Log(ctx, "DeleteVolumeRequest validation failed: %v"), err)
		return nil, err
	}

	volID := volumeID(req.GetVolumeId())
	secrets := req.GetSecrets()

	// lock out parallel delete operations
	if acquired := cs.VolumeLocks.TryAcquire(string(volID)); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, string(volID))
	}
	defer cs.VolumeLocks.Release(string(volID))

	// Find the volume using the provided VolumeID
	volOptions, vID, err := newVolumeOptionsFromVolID(ctx, string(volID), nil, secrets)
	if err != nil {
		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (subvolume and imageOMap are garbage collected already), hence
		// return success as deletion is complete
		if _, ok := err.(util.ErrKeyNotFound); ok {
			return &csi.DeleteVolumeResponse{}, nil
		}

		// ErrInvalidVolID may mean this is an 1.0.0 version volume
		if _, ok := err.(ErrInvalidVolID); ok && cs.MetadataStore != nil {
			return cs.deleteVolumeDeprecated(ctx, req)
		}

		return nil, status.Error(codes.Internal, err.Error())
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := undoVolReservation(ctx, volOptions, *vID, secrets); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof(util.Log(ctx, "cephfs: successfully deleted volume %s"), volID)

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
