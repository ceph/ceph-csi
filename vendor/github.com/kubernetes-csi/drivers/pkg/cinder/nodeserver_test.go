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
	"github.com/kubernetes-csi/drivers/pkg/cinder/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var fakeNs *nodeServer

// Init Node Server
func init() {
	if fakeNs == nil {
		d := NewDriver(fakeNodeID, fakeEndpoint, fakeConfig)
		fakeNs = NewNodeServer(d)
	}
}

// Test NodeGetId
func TestNodeGetId(t *testing.T) {

	// mock MountMock
	mmock := new(mount.MountMock)
	// GetInstanceID() (string, error)
	mmock.On("GetInstanceID").Return(fakeNodeID, nil)
	mount.MInstance = mmock

	// Init assert
	assert := assert.New(t)

	// Expected Result
	expectedRes := &csi.NodeGetIdResponse{
		NodeId: fakeNodeID,
	}

	// Fake request
	fakeReq := &csi.NodeGetIdRequest{}

	// Invoke NodeGetId
	actualRes, err := fakeNs.NodeGetId(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to NodeGetId: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}

// Test NodeGetInfo
func TestNodeGetInfo(t *testing.T) {

	// mock MountMock
	mmock := new(mount.MountMock)
	// GetInstanceID() (string, error)
	mmock.On("GetInstanceID").Return(fakeNodeID, nil)
	mount.MInstance = mmock

	// Init assert
	assert := assert.New(t)

	// Expected Result
	expectedRes := &csi.NodeGetInfoResponse{
		NodeId: fakeNodeID,
	}

	// Fake request
	fakeReq := &csi.NodeGetInfoRequest{}

	// Invoke NodeGetId
	actualRes, err := fakeNs.NodeGetInfo(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to NodeGetInfo: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}

// Test NodePublishVolume
func TestNodePublishVolume(t *testing.T) {

	// mock MountMock
	mmock := new(mount.MountMock)
	// ScanForAttach(devicePath string) error
	mmock.On("ScanForAttach", fakeDevicePath).Return(nil)
	// IsLikelyNotMountPointAttach(targetpath string) (bool, error)
	mmock.On("IsLikelyNotMountPointAttach", fakeTargetPath).Return(true, nil)
	// FormatAndMount(source string, target string, fstype string, options []string) error
	mmock.On("FormatAndMount", fakeDevicePath, fakeTargetPath, mock.AnythingOfType("string"), []string{"rw"}).Return(nil)
	mount.MInstance = mmock

	// Init assert
	assert := assert.New(t)

	// Expected Result
	expectedRes := &csi.NodePublishVolumeResponse{}

	// Fake request
	fakeReq := &csi.NodePublishVolumeRequest{
		VolumeId:         fakeVolID,
		PublishInfo:      map[string]string{"DevicePath": fakeDevicePath},
		TargetPath:       fakeTargetPath,
		VolumeCapability: nil,
		Readonly:         false,
	}

	// Invoke NodePublishVolume
	actualRes, err := fakeNs.NodePublishVolume(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to NodePublishVolume: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}

// Test NodeUnpublishVolume
func TestNodeUnpublishVolume(t *testing.T) {

	// mock MountMock
	mmock := new(mount.MountMock)

	// IsLikelyNotMountPointDetach(targetpath string) (bool, error)
	mmock.On("IsLikelyNotMountPointDetach", fakeTargetPath).Return(false, nil)
	// UnmountPath(mountPath string) error
	mmock.On("UnmountPath", fakeTargetPath).Return(nil)
	mount.MInstance = mmock

	// Init assert
	assert := assert.New(t)

	// Expected Result
	expectedRes := &csi.NodeUnpublishVolumeResponse{}

	// Fake request
	fakeReq := &csi.NodeUnpublishVolumeRequest{
		VolumeId:   fakeVolID,
		TargetPath: fakeTargetPath,
	}

	// Invoke NodeUnpublishVolume
	actualRes, err := fakeNs.NodeUnpublishVolume(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to NodeUnpublishVolume: %v", err)
	}

	// Assert
	assert.Equal(expectedRes, actualRes)
}
