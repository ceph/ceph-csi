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
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (

	// volIDVersion is the version number of volume ID encoding scheme
	volIDVersion uint16 = 1

	// csiConfigFile is the location of the CSI config file
	csiConfigFile = "/etc/ceph-csi-config/config.json"

	// RADOS namespace to store CSI specific objects and keys
	radosNamespace = "csi"
)

// PluginFolder defines the location of ceph plugin
var PluginFolder = ""

// Driver contains the default identity,node and controller struct
type Driver struct {
	cd *csicommon.CSIDriver

	is *IdentityServer
	ns *NodeServer
	cs *ControllerServer
}

var (
	// CSIInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
	// ceph clusters across CSI instances, to differentiate omap names per CSI instance
	CSIInstanceID = "default"

	// volJournal is used to maintain RADOS based journals for CO generated
	// VolumeName to backing CephFS subvolumes
	volJournal *util.CSIJournal
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
		VolumeLocks:             util.NewVolumeLocks(),
	}
}

// NewNodeServer initialize a node server for ceph CSI driver.
func NewNodeServer(d *csicommon.CSIDriver, t string) *NodeServer {
	return &NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d, t),
		VolumeLocks:       util.NewVolumeLocks(),
	}
}

// Run start a non-blocking grpc controller,node and identityserver for
// ceph CSI driver which can serve multiple parallel requests
func (fs *Driver) Run(conf *util.Config, cachePersister util.CachePersister) {
	// Configuration
	PluginFolder = conf.PluginPath

	if err := loadAvailableMounters(conf); err != nil {
		klog.Fatalf("cephfs: failed to load ceph mounters: %v", err)
	}

	if err := util.WriteCephConfig(); err != nil {
		klog.Fatalf("failed to write ceph configuration file: %v", err)
	}

	// Use passed in instance ID, if provided for omap suffix naming
	if conf.InstanceID != "" {
		CSIInstanceID = conf.InstanceID
	}
	// Get an instance of the volume journal
	volJournal = util.NewCSIVolumeJournal()

	// Update keys with CSI instance suffix
	volJournal.SetCSIDirectorySuffix(CSIInstanceID)

	// Update namespace for storing keys into a specific namespace on RADOS, in the CephFS
	// metadata pool
	volJournal.SetNamespace(radosNamespace)

	initVolumeMountCache(conf.DriverName, conf.MountCacheDir)
	if conf.MountCacheDir != "" {
		if err := remountCachedVolumes(); err != nil {
			klog.Warningf("failed to remount cached volumes: %v", err)
			// ignore remount fail
		}
	}
	// Initialize default library driver

	fs.cd = csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID)
	if fs.cd == nil {
		klog.Fatalln("failed to initialize CSI driver")
	}

	if conf.IsControllerServer || !conf.IsNodeServer {
		fs.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		})

		fs.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		})
	}
	// Create gRPC servers

	fs.is = NewIdentityServer(fs.cd)

	if conf.IsNodeServer {
		fs.ns = NewNodeServer(fs.cd, conf.Vtype)
	}

	if conf.IsControllerServer {
		fs.cs = NewControllerServer(fs.cd, cachePersister)
	}
	if !conf.IsControllerServer && !conf.IsNodeServer {
		fs.ns = NewNodeServer(fs.cd, conf.Vtype)
		fs.cs = NewControllerServer(fs.cd, cachePersister)
	}

	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(conf.Endpoint, conf.HistogramOption, fs.is, fs.cs, fs.ns, conf.EnableGRPCMetrics)
	if conf.EnableGRPCMetrics {
		go util.StartMetricsServer(conf)
	}
	server.Wait()
}
