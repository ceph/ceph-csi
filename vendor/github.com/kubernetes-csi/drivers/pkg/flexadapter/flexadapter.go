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

package flexadapter

import (
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type flexAdapter struct {
	driver *csicommon.CSIDriver

	flexDriver *flexVolumeDriver

	ns *nodeServer
	cs *controllerServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

var (
	version = "0.3.0"
)

func New() *flexAdapter {
	return &flexAdapter{}
}

func NewControllerServer(d *csicommon.CSIDriver, f *flexVolumeDriver) *controllerServer {
	return &controllerServer{
		flexDriver:              f,
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
}

func NewNodeServer(d *csicommon.CSIDriver, f *flexVolumeDriver) *nodeServer {
	return &nodeServer{
		flexDriver:        f,
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
	}
}

func (f *flexAdapter) Run(driverName, driverPath, nodeID, endpoint string) {
	var err error

	glog.Infof("Driver: %v version: %v", driverName, version)

	// Create flex volume driver
	f.flexDriver, err = NewFlexVolumeDriver(driverName, driverPath)
	if err != nil {
		glog.Errorf("Failed to initialize flex volume driver, error: %v", err.Error())
		os.Exit(1)
	}

	// Initialize default library driver
	f.driver = csicommon.NewCSIDriver(driverName, version, nodeID)
	if f.flexDriver.capabilities.Attach {
		f.driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME})
	}
	f.driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})

	// Create GRPC servers
	f.ns = NewNodeServer(f.driver, f.flexDriver)
	f.cs = NewControllerServer(f.driver, f.flexDriver)

	csicommon.RunControllerandNodePublishServer(endpoint, f.driver, f.cs, f.ns)
}
