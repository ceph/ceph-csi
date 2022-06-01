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
	"fmt"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	rbdutil "github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	rs "github.com/csi-addons/spec/lib/go/reclaimspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReclaimSpaceControllerServer struct of rbd CSI driver with supported methods
// of CSI-addons reclaimspace controller service spec.
type ReclaimSpaceControllerServer struct {
	*rs.UnimplementedReclaimSpaceControllerServer
}

// NewReclaimSpaceControllerServer creates a new IdentityServer which handles
// the Identity Service requests from the CSI-Addons specification.
func NewReclaimSpaceControllerServer() *ReclaimSpaceControllerServer {
	return &ReclaimSpaceControllerServer{}
}

func (rscs *ReclaimSpaceControllerServer) RegisterService(server grpc.ServiceRegistrar) {
	rs.RegisterReclaimSpaceControllerServer(server, rscs)
}

func (rscs *ReclaimSpaceControllerServer) ControllerReclaimSpace(
	ctx context.Context,
	req *rs.ControllerReclaimSpaceRequest,
) (*rs.ControllerReclaimSpaceResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	rbdVol, err := rbdutil.GenVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to find volume with ID %q: %s", volumeID, err.Error())
	}
	defer rbdVol.Destroy()

	err = rbdVol.Sparsify()
	if err != nil {
		// TODO: check for different error codes?
		return nil, status.Errorf(codes.Internal, "failed to sparsify volume %q: %s", rbdVol, err.Error())
	}

	return &rs.ControllerReclaimSpaceResponse{}, nil
}

// ReclaimSpaceNodeServer struct of rbd CSI driver with supported methods
// of CSI-addons reclaimspace controller service spec.
type ReclaimSpaceNodeServer struct {
	*rs.UnimplementedReclaimSpaceNodeServer
}

// NewReclaimSpaceNodeServer creates a new IdentityServer which handles the
// Identity Service requests from the CSI-Addons specification.
func NewReclaimSpaceNodeServer() *ReclaimSpaceNodeServer {
	return &ReclaimSpaceNodeServer{}
}

func (rsns *ReclaimSpaceNodeServer) RegisterService(server grpc.ServiceRegistrar) {
	rs.RegisterReclaimSpaceNodeServer(server, rsns)
}

// NodeReclaimSpace runs fstrim or blkdiscard on the path where the volume is
// mounted or attached. When a volume with multi-node permissions is detected,
// an error is returned to prevent potential data corruption.
func (rsns *ReclaimSpaceNodeServer) NodeReclaimSpace(
	ctx context.Context,
	req *rs.NodeReclaimSpaceRequest,
) (*rs.NodeReclaimSpaceResponse, error) {
	// volumeID is a required attribute, it is part of the path to run the
	// space reducing command on
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	// path can either be the staging path on the node, or the volume path
	// inside an application container
	path := req.GetStagingTargetPath()
	if path == "" {
		path = req.GetVolumePath()
		if path == "" {
			return nil, status.Error(
				codes.InvalidArgument,
				"required parameter staging_target_path or volume_path is not set")
		}
	} else {
		// append the right staging location used by this CSI-driver
		path = fmt.Sprintf("%s/%s", path, volumeID)
	}

	// do not allow RWX block-mode volumes, danger of data corruption
	isBlock, isMultiNode := csicommon.IsBlockMultiNode([]*csi.VolumeCapability{req.GetVolumeCapability()})
	if isMultiNode {
		return nil, status.Error(codes.Unimplemented, "multi-node space reclaim is not supported")
	}

	if isBlock {
		return nil, status.Error(codes.Unimplemented, "block-mode space reclaim is not supported")
	}

	cmd := "fstrim"
	_, stderr, err := util.ExecCommand(ctx, cmd, path)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to execute %q on %q (%s): %s",
			cmd,
			path,
			err.Error(),
			stderr)
	}

	return &rs.NodeReclaimSpaceResponse{}, nil
}
