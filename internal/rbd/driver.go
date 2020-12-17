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
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	mount "k8s.io/mount-utils"
)

const (
	// volIDVersion is the version number of volume ID encoding scheme
	volIDVersion uint16 = 1
)

// Driver contains the default identity,node and controller struct.
type Driver struct {
	cd *csicommon.CSIDriver

	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

var (

	// CSIInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
	// ceph clusters across CSI instances, to differentiate omap names per CSI instance
	CSIInstanceID = "default"

	// volJournal and snapJournal are used to maintain RADOS based journals for CO generated
	// VolumeName to backing RBD images
	volJournal  *journal.Config
	snapJournal *journal.Config
	// rbdHardMaxCloneDepth is the hard limit for maximum number of nested volume clones that are taken before a flatten occurs
	rbdHardMaxCloneDepth uint

	// rbdSoftMaxCloneDepth is the soft limit for maximum number of nested volume clones that are taken before a flatten occurs
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
func (r *Driver) Run(conf *util.Config) {
	var err error
	var topology map[string]string

	// Create ceph.conf for use with CLI commands
	if err = util.WriteCephConfig(); err != nil {
		util.FatalLogMsg("failed to write ceph configuration file (%v)", err)
	}

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

	// Initialize default library driver
	r.cd = csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID)
	if r.cd == nil {
		util.FatalLogMsg("Failed to initialize CSI Driver.")
	}
	if conf.IsControllerServer || !conf.IsNodeServer {
		r.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
			csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		})
		// We only support the multi-writer option when using block, but it's a supported capability for the plugin in general
		// In addition, we want to add the remaining modes like MULTI_NODE_READER_ONLY,
		// MULTI_NODE_SINGLE_WRITER etc, but need to do some verification of RO modes first
		// will work those as follow up features
		r.cd.AddVolumeCapabilityAccessModes(
			[]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})
	}

	// Create GRPC servers
	r.ids = NewIdentityServer(r.cd)

	if conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			util.FatalLogMsg(err.Error())
		}
		r.ns, err = NewNodeServer(r.cd, conf.Vtype, topology)
		if err != nil {
			util.FatalLogMsg("failed to start node server, err %v\n", err)
		}
	}

	if conf.IsControllerServer {
		r.cs = NewControllerServer(r.cd)
	}
	if !conf.IsControllerServer && !conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			util.FatalLogMsg(err.Error())
		}
		r.ns, err = NewNodeServer(r.cd, conf.Vtype, topology)
		if err != nil {
			util.FatalLogMsg("failed to start node server, err %v\n", err)
		}
		r.cs = NewControllerServer(r.cd)
	}

	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(conf.Endpoint, conf.HistogramOption, r.ids, r.cs, r.ns, conf.EnableGRPCMetrics)
	if conf.EnableGRPCMetrics {
		util.WarningLogMsg("EnableGRPCMetrics is deprecated")
		go util.StartMetricsServer(conf)
	}
	s.Wait()
}
