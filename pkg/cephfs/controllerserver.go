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

package cephfs

import (
	"fmt"
	"os"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

const (
	oneGB = 1073741824
)

func (cs *controllerServer) validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("Invalid CreateVolumeRequest: %v", err)
	}

	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}

	if req.GetVolumeCapabilities() == nil {
		return status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}

	return nil
}

func (cs *controllerServer) validateDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("Invalid DeleteVolumeRequest: %v", err)
	}

	return nil
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(req); err != nil {
		glog.Errorf("CreateVolumeRequest validation failed: %v", err)
		return nil, err
	}

	// Configuration

	volOptions, err := newVolumeOptions(req.GetParameters())
	if err != nil {
		glog.Errorf("validation of volume options failed: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volId := newVolumeIdentifier(volOptions, req)

	// Create a volume in case the user didn't provide one

	if volOptions.ProvisionVolume {
		// Admin access is required

		cr, err := getAdminCredentials(req.GetControllerCreateSecrets())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if err = storeCephAdminCredentials(cr); err != nil {
			glog.Errorf("failed to store admin credentials for '%s': %v", cr.id, err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		if err = createVolume(volOptions, cr, volId.uuid, req.GetCapacityRange().GetRequiredBytes()); err != nil {
			glog.Errorf("failed to create volume %s: %v", volId.name, err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		glog.V(4).Infof("cephfs: volume %s successfuly created", volId.id)
	} else {
		glog.V(4).Infof("cephfs: volume %s is provisioned statically", volId.id)
	}

	if err = volCache.insert(&volumeCacheEntry{Identifier: *volId, VolOptions: *volOptions}); err != nil {
		glog.Warningf("failed to store a volume cache entry: %v", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			Id:            volId.id,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			Attributes:    req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(req); err != nil {
		glog.Errorf("DeleteVolumeRequest validation failed: %v", err)
		return nil, err
	}

	var (
		cr      *credentials
		err     error
		volId   = req.GetVolumeId()
		volUuid = uuidFromVolumeId(volId)
	)

	// Load volume info from cache

	ent, found := volCache.get(volUuid)
	if !found {
		msg := fmt.Sprintf("failed to retrieve cache entry for volume %s", volId)
		glog.Error(msg)
		return nil, status.Error(codes.Internal, msg)
	}

	// Set the correct user for mounting

	if ent.VolOptions.ProvisionVolume {
		// Admin access is required

		cr, err = getAdminCredentials(req.GetControllerDeleteSecrets())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	} else {
		cr, err = getUserCredentials(req.GetControllerDeleteSecrets())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	// Delete the volume contents

	if err := purgeVolume(volId, cr, &ent.VolOptions); err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Clean up remaining files

	if ent.VolOptions.ProvisionVolume {
		// The user is no longer needed
		if err := deleteCephUser(volUuid); err != nil {
			glog.Warningf("failed to delete ceph user '%s': %v", cr.id, err)
		}

		userId := getCephUserName(volUuid)
		os.Remove(getCephKeyringPath(userId))
		os.Remove(getCephSecretPath(userId))
	} else {
		os.Remove(getCephKeyringPath(cr.id))
		os.Remove(getCephSecretPath(cr.id))
	}

	if err := volCache.erase(volUuid); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("cephfs: volume %s successfuly deleted", volId)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return &csi.ValidateVolumeCapabilitiesResponse{Supported: true}, nil
}
