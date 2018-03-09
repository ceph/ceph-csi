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
	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const (
	PluginFolder = "/var/lib/kubelet/plugins/cephfsplugin"
)

type cephfsDriver struct {
	driver *csicommon.CSIDriver

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer

	caps   []*csi.VolumeCapability_AccessMode
	cscaps []*csi.ControllerServiceCapability
}

var (
	driver  *cephfsDriver
	version = csi.Version{
		Minor: 1,
	}
)

func GetSupportedVersions() []*csi.Version {
	return []*csi.Version{&version}
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

func (fs *cephfsDriver) Run(driverName, nodeId, endpoint string) {
	glog.Infof("Driver: %v version: %v", driverName, GetVersionString(&version))

	// Initialize default library driver

	fs.driver = csicommon.NewCSIDriver(driverName, &version, GetSupportedVersions(), nodeId)
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

	fs.ids = NewIdentityServer(fs.driver)
	fs.ns = NewNodeServer(fs.driver)
	fs.cs = NewControllerServer(fs.driver)

	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(endpoint, fs.ids, fs.cs, fs.ns)
	server.Wait()
}
