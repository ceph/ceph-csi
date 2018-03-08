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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/cinder/openstack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var fakeCs *controllerServer

// Init Controller Server
func init() {
	if fakeCs == nil {
		d := NewDriver(fakeNodeID, fakeEndpoint, fakeConfig)
		fakeCs = NewControllerServer(d)
	}
}

// Test CreateVolume
func TestCreateVolume(t *testing.T) {

	// mock OpenStack
	osmock := new(openstack.OpenStackMock)
	// CreateVolume(name string, size int, vtype, availability string, tags *map[string]string) (string, string, error)
	osmock.On("CreateVolume", fakeVolName, mock.AnythingOfType("int"), fakeVolType, fakeAvailability, (*map[string]string)(nil)).Return(fakeVolID, fakeAvailability, nil)
	openstack.OsInstance = osmock

	// Init assert
	assert := assert.New(t)

	// Fake request
	fakeReq := &csi.CreateVolumeRequest{
		Name:               fakeVolName,
		VolumeCapabilities: nil,
	}

	// Invoke CreateVolume
	actualRes, err := fakeCs.CreateVolume(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to CreateVolume: %v", err)
	}

	// Assert
	assert.NotNil(actualRes.Volume)

	assert.NotEqual(0, len(actualRes.Volume.Id), "Volume Id is nil")

	assert.Equal(fakeAvailability, actualRes.Volume.Attributes["availability"])
}

// Test DeleteVolume
func TestDeleteVolume(t *testing.T) {

	// mock OpenStack
	osmock := new(openstack.OpenStackMock)
	// DeleteVolume(volumeID string) error
	osmock.On("DeleteVolume", fakeVolID).Return(nil)
	openstack.OsInstance = osmock

	// Init assert
	assert := assert.New(t)

	// Fake request
	fakeReq := &csi.DeleteVolumeRequest{
		VolumeId: fakeVolID,
	}

	// Expected Result
	expectedRes := &csi.DeleteVolumeResponse{}

	// Invoke DeleteVolume
	actualRes, err := fakeCs.DeleteVolume(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to DeleteVolume: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}

// Test ControllerPublishVolume
func TestControllerPublishVolume(t *testing.T) {

	// mock OpenStack
	osmock := new(openstack.OpenStackMock)
	// AttachVolume(instanceID, volumeID string) (string, error)
	osmock.On("AttachVolume", fakeNodeID, fakeVolID).Return(fakeVolID, nil)
	// WaitDiskAttached(instanceID string, volumeID string) error
	osmock.On("WaitDiskAttached", fakeNodeID, fakeVolID).Return(nil)
	// GetAttachmentDiskPath(instanceID, volumeID string) (string, error)
	osmock.On("GetAttachmentDiskPath", fakeNodeID, fakeVolID).Return(fakeDevicePath, nil)
	openstack.OsInstance = osmock

	// Init assert
	assert := assert.New(t)

	// Fake request
	fakeReq := &csi.ControllerPublishVolumeRequest{
		VolumeId:         fakeVolID,
		NodeId:           fakeNodeID,
		VolumeCapability: nil,
		Readonly:         false,
	}

	// Expected Result
	expectedRes := &csi.ControllerPublishVolumeResponse{
		PublishInfo: map[string]string{
			"DevicePath": fakeDevicePath,
		},
	}

	// Invoke ControllerPublishVolume
	actualRes, err := fakeCs.ControllerPublishVolume(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to ControllerPublishVolume: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}

// Test ControllerUnpublishVolume
func TestControllerUnpublishVolume(t *testing.T) {

	// mock OpenStack
	osmock := new(openstack.OpenStackMock)
	// DetachVolume(instanceID, volumeID string) error
	osmock.On("DetachVolume", fakeNodeID, fakeVolID).Return(nil)
	// WaitDiskDetached(instanceID string, volumeID string) error
	osmock.On("WaitDiskDetached", fakeNodeID, fakeVolID).Return(nil)
	openstack.OsInstance = osmock

	// Init assert
	assert := assert.New(t)

	// Fake request
	fakeReq := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: fakeVolID,
		NodeId:   fakeNodeID,
	}

	// Expected Result
	expectedRes := &csi.ControllerUnpublishVolumeResponse{}

	// Invoke ControllerUnpublishVolume
	actualRes, err := fakeCs.ControllerUnpublishVolume(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to ControllerUnpublishVolume: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}
