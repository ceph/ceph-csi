/*
Copyright 2021 The Ceph-CSI Authors.

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
	"context"

	"github.com/csi-addons/spec/lib/go/identity"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/ceph/ceph-csi/internal/util"
)

// IdentityServer struct of rbd CSI driver with supported methods of CSI
// identity server spec.
type IdentityServer struct {
	*identity.UnimplementedIdentityServer

	config *util.Config
}

// NewIdentityServer creates a new IdentityServer which handles the Identity
// Service requests from the CSI-Addons specification.
func NewIdentityServer(config *util.Config) *IdentityServer {
	return &IdentityServer{
		config: config,
	}
}

func (is *IdentityServer) RegisterService(server grpc.ServiceRegistrar) {
	identity.RegisterIdentityServer(server, is)
}

// GetIdentity returns available capabilities of the rbd driver.
func (is *IdentityServer) GetIdentity(
	ctx context.Context,
	req *identity.GetIdentityRequest,
) (*identity.GetIdentityResponse, error) {
	// only include Name and VendorVersion, Manifest is optional
	res := &identity.GetIdentityResponse{
		Name:          is.config.DriverName,
		VendorVersion: util.DriverVersion,
	}

	return res, nil
}

// GetCapabilities returns available capabilities of the rbd driver.
func (is *IdentityServer) GetCapabilities(
	ctx context.Context,
	req *identity.GetCapabilitiesRequest,
) (*identity.GetCapabilitiesResponse, error) {
	// build the list of capabilities, depending on the config
	caps := make([]*identity.Capability, 0)

	if is.config.IsControllerServer {
		// we're running as a CSI Controller service
		caps = append(caps,
			&identity.Capability{
				Type: &identity.Capability_Service_{
					Service: &identity.Capability_Service{
						Type: identity.Capability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			&identity.Capability{
				Type: &identity.Capability_ReclaimSpace_{
					ReclaimSpace: &identity.Capability_ReclaimSpace{
						Type: identity.Capability_ReclaimSpace_OFFLINE,
					},
				},
			}, &identity.Capability{
				Type: &identity.Capability_NetworkFence_{
					NetworkFence: &identity.Capability_NetworkFence{
						Type: identity.Capability_NetworkFence_NETWORK_FENCE,
					},
				},
			}, &identity.Capability{
				Type: &identity.Capability_VolumeReplication_{
					VolumeReplication: &identity.Capability_VolumeReplication{
						Type: identity.Capability_VolumeReplication_VOLUME_REPLICATION,
					},
				},
			})
	}

	if is.config.IsNodeServer {
		// we're running as a CSI node-plugin service
		caps = append(caps,
			&identity.Capability{
				Type: &identity.Capability_Service_{
					Service: &identity.Capability_Service{
						Type: identity.Capability_Service_NODE_SERVICE,
					},
				},
			},
			&identity.Capability{
				Type: &identity.Capability_ReclaimSpace_{
					ReclaimSpace: &identity.Capability_ReclaimSpace{
						Type: identity.Capability_ReclaimSpace_ONLINE,
					},
				},
			})
	}

	res := &identity.GetCapabilitiesResponse{
		Capabilities: caps,
	}

	return res, nil
}

// Probe is called by the CO plugin to validate that the CSI-Addons Node is
// still healthy.
func (is *IdentityServer) Probe(
	ctx context.Context,
	req *identity.ProbeRequest,
) (*identity.ProbeResponse, error) {
	// there is nothing that would cause a delay in getting ready
	res := &identity.ProbeResponse{
		Ready: &wrapperspb.BoolValue{Value: true},
	}

	return res, nil
}
