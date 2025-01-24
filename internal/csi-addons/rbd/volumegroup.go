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
	"errors"
	"fmt"
	"slices"

	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/rbd/group"
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

	// driverInstance is the unique ID for this CSI-driver deployment.
	driverInstance string
}

// NewVolumeGroupServer creates a new VolumeGroupServer which handles the
// VolumeGroup Service requests from the CSI-Addons specification.
func NewVolumeGroupServer(instanceID string) *VolumeGroupServer {
	return &VolumeGroupServer{
		driverInstance: instanceID,
	}
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
	mgr := rbd.NewManager(vs.driverInstance, req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	// resolve all volumes
	volumes := make([]types.Volume, len(req.GetVolumeIds()))
	defer func() {
		for _, vol := range volumes {
			vol.Destroy(ctx)
		}
	}()
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
		volumes[i] = vol
	}

	log.DebugLog(ctx, "all %d Volumes for VolumeGroup %q have been found", len(volumes), req.GetName())

	// create a RBDVolumeGroup
	vg, err := mgr.CreateVolumeGroup(ctx, req.GetName())
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to create volume group %q: %s",
			req.GetName(),
			err.Error())
	}

	log.DebugLog(ctx, "VolumeGroup %q has been created: %+v", req.GetName(), vg)

	// extract the flatten mode
	flattenMode, err := getFlattenMode(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}
	// Flatten the image if the flatten mode is set to FlattenModeForce
	// before adding it to the volume group.
	for _, v := range volumes {
		err = v.HandleParentImageExistence(ctx, flattenMode)
		if err != nil {
			err = fmt.Errorf("failed to handle parent image for volume group %q: %w", vg, err)

			return nil, getGRPCError(err)
		}
	}
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

	log.DebugLog(ctx, "all %d Volumes have been added to for VolumeGroup %q", len(volumes), req.GetName())

	csiVG, err := vg.ToCSI(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to convert volume group %q to CSI type: %s",
			req.GetName(),
			err.Error())
	}

	return &volumegroup.CreateVolumeGroupResponse{
		VolumeGroup: csiVG,
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
	mgr := rbd.NewManager(vs.driverInstance, nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	// resolve the volume group
	vg, err := mgr.GetVolumeGroupByID(ctx, req.GetVolumeGroupId())
	if err != nil {
		if errors.Is(err, group.ErrRBDGroupNotFound) {
			log.ErrorLog(ctx, "VolumeGroup %q doesn't exists", req.GetVolumeGroupId())

			return &volumegroup.DeleteVolumeGroupResponse{}, nil
		}

		return nil, status.Errorf(
			codes.Internal,
			"could not fetch volume group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}
	defer vg.Destroy(ctx)

	log.DebugLog(ctx, "VolumeGroup %q has been found", req.GetVolumeGroupId())

	// verify that the volume group is empty
	volumes, err := vg.ListVolumes(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			"could not list volumes for voluem group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}

	log.DebugLog(ctx, "VolumeGroup %q contains %d volumes", req.GetVolumeGroupId(), len(volumes))

	if len(volumes) != 0 {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"rejecting to delete non-empty volume group %q",
			req.GetVolumeGroupId())
	}

	// delete the volume group
	err = vg.Delete(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to delete volume group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}

	log.DebugLog(ctx, "VolumeGroup %q has been deleted", req.GetVolumeGroupId())

	return &volumegroup.DeleteVolumeGroupResponse{}, nil
}

// ModifyVolumeGroupMembership RPC call to modify a volume group.
//
// From the spec:
// This RPC will be called by the CO to modify an existing volume group on
// behalf of a user. volume_ids provided in the
// ModifyVolumeGroupMembershipRequest will be compared to the ones in the
// existing volume group. New volume_ids in the modified volume group will be
// added to the volume group. Existing volume_ids not in the modified volume
// group will be removed from the volume group. If volume_ids is empty, the
// volume group will be removed of all existing volumes. This operation MUST be
// idempotent.
//
// File-based storage systems usually do not support this PRC. Block-based
// storage systems usually support this PRC.
//
// By adding an existing volume to a group, however, there is no way to pass in
// parameters to influence placement when provisioning a volume.
//
// It is out of the scope of the CSI spec to determine whether a group is
// consistent or not. It is up to the storage provider to clarify that in the
// vendor specific documentation. This is true either when creating a new
// volume with a group id or adding an existing volume to a group.
//
// CSI drivers supporting MODIFY_VOLUME_GROUP_MEMBERSHIP MUST implement
// ModifyVolumeGroupMembership RPC.
//
// Note:
//
// The implementation works as the following:
// - resolve the existing volume group
// - get the CSI-IDs of all volumes
// - create a list of volumes that should be removed
// - create a list of volume IDs that should be added
// - remove the volumes from the group
// - add the volumes to the group
//
// Also, MODIFY_VOLUME_GROUP_MEMBERSHIP does not exist, it is called
// MODIFY_VOLUME_GROUP instead.
func (vs *VolumeGroupServer) ModifyVolumeGroupMembership(
	ctx context.Context,
	req *volumegroup.ModifyVolumeGroupMembershipRequest,
) (*volumegroup.ModifyVolumeGroupMembershipResponse, error) {
	mgr := rbd.NewManager(vs.driverInstance, nil, req.GetSecrets())
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

	beforeVolumes, err := vg.ListVolumes(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to list volumes of volume group %q: %v",
			vg,
			err)
	}

	// beforeIDs contains the csiID as key, volume as value
	beforeIDs := make(map[string]types.Volume, len(beforeVolumes))
	for _, vol := range beforeVolumes {
		id, idErr := vol.GetID(ctx)
		if idErr != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"failed to get the CSI ID of volume %q: %v",
				vol,
				err)
		}

		beforeIDs[id] = vol
	}

	// check which volumes should not be part of the group
	afterIDs := req.GetVolumeIds()
	toRemove := make([]string, 0)
	for id := range beforeIDs {
		if !slices.Contains(afterIDs, id) {
			toRemove = append(toRemove, id)
		}
	}

	// check which volumes are new to the group
	toAdd := make([]string, 0)
	for _, id := range afterIDs {
		if _, ok := beforeIDs[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}

	// remove the volume that should not be part of the group
	for _, id := range toRemove {
		vol := beforeIDs[id]
		err = vg.RemoveVolume(ctx, vol)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to remove volume %q from volume group %q: %v",
				vol,
				vg,
				err)
		}
	}

	// resolve all volumes
	volumes := make([]types.Volume, len(toAdd))
	defer func() {
		for _, vol := range volumes {
			vol.Destroy(ctx)
		}
	}()
	for i, id := range toAdd {
		var vol types.Volume
		vol, err = mgr.GetVolumeByID(ctx, id)
		if err != nil {
			return nil, status.Errorf(
				codes.NotFound,
				"failed to find a volume with CSI ID %q: %v",
				id,
				err)
		}
		volumes[i] = vol
	}

	// extract the flatten mode
	flattenMode, err := getFlattenMode(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}
	// Flatten the image if the flatten mode is set to FlattenModeForce
	// before adding it to the volume group.
	for _, vol := range volumes {
		err = vol.HandleParentImageExistence(ctx, flattenMode)
		if err != nil {
			err = fmt.Errorf("failed to handle parent image for volume group %q: %w", vg, err)

			return nil, getGRPCError(err)
		}
	}

	// add the new volumes to the group
	for _, vol := range volumes {
		err = vg.AddVolume(ctx, vol)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to add volume %q to volume group %q: %v",
				vol,
				vg,
				err)
		}
	}

	csiVG, err := vg.ToCSI(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to convert volume group %q to CSI format: %v",
			vg,
			err)
	}

	return &volumegroup.ModifyVolumeGroupMembershipResponse{
		VolumeGroup: csiVG,
	}, nil
}

