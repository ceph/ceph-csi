/*
Copyright 2025 The Ceph-CSI Authors.

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

package driver

import (
	"github.com/container-storage-interface/spec/lib/go/csi"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/driver"
	"github.com/ceph/ceph-csi/internal/nvmeof/controller"
	"github.com/ceph/ceph-csi/internal/nvmeof/identity"
	"github.com/ceph/ceph-csi/internal/nvmeof/nodeserver"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// nvmeofDriver provides the entry point for the NVMe-oF CSI driver.
// The actual implementation is delegated to identity, controller, and node servers.
type nvmeofDriver struct{}

// NewDriver returns new NVMe-oF driver.
func NewDriver() driver.Driver {
	return &nvmeofDriver{}
}

// assert that nvmeofDriver implements the Driver interface.
var _ driver.Driver = &nvmeofDriver{}

// Run starts the NVMe-oF CSI driver.
func (d *nvmeofDriver) Run(conf *util.Config) {
	// TODO: move rbd initialization to a common function
	// update clone soft and hard limit
	rbd.SetGlobalInt("rbdHardMaxCloneDepth", conf.RbdHardMaxCloneDepth)
	rbd.SetGlobalInt("rbdSoftMaxCloneDepth", conf.RbdSoftMaxCloneDepth)
	rbd.SetGlobalBool("skipForceFlatten", conf.SkipForceFlatten)
	rbd.SetGlobalInt("maxSnapshotsOnImage", conf.MaxSnapshotsOnImage)
	rbd.SetGlobalInt("minSnapshotsOnImageToStartFlatten", conf.MinSnapshotsOnImage)
	// Create instances of the volume and snapshot journal
	rbd.InitJournals(conf.InstanceID)
	// Initialize CSI driver
	cd := csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID, conf.InstanceID, conf.EnableFencing)
	if cd == nil {
		log.FatalLogMsg("failed to initialize CSI driver")
	}

	// Set capabilities
	if conf.IsControllerServer || !conf.IsNodeServer {
		cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		})

		cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
		})
	}

	// Create gRPC servers
	server := csicommon.NewNonBlockingGRPCServer()
	srv := &csicommon.Servers{
		IS: identity.NewIdentityServer(cd),
	}

	switch {
	case conf.IsNodeServer:
		ns, err := nodeserver.NewNodeServer(cd, conf.NodeID, conf.Vtype)
		if err != nil {
			log.FatalLogMsg("failed to initialize node server: %v", err)
		}
		srv.NS = ns
	case conf.IsControllerServer:
		cs, err := controller.NewControllerServer(cd)
		if err != nil {
			log.FatalLogMsg("failed to initialize controller server: %v", err)
		}
		srv.CS = cs
	default:
		ns, err := nodeserver.NewNodeServer(cd, conf.NodeID, conf.Vtype)
		if err != nil {
			log.FatalLogMsg("failed to initialize node server: %v", err)
		}
		srv.NS = ns
		cs, err := controller.NewControllerServer(cd)
		if err != nil {
			log.FatalLogMsg("failed to initialize controller server: %v", err)
		}
		srv.CS = cs
	}

	server.Start(conf.Endpoint, srv, csicommon.MiddlewareServerOptionConfig{
		LogSlowOpInterval: conf.LogSlowOpInterval,
	})

	if conf.EnableProfiling {
		go util.StartMetricsServer(conf)
		go util.EnableProfiling()
	}

	server.Wait()
}
