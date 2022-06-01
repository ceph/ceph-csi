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

package identity

import (
	"context"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Server struct of ceph CSI driver with supported methods of CSI identity
// server spec.
type Server struct {
	*csicommon.DefaultIdentityServer
}

// NewIdentityServer initialize a identity server for ceph CSI driver.
func NewIdentityServer(d *csicommon.CSIDriver) *Server {
	return &Server{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// GetPluginCapabilities returns available capabilities of the ceph driver.
func (is *Server) GetPluginCapabilities(
	ctx context.Context,
	req *csi.GetPluginCapabilitiesRequest,
) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}
