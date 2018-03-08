/*
Copyright 2017 The Kubernetes Authors.

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

package cinder

import (
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/drivers/pkg/cinder/openstack"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/volume/util"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	// Volume Name
	volName := req.GetName()
	if len(volName) == 0 {
		volName = uuid.NewUUID().String()
	}

	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(1 * 1024 * 1024 * 1024)
	if req.GetCapacityRange() != nil {
		volSizeBytes = int64(req.GetCapacityRange().GetRequiredBytes())
	}
	volSizeGB := int(util.RoundUpSize(volSizeBytes, 1024*1024*1024))

	// Volume Type
	volType := req.GetParameters()["type"]

	// Volume Availability - Default is nova
	volAvailability := req.GetParameters()["availability"]

	// Get OpenStack Provider
	cloud, err := openstack.GetOpenStackProvider()
	if err != nil {
		glog.V(3).Infof("Failed to GetOpenStackProvider: %v", err)
		return nil, err
	}

	// Volume Create
	resID, resAvailability, err := cloud.CreateVolume(volName, volSizeGB, volType, volAvailability, nil)
	if err != nil {
		glog.V(3).Infof("Failed to CreateVolume: %v", err)
		return nil, err
	}

	glog.V(4).Infof("Create volume %s in Availability Zone: %s", resID, resAvailability)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			Id: resID,
			Attributes: map[string]string{
				"availability": resAvailability,
			},
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {

	// Get OpenStack Provider
	cloud, err := openstack.GetOpenStackProvider()
	if err != nil {
		glog.V(3).Infof("Failed to GetOpenStackProvider: %v", err)
		return nil, err
	}

	// Volume Delete
	volID := req.GetVolumeId()
	err = cloud.DeleteVolume(volID)
	if err != nil {
		glog.V(3).Infof("Failed to DeleteVolume: %v", err)
		return nil, err
	}

	glog.V(4).Infof("Delete volume %s", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {

	// Get OpenStack Provider
	cloud, err := openstack.GetOpenStackProvider()
	if err != nil {
		glog.V(3).Infof("Failed to GetOpenStackProvider: %v", err)
		return nil, err
	}

	// Volume Attach
	instanceID := req.GetNodeId()
	volumeID := req.GetVolumeId()

	_, err = cloud.AttachVolume(instanceID, volumeID)
	if err != nil {
		glog.V(3).Infof("Failed to AttachVolume: %v", err)
		return nil, err
	}

	err = cloud.WaitDiskAttached(instanceID, volumeID)
	if err != nil {
		glog.V(3).Infof("Failed to WaitDiskAttached: %v", err)
		return nil, err
	}

	devicePath, err := cloud.GetAttachmentDiskPath(instanceID, volumeID)
	if err != nil {
		glog.V(3).Infof("Failed to GetAttachmentDiskPath: %v", err)
		return nil, err
	}

	glog.V(4).Infof("ControllerPublishVolume %s on %s", volumeID, instanceID)

	// Publish Volume Info
	pvInfo := map[string]string{}
	pvInfo["DevicePath"] = devicePath

	return &csi.ControllerPublishVolumeResponse{
		PublishInfo: pvInfo,
	}, nil
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

	// Get OpenStack Provider
	cloud, err := openstack.GetOpenStackProvider()
	if err != nil {
		glog.V(3).Infof("Failed to GetOpenStackProvider: %v", err)
		return nil, err
	}

	// Volume Detach
	instanceID := req.GetNodeId()
	volumeID := req.GetVolumeId()

	err = cloud.DetachVolume(instanceID, volumeID)
	if err != nil {
		glog.V(3).Infof("Failed to DetachVolume: %v", err)
		return nil, err
	}

	err = cloud.WaitDiskDetached(instanceID, volumeID)
	if err != nil {
		glog.V(3).Infof("Failed to WaitDiskDetached: %v", err)
		return nil, err
	}

	glog.V(4).Infof("ControllerUnpublishVolume %s on %s", volumeID, instanceID)

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}
