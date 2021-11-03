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
)

// IdentityServer struct of rbd CSI driver with supported methods of CSI
// identity server spec.
type IdentityServer struct {
	*identity.UnimplementedIdentityServer
}

func (is *IdentityServer) RegisterService(server grpc.ServiceRegistrar) {
	identity.RegisterIdentityServer(server, is)
}

// GetIdentity returns available capabilities of the rbd driver.
func (is *IdentityServer) GetIdentity(
	ctx context.Context,
	req *identity.GetIdentityRequest) (*identity.GetIdentityResponse, error) {
	return nil, nil
}

// GetCapabilities returns available capabilities of the rbd driver.
func (is *IdentityServer) GetCapabilities(
	ctx context.Context,
	req *identity.GetCapabilitiesRequest) (*identity.GetCapabilitiesResponse, error) {
	return nil, nil
}

// GetCapabilities returns available capabilities of the rbd driver.
func (is *IdentityServer) Probe(
	ctx context.Context,
	req *identity.ProbeRequest) (*identity.ProbeResponse, error) {
	return nil, nil
}
