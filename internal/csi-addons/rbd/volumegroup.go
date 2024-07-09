/*
Copyright 2024 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/csi-addons/spec/lib/go/volumegroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VolumeGroupServer struct of rbd CSI driver with supported methods of
// VolumeGroup controller server spec.
type VolumeGroupServer struct {
	// added UnimplementedControllerServer as a member of ControllerServer.
	// if volumegroup spec add more RPC services in the proto file, then we
	// don't need to add all RPC methods leading to forward compatibility.
	*volumegroup.UnimplementedControllerServer
}

// NewVolumeGroupServer creates a new VolumeGroupServer which handles the
// VolumeGroup Service requests from the CSI-Addons specification.
func NewVolumeGroupServer() *VolumeGroupServer {
	return &VolumeGroupServer{}
}

func (vs *VolumeGroupServer) RegisterService(server grpc.ServiceRegistrar) {
	volumegroup.RegisterControllerServer(server, vs)
}

// CreateVolumeGroup RPC call to create a volume group.
//
// From the spec:
// This RPC will be called by the CO to create a new volume group on behalf of
// a user. This operation MUST be idempotent. If a volume group corresponding
// to the specified volume group name already exists, is compatible with the
// specified parameters in the CreateVolumeGroupRequest, the Plugin MUST reply
// 0 OK with the corresponding CreateVolumeGroupResponse. CSI Plugins MAY
// create the following types of volume groups:
//
// Create a new empty volume group or a group with specific volumes. Note that
// N volumes with some backend label Y could be considered to be in "group Y"
// which might not be a physical group on the storage backend. In this case, an
// empty group can still be created by the CO to hold volumes. After the empty
// group is created, create a new volume. CO may call
// ModifyVolumeGroupMembership to add new volumes to the group.
//
// Implementation steps:
// 1. resolve all volumes given in the volume_ids list (can be empty)
// 2. create the Volume Group
// 3. add all volumes to the Volume Group
//
// Idempotency should be handled by the rbd.Manager, keeping this function and
// the potential error handling as simple as possible.
func (vs *VolumeGroupServer) CreateVolumeGroup(
	ctx context.Context,
	req *volumegroup.CreateVolumeGroupRequest,
) (*volumegroup.CreateVolumeGroupResponse, error) {
	mgr := rbd.NewManager(req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	// resolve all volumes
	volumes := make([]types.Volume, len(req.GetVolumeIds()))
	for i, id := range req.GetVolumeIds() {
		vol, err := mgr.GetVolumeByID(ctx, id)
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"failed to find required volume %q for volume group %q: %s",
				id,
				req.GetName(),
				err.Error())
		}

		//nolint:gocritic // need to call .Destroy() for all volumes
		defer vol.Destroy(ctx)
		volumes[i] = vol
	}

	log.DebugLog(ctx, fmt.Sprintf("all %d Volumes for VolumeGroup %q have been found", len(volumes), req.GetName()))

	// create a RBDVolumeGroup
	vg, err := mgr.CreateVolumeGroup(ctx, req.GetName())
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to create volume group %q: %s",
			req.GetName(),
			err.Error())
	}

	log.DebugLog(ctx, fmt.Sprintf("VolumeGroup %q had been created", req.GetName()))

	// add each rbd-image to the RBDVolumeGroup
	for _, vol := range volumes {
		err = vg.AddVolume(ctx, vol)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to add volume %q to volume group %q: %s",
				vol,
				req.GetName(),
				err.Error())
		}
	}

	log.DebugLog(ctx, fmt.Sprintf("all %d Volumes have been added to for VolumeGroup %q", len(volumes), req.GetName()))

	return &volumegroup.CreateVolumeGroupResponse{
		VolumeGroup: vg.ToCSI(ctx),
	}, nil
}

// DeleteVolumeGroup RPC call to delete a volume group.
//
// From the spec:
// This RPC will be called by the CO to delete a volume group on behalf of a
// user. This operation MUST be idempotent.
//
// If a volume group corresponding to the specified volume_group_id does not
// exist or the artifacts associated with the volume group do not exist
// anymore, the Plugin MUST reply 0 OK.
//
// A volume cannot be deleted individually when it is part of the group. It has
// to be removed from the group first. Delete a volume group will delete all
// volumes in the group.
//
// Note:
// The undocumented DO_NOT_ALLOW_VG_TO_DELETE_VOLUMES capability is set. There
// is no need to delete each volume that may be part of the volume group. If
// the volume group is not empty, a FAILED_PRECONDITION error will be returned.
func (vs *VolumeGroupServer) DeleteVolumeGroup(
	ctx context.Context,
	req *volumegroup.DeleteVolumeGroupRequest,
) (*volumegroup.DeleteVolumeGroupResponse, error) {
	mgr := rbd.NewManager(nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	// resolve the volume group
	vg, err := mgr.GetVolumeGroupByID(ctx, req.GetVolumeGroupId())
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			"could not find volume group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}
	defer vg.Destroy(ctx)

	log.DebugLog(ctx, fmt.Sprintf("VolumeGroup %q has been found", req.GetVolumeGroupId()))

	// verify that the volume group is empty
	volumes, err := vg.ListVolumes(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			"could not list volumes for voluem group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}

	log.DebugLog(ctx, fmt.Sprintf("VolumeGroup %q contains %d volumes", req.GetVolumeGroupId(), len(volumes)))

	if len(volumes) != 0 {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"rejecting to delete non-empty volume group %q",
			req.GetVolumeGroupId())
	}

	// delete the volume group
	err = mgr.DeleteVolumeGroup(ctx, vg)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to delete volume group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}

	log.DebugLog(ctx, fmt.Sprintf("VolumeGroup %q has been deleted", req.GetVolumeGroupId()))

	return &volumegroup.DeleteVolumeGroupResponse{}, nil
}
