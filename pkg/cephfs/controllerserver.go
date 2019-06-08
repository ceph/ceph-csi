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
	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/utils/keymutex"
)

// ControllerServer struct of CEPH CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	MetadataStore util.CachePersister
}

type controllerCacheEntry struct {
	VolOptions volumeOptions
	VolumeID   volumeID
}

var (
	mtxControllerVolumeID   = keymutex.NewHashed(0)
	mtxControllerVolumeName = keymutex.NewHashed(0)
)

// createBackingVolume creates the backing subvolume and user/key for the given volOptions and vID,
// and on any error cleans up any created entities
func (cs *ControllerServer) createBackingVolume(volOptions *volumeOptions, vID *volumeIdentifier, secret map[string]string) error {
	cr, err := getAdminCredentials(secret)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	if err = createVolume(volOptions, volumeID(vID.FsSubvolName), volOptions.Size); err != nil {
		klog.Errorf("failed to create volume %s: %v", volOptions.RequestName, err)
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			if errDefer := purgeVolume(volumeID(vID.FsSubvolName), volOptions); errDefer != nil {
				klog.Warningf("failed purging volume: %s (%s)", volOptions.RequestName, errDefer)
			}
		}
	}()

	if _, err = createCephUser(volOptions, cr, volumeID(vID.FsSubvolName)); err != nil {
		klog.Errorf("failed to create ceph user for volume %s: %v", volOptions.RequestName, err)
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// CreateVolume creates a reservation and the volume in backend, if it is not already present
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(req); err != nil {
		klog.Errorf("CreateVolumeRequest validation failed: %v", err)
		return nil, err
	}

	// Configuration
	secret := req.GetSecrets()
	requestName := req.GetName()
	volOptions, err := newVolumeOptions(requestName, req.GetCapacityRange().GetRequiredBytes(),
		req.GetParameters(), secret)
	if err != nil {
		klog.Errorf("validation and extraction of volume options failed: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Existence and conflict checks
	mtxControllerVolumeName.LockKey(requestName)
	defer mustUnlock(mtxControllerVolumeName, requestName)

	vID, err := checkVolExists(volOptions, secret)
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
	vID, err = reserveVol(volOptions, secret)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			errDefer := undoVolReservation(volOptions, *vID, secret)
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of volume: %s (%s)",
					requestName, errDefer)
			}
		}
	}()

	// Create a volume
	err = cs.createBackingVolume(volOptions, vID, secret)
	if err != nil {
		return nil, err
	}

	klog.Infof("cephfs: successfully created backing volume named %s for request name %s",
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
func (cs *ControllerServer) deleteVolumeDeprecated(req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var (
		volID   = volumeID(req.GetVolumeId())
		secrets = req.GetSecrets()
	)

	ce := &controllerCacheEntry{}
	if err := cs.MetadataStore.Get(string(volID), ce); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			klog.Infof("cephfs: metadata for volume %s not found, assuming the volume to be already deleted (%v)", volID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	if !ce.VolOptions.ProvisionVolume {
		// DeleteVolume() is forbidden for statically provisioned volumes!

		klog.Warningf("volume %s is provisioned statically, aborting delete", volID)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// mons may have changed since create volume,
	// retrieve the latest mons and override old mons
	if mon, secretsErr := getMonValFromSecret(secrets); secretsErr == nil && len(mon) > 0 {
		klog.Infof("overriding monitors [%q] with [%q] for volume %s", ce.VolOptions.Monitors, mon, volID)
		ce.VolOptions.Monitors = mon
	}

	// Deleting a volume requires admin credentials

	cr, err := getAdminCredentials(secrets)
	if err != nil {
		klog.Errorf("failed to retrieve admin credentials: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	mtxControllerVolumeID.LockKey(string(volID))
	defer mustUnlock(mtxControllerVolumeID, string(volID))

	if err = purgeVolume(volID, &ce.VolOptions); err != nil {
		klog.Errorf("failed to delete volume %s: %v", volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = deleteCephUser(&ce.VolOptions, cr, volID); err != nil {
		klog.Errorf("failed to delete ceph user for volume %s: %v", volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = cs.MetadataStore.Delete(string(volID)); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof("cephfs: successfully deleted volume %s", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteVolume deletes the volume in backend and its reservation
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(); err != nil {
		klog.Errorf("DeleteVolumeRequest validation failed: %v", err)
		return nil, err
	}

	volID := volumeID(req.GetVolumeId())
	secrets := req.GetSecrets()

	// Find the volume using the provided VolumeID
	volOptions, vID, err := newVolumeOptionsFromVolID(string(volID), nil, secrets)
	if err != nil {
		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (subvolume and imageOMap are garbage collected already), hence
		// return success as deletion is complete
		if _, ok := err.(util.ErrKeyNotFound); ok {
			return &csi.DeleteVolumeResponse{}, nil
		}

		// ErrInvalidVolID may mean this is an 1.0.0 version volume
		if _, ok := err.(ErrInvalidVolID); ok && cs.MetadataStore != nil {
			return cs.deleteVolumeDeprecated(req)
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	// Deleting a volume requires admin credentials
	cr, err := getAdminCredentials(secrets)
	if err != nil {
		klog.Errorf("failed to retrieve admin credentials: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// lock out parallel delete and create requests against the same volume name as we
	// cleanup the subvolume and associated omaps for the same
	mtxControllerVolumeName.LockKey(volOptions.RequestName)
	defer mustUnlock(mtxControllerVolumeName, volOptions.RequestName)

	if err = purgeVolume(volumeID(vID.FsSubvolName), volOptions); err != nil {
		klog.Errorf("failed to delete volume %s: %v", volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = deleteCephUser(volOptions, cr, volumeID(vID.FsSubvolName)); err != nil {
		klog.Errorf("failed to delete ceph user for volume %s: %v", volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := undoVolReservation(volOptions, *vID, secrets); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof("cephfs: successfully deleted volume %s", volID)

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
