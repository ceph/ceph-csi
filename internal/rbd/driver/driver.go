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

package rbddriver

import (
	"errors"
	"fmt"
	"os"

	casrbd "github.com/ceph/ceph-csi/internal/csi-addons/rbd"
	csiaddons "github.com/ceph/ceph-csi/internal/csi-addons/server"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Driver contains the default identity,node and controller struct.
type Driver struct {
	cd  *csicommon.CSIDriver
	ids *rbd.IdentityServer
	ns  *rbd.NodeServer
	cs  *rbd.ControllerServer

	// cas is the CSIAddonsServer where CSI-Addons services are handled
	cas *csiaddons.CSIAddonsServer
}

// NewDriver returns new rbd driver.
func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for rbd CSI driver.
func NewIdentityServer(d *csicommon.CSIDriver) *rbd.IdentityServer {
	return &rbd.IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a controller server for rbd CSI driver.
func NewControllerServer(d *csicommon.CSIDriver) *rbd.ControllerServer {
	return &rbd.ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		VolumeLocks:             util.NewVolumeLocks(),
		SnapshotLocks:           util.NewVolumeLocks(),
		OperationLocks:          util.NewOperationLock(),
	}
}

// NewNodeServer initialize a node server for rbd CSI driver.
func NewNodeServer(
	d *csicommon.CSIDriver,
	t string,
	nodeLabels, topology, crushLocationMap map[string]string,
) (*rbd.NodeServer, error) {
	cliReadAffinityMapOptions := util.ConstructReadAffinityMapOption(crushLocationMap)
	ns := rbd.NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d, t, cliReadAffinityMapOptions, topology, nodeLabels),
		VolumeLocks:       util.NewVolumeLocks(),
	}

	return &ns, nil
}

// Run start a non-blocking grpc controller,node and identityserver for
// rbd CSI driver which can serve multiple parallel requests.
//
// This also configures and starts a new CSI-Addons service, by calling
// setupCSIAddonsServer().
func (r *Driver) Run(conf *util.Config) {
	var (
		err                                    error
		nodeLabels, topology, crushLocationMap map[string]string
	)
	// update clone soft and hard limit
	rbd.SetGlobalInt("rbdHardMaxCloneDepth", conf.RbdHardMaxCloneDepth)
	rbd.SetGlobalInt("rbdSoftMaxCloneDepth", conf.RbdSoftMaxCloneDepth)
	rbd.SetGlobalBool("skipForceFlatten", conf.SkipForceFlatten)
	rbd.SetGlobalInt("maxSnapshotsOnImage", conf.MaxSnapshotsOnImage)
	rbd.SetGlobalInt("minSnapshotsOnImageToStartFlatten", conf.MinSnapshotsOnImage)
	// Create instances of the volume and snapshot journal
	rbd.InitJournals(conf.InstanceID)

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
				csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			})
	}

	if k8s.RunsOnKubernetes() {
		nodeLabels, err = k8s.GetNodeLabels(conf.NodeID)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
	}

	if conf.EnableReadAffinity {
		crushLocationMap = util.GetCrushLocationMap(conf.CrushLocationLabels, nodeLabels)
	}

	// Create GRPC servers
	r.ids = NewIdentityServer(r.cd)

	if conf.IsNodeServer {
		topology, err = util.GetTopologyFromDomainLabels(conf.DomainLabels, conf.NodeID, conf.DriverName)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		r.ns, err = NewNodeServer(r.cd, conf.Vtype, nodeLabels, topology, crushLocationMap)
		if err != nil {
			log.FatalLogMsg("failed to start node server, err %v\n", err)
		}
		var attr string
		attr, err = rbd.GetKrbdSupportedFeatures()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.FatalLogMsg(err.Error())
		}
		var krbdFeatures uint
		krbdFeatures, err = rbd.HexStringToInteger(attr)
		if err != nil {
			log.FatalLogMsg(err.Error())
		}
		rbd.SetGlobalInt("krbdFeatures", krbdFeatures)

		rbd.SetRbdNbdToolFeatures()
	}

	if conf.IsControllerServer {
		r.cs = NewControllerServer(r.cd)
		r.cs.ClusterName = conf.ClusterName
		r.cs.SetMetadata = conf.SetMetadata
	}

	// configure CSI-Addons server and components
	err = r.setupCSIAddonsServer(conf)
	if err != nil {
		log.FatalLogMsg(err.Error())
	}

	s := csicommon.NewNonBlockingGRPCServer()
	srv := csicommon.Servers{
		IS: r.ids,
		CS: r.cs,
		NS: r.ns,
	}
	s.Start(conf.Endpoint, srv)

	r.startProfiling(conf)

	if conf.IsNodeServer {
		go func() {
			// TODO: move the healer to csi-addons
			err := rbd.RunVolumeHealer(r.ns, conf)
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

	if conf.IsControllerServer {
		rs := casrbd.NewReclaimSpaceControllerServer()
		r.cas.RegisterService(rs)

		fcs := casrbd.NewFenceControllerServer()
		r.cas.RegisterService(fcs)

		rcs := casrbd.NewReplicationServer(NewControllerServer(r.cd))
		r.cas.RegisterService(rcs)
	}

	if conf.IsNodeServer {
		rs := casrbd.NewReclaimSpaceNodeServer()
		r.cas.RegisterService(rs)
	}

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
		go util.StartMetricsServer(conf)
		log.DebugLogMsg("Registering profiling handler")
		go util.EnableProfiling()
	}
}
