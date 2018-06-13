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
		return fmt.Errorf("invalid CreateVolumeRequest: %v", err)
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
		return fmt.Errorf("invalid DeleteVolumeRequest: %v", err)
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

	conf := cephConfigData{Monitors: volOptions.Monitors, VolumeUuid: volId.uuid}
	if err = conf.writeToFile(); err != nil {
		glog.Errorf("failed to write ceph config file to %s: %v", getCephConfPath(volId.uuid), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Create a volume in case the user didn't provide one

	if volOptions.ProvisionVolume {
		// Admin access is required

		cr, err := getAdminCredentials(req.GetControllerCreateSecrets())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if err = storeCephAdminCredentials(volId.uuid, cr); err != nil {
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

	sz := req.GetCapacityRange().GetRequiredBytes()
	if sz == 0 {
		sz = oneGB
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			Id:            volId.id,
			CapacityBytes: sz,
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

	if !ent.VolOptions.ProvisionVolume {
		// DeleteVolume() is forbidden for statically provisioned volumes!
		msg := fmt.Sprintf("volume %s is provisioned statically, aborting delete", volId)
		glog.Warningf(msg)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// Requires admin credentials

	cr, err := getAdminCredentials(req.GetControllerDeleteSecrets())
	if err != nil {
		glog.Errorf("failed to retrieve admin credentials: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Remove the volume, the user and the volume cache entry

	if err = purgeVolume(volId, cr, &ent.VolOptions); err != nil {
		glog.Errorf("failed to delete volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = deleteCephUser(cr, volUuid); err != nil {
		glog.Errorf("failed to delete ceph user %s: %v", getCephUserName(volUuid), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = volCache.erase(volUuid); err != nil {
		glog.Errorf("failed to delete cache entry for volume %s: %v", volId, err)
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
