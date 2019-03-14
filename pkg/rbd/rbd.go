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
	"github.com/golang/glog"

	"github.com/ceph/ceph-csi/pkg/util"
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"

	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/nsenter"
	"k8s.io/utils/exec"
)

const (
	rbdDefaultAdminId = "admin"
	rbdDefaultUserId  = rbdDefaultAdminId
)

var (
	// PluginFolder defines the location of rbdplugin
	PluginFolder = "/var/lib/kubelet/plugins/"
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
	version   = "0.3.0"
)

func GetRBDDriver() *rbd {
	return &rbd{}
}

func NewIdentityServer(d *csicommon.CSIDriver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

func NewControllerServer(d *csicommon.CSIDriver, cachePersister util.CachePersister) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		MetadataStore:           cachePersister,
	}
}

func NewNodeServer(d *csicommon.CSIDriver, containerized bool) (*nodeServer, error) {
	mounter := mount.New("")
	if containerized {
		ne, err := nsenter.NewNsenter(nsenter.DefaultHostRootFsPath, exec.New())
		if err != nil {
			return nil, err
		}
		mounter = mount.NewNsenterMounter("", ne)
	}
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mounter,
	}, nil
}

func (rbd *rbd) Run(driverName, nodeID, endpoint string, containerized bool, cachePersister util.CachePersister) {
	var err error
	glog.Infof("Driver: %v version: %v", driverName, version)

	// Initialize default library driver
	rbd.driver = csicommon.NewCSIDriver(driverName, version, nodeID)
	if rbd.driver == nil {
		glog.Fatalln("Failed to initialize CSI Driver.")
	}
	rbd.driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	})
	rbd.driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})

	// Create GRPC servers
	rbd.ids = NewIdentityServer(rbd.driver)
	rbd.ns, err = NewNodeServer(rbd.driver, containerized)
	if err != nil {
		glog.Fatalf("failed to start node server, err %v \n", err)
	}

	rbd.cs = NewControllerServer(rbd.driver, cachePersister)
	rbd.cs.LoadExDataFromMetadataStore()

	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, rbd.ids, rbd.cs, rbd.ns)
	s.Wait()
}
