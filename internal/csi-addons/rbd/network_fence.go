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

package rbd

import (
	"context"
	"errors"

	nf "github.com/ceph/ceph-csi/internal/csi-addons/networkfence"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/csi-addons/spec/lib/go/fence"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FenceControllerServer struct of rbd CSI driver with supported methods
// of CSI-addons networkfence controller service spec.
type FenceControllerServer struct {
	*fence.UnimplementedFenceControllerServer
}

// NewFenceControllerServer creates a new IdentityServer which handles
// the Identity Service requests from the CSI-Addons specification.
func NewFenceControllerServer() *FenceControllerServer {
	return &FenceControllerServer{}
}

func (fcs *FenceControllerServer) RegisterService(server grpc.ServiceRegistrar) {
	fence.RegisterFenceControllerServer(server, fcs)
}

// validateFenceClusterNetworkReq checks the sanity of FenceClusterNetworkRequest.
func validateNetworkFenceReq(fenceClients []*fence.CIDR, options map[string]string) error {
	if len(fenceClients) == 0 {
		return errors.New("CIDR block cannot be empty")
	}

	if value, ok := options["clusterID"]; !ok || value == "" {
		return errors.New("missing or empty clusterID")
	}

	return nil
}

// FenceClusterNetwork blocks access to a CIDR block by creating a network fence.
// It adds the range of IPs to the osd blocklist, which helps ceph in denying access
// to the malicious clients to prevent data corruption.
func (fcs *FenceControllerServer) FenceClusterNetwork(
	ctx context.Context,
	req *fence.FenceClusterNetworkRequest,
) (*fence.FenceClusterNetworkResponse, error) {
	err := validateNetworkFenceReq(req.GetCidrs(), req.Parameters)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	nwFence, err := nf.NewNetworkFence(ctx, cr, req.Cidrs, req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = nwFence.AddNetworkFence(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fence CIDR block %q: %s", nwFence.Cidr, err.Error())
	}

	return &fence.FenceClusterNetworkResponse{}, nil
}

// UnfenceClusterNetwork unblocks the access to a CIDR block by removing the network fence.
func (fcs *FenceControllerServer) UnfenceClusterNetwork(
	ctx context.Context,
	req *fence.UnfenceClusterNetworkRequest,
) (*fence.UnfenceClusterNetworkResponse, error) {
	err := validateNetworkFenceReq(req.GetCidrs(), req.Parameters)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	nwFence, err := nf.NewNetworkFence(ctx, cr, req.Cidrs, req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = nwFence.RemoveNetworkFence(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unfence CIDR block %q: %s", nwFence.Cidr, err.Error())
	}

	return &fence.UnfenceClusterNetworkResponse{}, nil
}
