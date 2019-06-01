/*
Copyright 2018 The Ceph-CSI Authors.

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
	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/utils/exec"
	"k8s.io/utils/nsenter"
)

const (
	// volIDVersion is the version number of volume ID encoding scheme
	volIDVersion uint16 = 1

	// csiConfigFile is the location of the CSI config file
	csiConfigFile = "/etc/ceph-csi-config/config.json"
)

// Driver contains the default identity,node and controller struct
type Driver struct {
	cd *csicommon.CSIDriver

	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

var (
	version = "1.0.0"

	// PluginFolder defines the location of ceph plugin
	PluginFolder = "/var/lib/kubelet/plugins/"

	// CSIInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
	// ceph clusters across CSI instances, to differentiate omap names per CSI instance
	CSIInstanceID = "default"

	// volJournal and snapJournal are used to maintain RADOS based journals for CO generated
	// VolumeName to backing RBD images
	volJournal  *util.CSIJournal
	snapJournal *util.CSIJournal
)

// NewDriver returns new rbd driver
func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for rbd CSI driver
func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a controller server for rbd CSI driver
func NewControllerServer(d *csicommon.CSIDriver) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
}

// NewNodeServer initialize a node server for rbd CSI driver.
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

// Run start a non-blocking grpc controller,node and identityserver for
// rbd CSI driver which can serve multiple parallel requests
func (r *Driver) Run(driverName, nodeID, endpoint, instanceID string, containerized bool) {
	var err error

	klog.Infof("Driver: %v version: %v", driverName, version)

	// Create ceph.conf for use with CLI commands
	if err = util.WriteCephConfig(); err != nil {
		klog.Fatalf("failed to write ceph configuration file (%v)", err)
	}

	// Use passed in instance ID, if provided for omap suffix naming
	if instanceID != "" {
		CSIInstanceID = instanceID
	}

	// Get an instance of the volume and snapshot journal keys
	volJournal = util.NewCSIVolumeJournal()
	snapJournal = util.NewCSISnapshotJournal()

	// Update keys with CSI instance suffix
	volJournal.SetCSIDirectorySuffix(CSIInstanceID)
	snapJournal.SetCSIDirectorySuffix(CSIInstanceID)

	// Initialize default library driver
	r.cd = csicommon.NewCSIDriver(driverName, version, nodeID)
	if r.cd == nil {
		klog.Fatalln("Failed to initialize CSI Driver.")
	}
	r.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	})

	// We only support the multi-writer option when using block, but it's a supported capability for the plugin in general
	// In addition, we want to add the remaining modes like MULTI_NODE_READER_ONLY,
	// MULTI_NODE_SINGLE_WRITER etc, but need to do some verification of RO modes first
	// will work those as follow up features
	r.cd.AddVolumeCapabilityAccessModes(
		[]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})

	// Create GRPC servers
	r.ids = NewIdentityServer(r.cd)
	r.ns, err = NewNodeServer(r.cd, containerized)
	if err != nil {
		klog.Fatalf("failed to start node server, err %v\n", err)
	}

	r.cs = NewControllerServer(r.cd)

	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, r.ids, r.cs, r.ns)
	s.Wait()
}
