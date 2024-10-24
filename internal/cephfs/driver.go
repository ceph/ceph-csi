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
	"fmt"

	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	casceph "github.com/ceph/ceph-csi/internal/csi-addons/cephfs"
	csiaddons "github.com/ceph/ceph-csi/internal/csi-addons/server"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	hc "github.com/ceph/ceph-csi/internal/health-checker"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Driver contains the default identity,node and controller struct.
type Driver struct {
	cd *csicommon.CSIDriver

	is *IdentityServer
	ns *NodeServer
	cs *ControllerServer
	// cas is the CSIAddonsServer where CSI-Addons services are handled
	cas *csiaddons.CSIAddonsServer
}

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
		VolumeGroupLocks:        util.NewVolumeLocks(),
		OperationLocks:          util.NewOperationLock(),
	}
}

// NewNodeServer initialize a node server for ceph CSI driver.
func NewNodeServer(
	d *csicommon.CSIDriver,
	t string,
	kernelMountOptions string,
	fuseMountOptions string,
	nodeLabels, topology, crushLocationMap map[string]string,
) *NodeServer {
	cliReadAffinityMapOptions := util.ConstructReadAffinityMapOption(crushLocationMap)
	ns := &NodeServer{
		DefaultNodeServer:  csicommon.NewDefaultNodeServer(d, t, cliReadAffinityMapOptions, topology, nodeLabels),
		VolumeLocks:        util.NewVolumeLocks(),
		kernelMountOptions: kernelMountOptions,
		fuseMountOptions:   fuseMountOptions,
		healthChecker:      hc.NewHealthCheckManager(),
	}

	return ns
}

// Run start a non-blocking grpc controller,node and identityserver for
// ceph CSI driver which can serve multiple parallel requests.
func (fs *Driver) Run(conf *util.Config) {
	var (
		err                                    error
		nodeLabels, topology, crushLocationMap map[string]string
	)

	// Configuration
	if err = mounter.LoadAvailableMounters(conf); err != nil {
		log.FatalLogMsg("cephfs: failed to load ceph mounters: %v", err)
	}

	// Use passed in radosNamespace, if provided for storing CSI specific objects and keys.
	if conf.RadosNamespaceCephFS != "" {
		fsutil.RadosNamespace = conf.RadosNamespaceCephFS
	}

	if conf.IsNodeServer && k8s.RunsOnKubernetes() {
		nodeLabels, err = k8s.GetNodeLabels(conf.NodeID)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
	}

	if conf.EnableReadAffinity {
		crushLocationMap = util.GetCrushLocationMap(conf.CrushLocationLabels, nodeLabels)
	}

	// Create an instance of the volume journal
	store.VolJournal = journal.NewCSIVolumeJournalWithNamespace(conf.InstanceID, fsutil.RadosNamespace)

	store.SnapJournal = journal.NewCSISnapshotJournalWithNamespace(conf.InstanceID, fsutil.RadosNamespace)

	store.VolumeGroupJournal = journal.NewCSIVolumeGroupJournalWithNamespace(
		conf.InstanceID,
		fsutil.RadosNamespace)
	// Initialize default library driver

	fs.cd = csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID, conf.InstanceID)
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
			csi.ControllerServiceCapability_RPC_MODIFY_VOLUME,
		})

		fs.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		})

		fs.cd.AddGroupControllerServiceCapabilities([]csi.GroupControllerServiceCapability_RPC_Type{
			csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
		})
	}
	// Create gRPC servers

	fs.is = NewIdentityServer(fs.cd)

	if conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		fs.ns = NewNodeServer(
			fs.cd, conf.Vtype,
			conf.KernelMountOptions, conf.FuseMountOptions,
			nodeLabels, topology, crushLocationMap,
		)
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
		fs.ns = NewNodeServer(
			fs.cd, conf.Vtype,
			conf.KernelMountOptions, conf.FuseMountOptions,
			nodeLabels, topology, crushLocationMap,
		)
		fs.cs = NewControllerServer(fs.cd)
	}

	// configure CSI-Addons server and components
	err = fs.setupCSIAddonsServer(conf)
	if err != nil {
		log.FatalLogMsg(err.Error())
	}

	server := csicommon.NewNonBlockingGRPCServer()
	srv := csicommon.Servers{
		IS: fs.is,
		CS: fs.cs,
		NS: fs.ns,
		GS: fs.cs,
	}
	server.Start(conf.Endpoint, srv, csicommon.MiddlewareServerOptionConfig{
		LogSlowOpInterval: conf.LogSlowOpInterval,
	})

	if conf.EnableProfiling {
		go util.StartMetricsServer(conf)
		log.DebugLogMsg("Registering profiling handler")
		go util.EnableProfiling()
	}
	server.Wait()
}

// setupCSIAddonsServer creates a new CSI-Addons Server on the given (URL)
// endpoint. The supported CSI-Addons operations get registered as their own
// services.
func (fs *Driver) setupCSIAddonsServer(conf *util.Config) error {
	var err error

	fs.cas, err = csiaddons.NewCSIAddonsServer(conf.CSIAddonsEndpoint)
	if err != nil {
		return fmt.Errorf("failed to create CSI-Addons server: %w", err)
	}

	// register services
	is := casceph.NewIdentityServer(conf)
	fs.cas.RegisterService(is)

	if conf.IsControllerServer {
		fcs := casceph.NewFenceControllerServer()
		fs.cas.RegisterService(fcs)
	}

	// start the server, this does not block, it runs a new go-routine
	err = fs.cas.Start(csicommon.MiddlewareServerOptionConfig{
		LogSlowOpInterval: conf.LogSlowOpInterval,
	})
	if err != nil {
		return fmt.Errorf("failed to start CSI-Addons server: %w", err)
	}

	return nil
}
