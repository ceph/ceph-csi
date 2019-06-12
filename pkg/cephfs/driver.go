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

package cephfs

import (
	"k8s.io/klog"

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/metrics"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (

	// version of ceph driver
	version = "1.0.0"
)

// PluginFolder defines the location of ceph plugin
var PluginFolder = "/var/lib/kubelet/plugins/"

// Driver contains the default identity,node and controller struct
type Driver struct {
	cd *csicommon.CSIDriver

	is *IdentityServer
	ns *NodeServer
	cs *ControllerServer
	ms *metrics.Server
}

var (
	// DefaultVolumeMounter for mounting volumes
	DefaultVolumeMounter string
)

// NewDriver returns new ceph driver
func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for ceph CSI driver
func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a controller server for ceph CSI driver
func NewControllerServer(d *csicommon.CSIDriver, cachePersister util.CachePersister) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		MetadataStore:           cachePersister,
	}
}

// NewNodeServer initialize a node server for ceph CSI driver.
func NewNodeServer(d *csicommon.CSIDriver) *NodeServer {
	return &NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
	}
}

// Run start a non-blocking grpc controller,node and identityserver for
// ceph CSI driver which can serve multiple parallel requests
func (fs *Driver) Run(driverName, nodeID, endpoint, volumeMounter, mountCacheDir string, cachePersister util.CachePersister) {
	klog.Infof("Driver: %v version: %v", driverName, version)

	// Configuration

	if err := loadAvailableMounters(); err != nil {
		klog.Fatalf("cephfs: failed to load ceph mounters: %v", err)
	}

	if volumeMounter != "" {
		if err := validateMounter(volumeMounter); err != nil {
			klog.Fatalln(err)
		} else {
			DefaultVolumeMounter = volumeMounter
		}
	} else {
		// Pick the first available mounter as the default one.
		// The choice is biased towards "fuse" in case both
		// ceph fuse and kernel mounters are available.
		DefaultVolumeMounter = availableMounters[0]
	}

	klog.Infof("cephfs: setting default volume mounter to %s", DefaultVolumeMounter)

	if err := writeCephConfig(); err != nil {
		klog.Fatalf("failed to write ceph configuration file: %v", err)
	}

	initVolumeMountCache(driverName, mountCacheDir, cachePersister)
	if mountCacheDir != "" {
		if err := remountCachedVolumes(); err != nil {
			klog.Warningf("failed to remount cached volumes: %v", err)
			//ignore remount fail
		}
	}
	// Initialize default library driver

	fs.cd = csicommon.NewCSIDriver(driverName, version, nodeID)
	if fs.cd == nil {
		klog.Fatalln("failed to initialize CSI driver")
	}

	fs.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	})

	fs.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	// Create gRPC servers

	fs.is = NewIdentityServer(fs.cd)
	fs.ns = NewNodeServer(fs.cd)

	fs.cs = NewControllerServer(fs.cd, cachePersister)
	fs.ms = metrics.NewMetricServer()

	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(endpoint, fs.is, fs.cs, fs.ns, fs.ms)
	server.Wait()
}
