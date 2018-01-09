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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"k8s.io/client-go/kubernetes"
)

// PluginFolder defines the location of rbdplugin
const (
	PluginFolder = "/var/lib/kubelet/plugins/rbdplugin"
)

type rbd struct {
	driver *csicommon.CSIDriver

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

var (
	rbdDriver *rbd
	version   = csi.Version{
		Minor: 1,
	}
)

func GetSupportedVersions() []*csi.Version {
	return []*csi.Version{&version}
}

func GetRBDDriver() *rbd {
	return &rbd{}
}

func NewIdentityServer(d *csicommon.CSIDriver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

func NewControllerServer(d *csicommon.CSIDriver, clientSet *kubernetes.Clientset) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		clientSet:               clientSet,
	}
}

func NewNodeServer(d *csicommon.CSIDriver, clientSet *kubernetes.Clientset) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		clientSet:         clientSet,
	}
}

func (rbd *rbd) Run(driverName, nodeID, endpoint string, clientSet *kubernetes.Clientset) {
	glog.Infof("Driver: %v version: %v", driverName, GetVersionString(&version))

	// Initialize default library driver
	rbd.driver = csicommon.NewCSIDriver(driverName, &version, GetSupportedVersions(), nodeID)
	if rbd.driver == nil {
		glog.Fatalln("Failed to initialize CSI Driver.")
	}
	rbd.driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	})
	rbd.driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})

	// Create GRPC servers
	rbd.ids = NewIdentityServer(rbd.driver)
	rbd.ns = NewNodeServer(rbd.driver, clientSet)
	rbd.cs = NewControllerServer(rbd.driver, clientSet)
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, rbd.ids, rbd.cs, rbd.ns)
	s.Wait()
}
