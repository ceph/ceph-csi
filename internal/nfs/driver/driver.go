/*
Copyright 2022 The Ceph-CSI Authors.

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
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/nfs/controller"
	"github.com/ceph/ceph-csi/internal/nfs/identity"
	"github.com/ceph/ceph-csi/internal/nfs/nodeserver"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Driver contains the default identity and controller struct.
type Driver struct{}

// NewDriver returns new ceph driver.
func NewDriver() *Driver {
	return &Driver{}
}

// Run start a non-blocking grpc controller,node and identityserver for
// ceph CSI driver which can serve multiple parallel requests.
func (fs *Driver) Run(conf *util.Config) {
	// Initialize default library driver
	cd := csicommon.NewCSIDriver(conf.DriverName, util.DriverVersion, conf.NodeID)
	if cd == nil {
		log.FatalLogMsg("failed to initialize CSI driver")
	}

	if conf.IsControllerServer || !conf.IsNodeServer {
		cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
			csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		})
		// VolumeCapabilities are validated by the CephFS Controller
		cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		})
	}

	// Create gRPC servers
	server := csicommon.NewNonBlockingGRPCServer()
	srv := csicommon.Servers{
		IS: identity.NewIdentityServer(cd),
	}

	switch {
	case conf.IsNodeServer:
		srv.NS = nodeserver.NewNodeServer(cd, conf.Vtype)
	case conf.IsControllerServer:
		srv.CS = controller.NewControllerServer(cd)
	default:
		srv.NS = nodeserver.NewNodeServer(cd, conf.Vtype)
		srv.CS = controller.NewControllerServer(cd)
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
