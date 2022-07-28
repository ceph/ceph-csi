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
	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Driver contains the default identity,node and controller struct.
type Driver struct {
	cd *csicommon.CSIDriver

	is *IdentityServer
	ns *NodeServer
	cs *ControllerServer
}

// CSIInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
// ceph clusters across CSI instances, to differentiate omap names per CSI instance.
var CSIInstanceID = "default"

// NewDriver returns new ceph driver.
func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for ceph CSI driver.
func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a controller server for ceph CSI driver.
func NewControllerServer(d *csicommon.CSIDriver) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		VolumeLocks:             util.NewVolumeLocks(),
		SnapshotLocks:           util.NewVolumeLocks(),
		OperationLocks:          util.NewOperationLock(),
	}
}

// NewNodeServer initialize a node server for ceph CSI driver.
func NewNodeServer(
	d *csicommon.CSIDriver,
	t string,
	topology map[string]string,
	kernelMountOptions string,
	fuseMountOptions string,
) *NodeServer {
	return &NodeServer{
		DefaultNodeServer:  csicommon.NewDefaultNodeServer(d, t, topology),
		VolumeLocks:        util.NewVolumeLocks(),
		kernelMountOptions: kernelMountOptions,
		fuseMountOptions:   fuseMountOptions,
	}
}

// Run start a non-blocking grpc controller,node and identityserver for
// ceph CSI driver which can serve multiple parallel requests.
func (fs *Driver) Run(conf *util.Config) {
	var err error
	var topology map[string]string

	// Configuration
	if err = mounter.LoadAvailableMounters(conf); err != nil {
		log.FatalLogMsg("cephfs: failed to load ceph mounters: %v", err)
	}

	// Use passed in instance ID, if provided for omap suffix naming
	if conf.InstanceID != "" {
		CSIInstanceID = conf.InstanceID
	}
	// Create an instance of the volume journal
	store.VolJournal = journal.NewCSIVolumeJournalWithNamespace(CSIInstanceID, fsutil.RadosNamespace)

	store.SnapJournal = journal.NewCSISnapshotJournalWithNamespace(CSIInstanceID, fsutil.RadosNamespace)
	// Initialize default library driver

	fs.cd = csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID)
	if fs.cd == nil {
		log.FatalLogMsg("failed to initialize CSI driver")
	}

	if conf.IsControllerServer || !conf.IsNodeServer {
		fs.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
			csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
			csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
		})

		fs.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		})
	}
	// Create gRPC servers

	fs.is = NewIdentityServer(fs.cd)

	if conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		fs.ns = NewNodeServer(fs.cd, conf.Vtype, topology, conf.KernelMountOptions, conf.FuseMountOptions)
	}

	if conf.IsControllerServer {
		fs.cs = NewControllerServer(fs.cd)
		fs.cs.ClusterName = conf.ClusterName
		fs.cs.SetMetadata = conf.SetMetadata
	}
	if !conf.IsControllerServer && !conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		fs.ns = NewNodeServer(fs.cd, conf.Vtype, topology, conf.KernelMountOptions, conf.FuseMountOptions)
		fs.cs = NewControllerServer(fs.cd)
	}

	server := csicommon.NewNonBlockingGRPCServer()
	srv := csicommon.Servers{
		IS: fs.is,
		CS: fs.cs,
		NS: fs.ns,
		// passing nil for replication server as cephFS does not support mirroring.
		RS: nil,
	}
	server.Start(conf.Endpoint, conf.HistogramOption, srv, conf.EnableGRPCMetrics)
	if conf.EnableGRPCMetrics {
		log.WarningLogMsg("EnableGRPCMetrics is deprecated")
		go util.StartMetricsServer(conf)
	}
	if conf.EnableProfiling {
		if !conf.EnableGRPCMetrics {
			go util.StartMetricsServer(conf)
		}
		log.DebugLogMsg("Registering profiling handler")
		go util.EnableProfiling()
	}
	server.Wait()
}
