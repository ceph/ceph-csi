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
	"path"

	"github.com/golang/glog"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const (
	oneGB = 1073741824
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

func GetVersionString(ver *csi.Version) string {
	return fmt.Sprintf("%d.%d.%d", ver.Major, ver.Minor, ver.Patch)
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(req.Version, csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.V(3).Infof("invalid create volume req: %v", req)
		return nil, err
	}

	volOptions, err := getRBDVolumeOptions(req.GetParameters())
	if err != nil {
		return nil, err
	}

	// Generating Volume Name and Volume ID, as accoeding to CSI spec they MUST be different
	volName := req.GetName()
	uniqueID := uuid.NewUUID().String()
	if len(volName) == 0 {
		volName = volOptions.Pool + "-dynamic-pvc-" + uniqueID
	}
	volOptions.VolName = volName
	volumeID := "csi-rbd-" + uniqueID
	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(oneGB)
	if req.GetCapacityRange() != nil {
		volSizeBytes = int64(req.GetCapacityRange().GetRequiredBytes())
	}
	volSizeGB := int(volSizeBytes / 1024 / 1024 / 1024)

	// Check if there is already RBD image with requested name
	found, _, _ := rbdStatus(volOptions)
	if !found {
		if err := createRBDImage(volOptions, volSizeGB); err != nil {
			if err != nil {
				glog.Warningf("failed to create volume: %v", err)
				return nil, err
			}
		}
		glog.V(4).Infof("create volume %s", volName)
	}
	// Storing volInfo into a persistent file, will need info to delete rbd image
	// in ControllerUnpublishVolume
	if err := persistVolInfo(volumeID, path.Join(PluginFolder, "controller"), volOptions); err != nil {
		glog.Warningf("rbd: failed to store volInfo with error: %v", err)
	}

	return &csi.CreateVolumeResponse{
		VolumeInfo: &csi.VolumeInfo{
			Id:            volumeID,
			CapacityBytes: uint64(volSizeBytes),
			Attributes:    req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(req.Version, csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.Warningf("invalid delete volume req: %v", req)
		return nil, err
	}

	// For now the image get unconditionally deleted, but here retention policy can be checked
	volumeID := req.GetVolumeId()
	volOptions := &rbdVolumeOptions{}
	if err := loadVolInfo(volumeID, path.Join(PluginFolder, "controller"), volOptions); err != nil {
		return nil, err
	}

	volName := volOptions.VolName
	// Deleting rbd image
	glog.V(4).Infof("deleting volume %s", volName)
	if err := deleteRBDImage(volOptions); err != nil {
		glog.V(3).Infof("failed to delete rbd image: %s/%s with error: %v", volOptions.Pool, volName, err)
		return nil, err
	}
	// Removing persistent storage file for the unmapped volume
	if err := deleteVolInfo(volumeID, path.Join(PluginFolder, "controller")); err != nil {
		return nil, err
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return &csi.ValidateVolumeCapabilitiesResponse{Supported: false, Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Supported: true, Message: ""}, nil
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}
