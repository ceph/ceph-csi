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

package hostpath

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type hostPath struct {
	driver *csicommon.CSIDriver

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

var (
	hostPathDriver *hostPath
	version        = csi.Version{
		Minor: 1,
	}
)

func GetSupportedVersions() []*csi.Version {
	return []*csi.Version{&version}
}

func GetHostPathDriver() *hostPath {
	return &hostPath{}
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

func (hp *hostPath) Run(driverName, nodeID, endpoint string) {
	glog.Infof("Driver: %v version: %v", driverName, GetVersionString(&version))

	// Initialize default library driver
	hp.driver = csicommon.NewCSIDriver(driverName, &version, GetSupportedVersions(), nodeID)
	hp.driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME})
	hp.driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})

	// Create GRPC servers
	hp.ids = NewIdentityServer(hp.driver)
	hp.ns = NewNodeServer(hp.driver)
	hp.cs = NewControllerServer(hp.driver)

	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, hp.ids, hp.cs, hp.ns)
	s.Wait()
}
