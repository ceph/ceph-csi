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
	"fmt"

	casrbd "github.com/ceph/ceph-csi/internal/csi-addons/rbd"
	csiaddons "github.com/ceph/ceph-csi/internal/csi-addons/server"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	mount "k8s.io/mount-utils"
)

const (
	// volIDVersion is the version number of volume ID encoding scheme.
	volIDVersion uint16 = 1
)

// Driver contains the default identity,node and controller struct.
type Driver struct {
	cd *csicommon.CSIDriver

	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
	rs  *ReplicationServer

	// cas is the CSIAddonsServer where CSI-Addons services are handled
	cas *csiaddons.CSIAddonsServer
}

var (

	// CSIInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
	// ceph clusters across CSI instances, to differentiate omap names per CSI instance.
	CSIInstanceID = "default"

	// volJournal and snapJournal are used to maintain RADOS based journals for CO generated
	// VolumeName to backing RBD images.
	volJournal  *journal.Config
	snapJournal *journal.Config
	// rbdHardMaxCloneDepth is the hard limit for maximum number of nested volume clones that are taken before flatten
	// occurs.
	rbdHardMaxCloneDepth uint

	// rbdSoftMaxCloneDepth is the soft limit for maximum number of nested volume clones that are taken before flatten
	// occurs.
	rbdSoftMaxCloneDepth              uint
	maxSnapshotsOnImage               uint
	minSnapshotsOnImageToStartFlatten uint
	skipForceFlatten                  bool
)

// NewDriver returns new rbd driver.
func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for rbd CSI driver.
func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a controller server for rbd CSI driver.
func NewControllerServer(d *csicommon.CSIDriver) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		VolumeLocks:             util.NewVolumeLocks(),
		SnapshotLocks:           util.NewVolumeLocks(),
		OperationLocks:          util.NewOperationLock(),
	}
}

func NewReplicationServer(c *ControllerServer) *ReplicationServer {
	return &ReplicationServer{ControllerServer: c}
}

// NewNodeServer initialize a node server for rbd CSI driver.
func NewNodeServer(d *csicommon.CSIDriver, t string, topology map[string]string) (*NodeServer, error) {
	mounter := mount.New("")

	return &NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d, t, topology),
		mounter:           mounter,
		VolumeLocks:       util.NewVolumeLocks(),
	}, nil
}

// Run start a non-blocking grpc controller,node and identityserver for
// rbd CSI driver which can serve multiple parallel requests.
//
// This also configures and starts a new CSI-Addons service, by calling
// setupCSIAddonsServer().
func (r *Driver) Run(conf *util.Config) {
	var err error
	var topology map[string]string

	// Use passed in instance ID, if provided for omap suffix naming
	if conf.InstanceID != "" {
		CSIInstanceID = conf.InstanceID
	}

	// update clone soft and hard limit
	rbdHardMaxCloneDepth = conf.RbdHardMaxCloneDepth
	rbdSoftMaxCloneDepth = conf.RbdSoftMaxCloneDepth
	skipForceFlatten = conf.SkipForceFlatten
	maxSnapshotsOnImage = conf.MaxSnapshotsOnImage
	minSnapshotsOnImageToStartFlatten = conf.MinSnapshotsOnImage
	// Create instances of the volume and snapshot journal
	volJournal = journal.NewCSIVolumeJournal(CSIInstanceID)
	snapJournal = journal.NewCSISnapshotJournal(CSIInstanceID)

	// configre CSI-Addons server and components
	err = r.setupCSIAddonsServer(conf)
	if err != nil {
		log.FatalLogMsg(err.Error())
	}

	// Initialize default library driver
	r.cd = csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID)
	if r.cd == nil {
		log.FatalLogMsg("Failed to initialize CSI Driver.")
	}
	if conf.IsControllerServer || !conf.IsNodeServer {
		r.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
			csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		})
		// We only support the multi-writer option when using block, but it's a supported capability for the plugin in
		// general
		// In addition, we want to add the remaining modes like MULTI_NODE_READER_ONLY,
		// MULTI_NODE_SINGLE_WRITER etc, but need to do some verification of RO modes first
		// will work those as follow-up features
		r.cd.AddVolumeCapabilityAccessModes(
			[]csi.VolumeCapability_AccessMode_Mode{
				csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			})
	}

	// Create GRPC servers
	r.ids = NewIdentityServer(r.cd)

	if conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		r.ns, err = NewNodeServer(r.cd, conf.Vtype, topology)
		if err != nil {
			log.FatalLogMsg("failed to start node server, err %v\n", err)
		}
		var attr string
		attr, err = getKrbdSupportedFeatures()
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		krbdFeatures, err = hexStringToInteger(attr)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
	}

	if conf.IsControllerServer {
		r.cs = NewControllerServer(r.cd)
		r.rs = NewReplicationServer(r.cs)
	}
	if !conf.IsControllerServer && !conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		r.ns, err = NewNodeServer(r.cd, conf.Vtype, topology)
		if err != nil {
			log.FatalLogMsg("failed to start node server, err %v\n", err)
		}
		r.cs = NewControllerServer(r.cd)
	}

	s := csicommon.NewNonBlockingGRPCServer()
	srv := csicommon.Servers{
		IS: r.ids,
		CS: r.cs,
		NS: r.ns,
		// Register the replication controller to expose replication
		// operations.
		RS: r.rs,
	}
	s.Start(conf.Endpoint, conf.HistogramOption, srv, conf.EnableGRPCMetrics)
	if conf.EnableGRPCMetrics {
		log.WarningLogMsg("EnableGRPCMetrics is deprecated")
		go util.StartMetricsServer(conf)
	}

	r.startProfiling(conf)

	if conf.IsNodeServer {
		go func() {
			// TODO: move the healer to csi-addons
			err := runVolumeHealer(r.ns, conf)
			if err != nil {
				log.ErrorLogMsg("healer had failures, err %v\n", err)
			}
		}()
	}
	s.Wait()
}

// setupCSIAddonsServer creates a new CSI-Addons Server on the given (URL)
// endpoint. The supported CSI-Addons operations get registered as their own
// services.
func (r *Driver) setupCSIAddonsServer(conf *util.Config) error {
	var err error

	r.cas, err = csiaddons.NewCSIAddonsServer(conf.CSIAddonsEndpoint)
	if err != nil {
		return fmt.Errorf("failed to create CSI-Addons server: %w", err)
	}

	// register services
	is := casrbd.NewIdentityServer(conf)
	r.cas.RegisterService(is)

	// start the server, this does not block, it runs a new go-routine
	err = r.cas.Start()
	if err != nil {
		return fmt.Errorf("failed to start CSI-Addons server: %w", err)
	}

	return nil
}

// startProfiling checks which profiling options are enabled in the config and
// starts the required profiling services.
func (r *Driver) startProfiling(conf *util.Config) {
	if conf.EnableProfiling {
		if !conf.EnableGRPCMetrics {
			go util.StartMetricsServer(conf)
		}
		log.DebugLogMsg("Registering profiling handler")
		go util.EnableProfiling()
	}
}