// ControllerGetVolumeGroup RPC call to get a volume group.
//
// From the spec:
// ControllerGetVolumeGroupResponse should contain current information of a
// volume group if it exists. If the volume group does not exist any more,
// ControllerGetVolumeGroup should return gRPC error code NOT_FOUND.
func (vs *VolumeGroupServer) ControllerGetVolumeGroup(
	ctx context.Context,
	req *volumegroup.ControllerGetVolumeGroupRequest,
) (*volumegroup.ControllerGetVolumeGroupResponse, error) {
	mgr := rbd.NewManager(vs.driverInstance, nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	// resolve the volume group
	vg, err := mgr.GetVolumeGroupByID(ctx, req.GetVolumeGroupId())
	if err != nil {
		if errors.Is(err, group.ErrRBDGroupNotFound) {
			log.ErrorLog(ctx, "VolumeGroup %q doesn't exists", req.GetVolumeGroupId())

			return nil, status.Errorf(
				codes.NotFound,
				"could not find volume group %q: %s",
				req.GetVolumeGroupId(),
				err.Error())
		}

		return nil, status.Errorf(
			codes.Internal,
			"could not fetch volume group %q: %s",
			req.GetVolumeGroupId(),
			err.Error())
	}
	defer vg.Destroy(ctx)

	csiVG, err := vg.ToCSI(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to convert volume group %q to CSI format: %v",
			vg,
			err)
	}

	return &volumegroup.ControllerGetVolumeGroupResponse{
		VolumeGroup: csiVG,
	}, nil
}
