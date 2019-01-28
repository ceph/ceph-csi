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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"

	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/nsenter"
	"k8s.io/utils/exec"
)

// PluginFolder defines the location of rbdplugin
const (
	PluginFolder      = "/var/lib/kubelet/plugins/csi-rbdplugin"
	rbdDefaultAdminID = "admin"
	rbdDefaultUserID  = rbdDefaultAdminID
)

type Driver struct {
	cd *csicommon.CSIDriver

	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

var (
	version = "1.0.0"
)

func GetDriver() *Driver {
	return &Driver{}
}

func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

func NewControllerServer(d *csicommon.CSIDriver, cachePersister util.CachePersister) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		MetadataStore:           cachePersister,
	}
}

func NewNodeServer(d *csicommon.CSIDriver, containerized bool) (*NodeServer, error) {
	mounter := mount.New("")
	if containerized {
		ne, err := nsenter.NewNsenter(nsenter.DefaultHostRootFsPath, exec.New())
		if err != nil {
			return nil, err
		}
		mounter = mount.NewNsenterMounter("", ne)
	}
	return &NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mounter,
	}, nil
}

func (r *Driver) Run(driverName, nodeID, endpoint string, containerized bool, cachePersister util.CachePersister) {
	var err error
	glog.Infof("Driver: %v version: %v", driverName, version)

	// Initialize default library driver
	r.cd = csicommon.NewCSIDriver(driverName, version, nodeID)
	if r.cd == nil {
		glog.Fatalln("Failed to initialize CSI Driver.")
	}
	r.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	})
	r.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})

	// Create GRPC servers
	r.ids = NewIdentityServer(r.cd)
	r.ns, err = NewNodeServer(r.cd, containerized)
	if err != nil {
		glog.Fatalf("failed to start node server, err %v\n", err)
	}

	r.cs = NewControllerServer(r.cd, cachePersister)
	r.cs.LoadExDataFromMetadataStore()

	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, r.ids, r.cs, r.ns)
	s.Wait()
}
