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
	"k8s.io/kubernetes/pkg/util/keymutex"
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
	mtxControllerVolumeID = keymutex.NewHashed(0)
)

// CreateVolume creates the volume in backend and store the volume metadata
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(req); err != nil {
		klog.Errorf("CreateVolumeRequest validation failed: %v", err)
		return nil, err
	}

	// Configuration

	secret := req.GetSecrets()
	volOptions, err := newVolumeOptions(req.GetParameters(), secret)
	if err != nil {
		klog.Errorf("validation of volume options failed: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volID := makeVolumeID(req.GetName())

	mtxControllerVolumeID.LockKey(string(volID))
	defer mustUnlock(mtxControllerVolumeID, string(volID))

	// Create a volume in case the user didn't provide one

	if volOptions.ProvisionVolume {
		// Admin credentials are required
		cr, err := getAdminCredentials(secret)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if err = createVolume(volOptions, cr, volID, req.GetCapacityRange().GetRequiredBytes()); err != nil {
			klog.Errorf("failed to create volume %s: %v", req.GetName(), err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		if _, err = createCephUser(volOptions, cr, volID); err != nil {
			klog.Errorf("failed to create ceph user for volume %s: %v", req.GetName(), err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		klog.Infof("cephfs: successfully created volume %s", volID)
	} else {
		klog.Infof("cephfs: volume %s is provisioned statically", volID)
	}

	ce := &controllerCacheEntry{VolOptions: *volOptions, VolumeID: volID}
	if err := cs.MetadataStore.Create(string(volID), ce); err != nil {
		klog.Errorf("failed to store a cache entry for volume %s: %v", volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      string(volID),
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: req.GetParameters(),
		},
	}, nil
}

// DeleteVolume deletes the volume in backend
// and removes the volume metadata from store
// nolint: gocyclo
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(); err != nil {
		klog.Errorf("DeleteVolumeRequest validation failed: %v", err)
		return nil, err
	}

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

	if err = purgeVolume(volID, cr, &ce.VolOptions); err != nil {
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
