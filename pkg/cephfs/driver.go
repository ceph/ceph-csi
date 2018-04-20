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
	"os"

	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const (
	PluginFolder = "/var/lib/kubelet/plugins/csi-cephfsplugin"
	Version      = "0.2.0"
)

type cephfsDriver struct {
	driver *csicommon.CSIDriver

	is *identityServer
	ns *nodeServer
	cs *controllerServer

	caps   []*csi.VolumeCapability_AccessMode
	cscaps []*csi.ControllerServiceCapability
}

var (
	driver               *cephfsDriver
	DefaultVolumeMounter string
)

func getVolumeMounterByProbing() string {
	if execCommandAndValidate("ceph-fuse", "--version") == nil {
		return volumeMounter_fuse
	} else {
		return volumeMounter_kernel
	}
}

func NewCephFSDriver() *cephfsDriver {
	return &cephfsDriver{}
}

func NewIdentityServer(d *csicommon.CSIDriver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

func NewControllerServer(d *csicommon.CSIDriver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
}

func NewNodeServer(d *csicommon.CSIDriver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
	}
}

func (fs *cephfsDriver) Run(driverName, nodeId, endpoint, volumeMounter string) {
	glog.Infof("Driver: %v version: %v", driverName, Version)

	// Configuration

	if err := os.MkdirAll(volumeCacheRoot, 0755); err != nil {
		glog.Fatalf("cephfs: failed to create %s: %v", volumeCacheRoot, err)
		return
	}

	if err := loadVolumeCache(); err != nil {
		glog.Errorf("cephfs: failed to read volume cache: %v", err)
	}

	if volumeMounter != "" {
		if err := validateMounter(volumeMounter); err != nil {
			glog.Fatalln(err)
		} else {
			DefaultVolumeMounter = volumeMounter
		}
	} else {
		DefaultVolumeMounter = getVolumeMounterByProbing()
	}

	glog.Infof("cephfs: setting default volume mounter to %s", DefaultVolumeMounter)

	// Initialize default library driver

	fs.driver = csicommon.NewCSIDriver(driverName, Version, nodeId)
	if fs.driver == nil {
		glog.Fatalln("Failed to initialize CSI driver")
	}

	fs.driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	})

	fs.driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	// Create gRPC servers

	fs.is = NewIdentityServer(fs.driver)
	fs.ns = NewNodeServer(fs.driver)
	fs.cs = NewControllerServer(fs.driver)

	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(endpoint, fs.is, fs.cs, fs.ns)
	server.Wait()
}
