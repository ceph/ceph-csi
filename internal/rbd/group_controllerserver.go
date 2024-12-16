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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ceph/ceph-csi/internal/rbd/group"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// CreateVolumeGroupSnapshot receives a list of volume handles and is requested
// to create a list of snapshots that are created at the same time. This is
// similar (although not exactly the same) to consistency groups.
//
// RBD has a limitation that an image can only belong to a single group. It is
// therefore required to create a temporary group, add all images, create the
// group snapshot and remove all images from the group again. This leaves the
// group and its snapshot around, the group snapshot can be inspected to list
// the snapshots of the images.
//
//nolint:gocyclo,cyclop // TODO: reduce complexity.
func (cs *ControllerServer) CreateVolumeGroupSnapshot(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest,
) (*csi.CreateVolumeGroupSnapshotResponse, error) {
	var (
		err           error
		vg            types.VolumeGroup
		groupSnapshot types.VolumeGroupSnapshot

		// the VG and VGS should not have the same name
		vgName  = req.GetName() + "-vg" // stable temporary name
		vgsName = req.GetName()
	)

	// Existence and conflict checks
	if acquired := cs.VolumeGroupLocks.TryAcquire(vgsName); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, vgsName)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, vgsName)
	}
	defer cs.VolumeGroupLocks.Release(vgsName)

	mgr := NewManager(cs.Driver.GetInstanceID(), req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	// resolve all volumes, free them later on
	volumes := make([]types.Volume, len(req.GetSourceVolumeIds()))
	defer func() {
		for _, volume := range volumes {
			if vg != nil {
				// 'normal' cleanup, remove all images from the group
				vgErr := vg.RemoveVolume(ctx, volume)
				if vgErr != nil {
					log.ErrorLog(
						ctx,
						"failed to remove volume %q from volume group %q: %v",
						volume, vg, vgErr)
				}
			}

			// free all allocated volumes
			if volume != nil {
				volume.Destroy(ctx)
			}
		}

		if vg != nil {
			// the VG should always be deleted, volumes can only belong to a single VG
			log.DebugLog(ctx, "removing temporary volume group %q", vg)

			vgErr := vg.Delete(ctx)
			if vgErr != nil {
				log.ErrorLog(ctx, "failed to remove temporary volume group %q: %v", vg, vgErr)
			}

			// free the resources of the VolumeGroup
			vg.Destroy(ctx)
		}
	}()
	for i, id := range req.GetSourceVolumeIds() {
		var vol types.Volume
		vol, err = mgr.GetVolumeByID(ctx, id)
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"failed to find required volume %q for volume group snapshot %q: %s",
				id,
				vgsName,
				err.Error())
		}
		volumes[i] = vol
	}

	log.DebugLog(ctx, "all %d Volumes for VolumeGroup %q have been found", len(volumes), vgsName)

	groupSnapshot, err = mgr.GetVolumeGroupSnapshotByName(ctx, vgsName)
	if groupSnapshot != nil {
		defer groupSnapshot.Destroy(ctx)

		csiVGS, csiErr := groupSnapshot.ToCSI(ctx)
		if csiErr != nil {
			return nil, status.Error(codes.Aborted, csiErr.Error())
		}

		return &csi.CreateVolumeGroupSnapshotResponse{
			GroupSnapshot: csiVGS,
		}, nil
	}
	if err != nil {
		log.DebugLog(ctx, "need to create new volume group snapshot, "+
			"failed to get existing one with name %q: %v", vgsName, err)
	}

	creds, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer creds.DeleteCredentials()

	errList := make([]error, 0)
	for _, volume := range volumes {
		err = volume.PrepareVolumeForSnapshot(ctx, creds)
		if err != nil {
			errList = append(errList, err)
		}
	}
	if len(errList) > 0 {
		// FIXME: we should probably choose a error code that has more priority.
		return nil, status.Errorf(
			status.Code(errList[0]),
			"failed to prepare volumes for snapshot: %v",
			errList)
	}

	// create a temporary VolumeGroup with a different name
	vg, err = mgr.CreateVolumeGroup(ctx, vgName)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to create volume group %q: %s",
			vgName,
			err.Error())
	}
	// vg.Destroy(ctx) is called in a defer further up ^

	log.DebugLog(ctx, "VolumeGroup %q has been created: %+v", vgName, vg)

	// add images to the group
	for _, volume := range volumes {
		err = vg.AddVolume(ctx, volume)
		if err != nil {
			return nil, status.Error(codes.Aborted, err.Error())
		}
	}

	groupSnapshot, err = mgr.CreateVolumeGroupSnapshot(ctx, vg, vgsName)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer groupSnapshot.Destroy(ctx)

	csiVGS, err := groupSnapshot.ToCSI(ctx)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &csi.CreateVolumeGroupSnapshotResponse{
		GroupSnapshot: csiVGS,
	}, nil
}

func (cs *ControllerServer) DeleteVolumeGroupSnapshot(
	ctx context.Context,
	req *csi.DeleteVolumeGroupSnapshotRequest,
) (*csi.DeleteVolumeGroupSnapshotResponse, error) {
	// FIXME: more checking of the request in needed
	// 1. verify that all snapshots in the request are all snapshots in the group
	// 2. delete the group

	groupSnapshotID := req.GetGroupSnapshotId()

	// Existence and conflict checks
	if acquired := cs.VolumeGroupLocks.TryAcquire(groupSnapshotID); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, groupSnapshotID)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, groupSnapshotID)
	}
	defer cs.VolumeGroupLocks.Release(groupSnapshotID)

	mgr := NewManager(cs.Driver.GetInstanceID(), nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	groupSnapshot, err := mgr.GetVolumeGroupSnapshotByID(ctx, groupSnapshotID)
	if err != nil {
		if errors.Is(err, group.ErrRBDGroupNotFound) {
			log.ErrorLog(ctx, "VolumeGroupSnapshot %q doesn't exists", groupSnapshotID)

			return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
		}

		return nil, status.Errorf(
			codes.Internal,
			"could not fetch volume group snapshot with id %q: %s",
			groupSnapshotID,
			err.Error())
	}
	defer groupSnapshot.Destroy(ctx)

	err = groupSnapshot.Delete(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to delete volume group snapshot %q: %v",
			groupSnapshot, err)
	}

	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

// GetVolumeGroupSnapshot is sortof optional, only used for
// static/pre-provisioned VolumeGroupSnapshots.
func (cs *ControllerServer) GetVolumeGroupSnapshot(
	ctx context.Context,
	req *csi.GetVolumeGroupSnapshotRequest,
) (*csi.GetVolumeGroupSnapshotResponse, error) {
	groupSnapshotID := req.GetGroupSnapshotId()

	// Existence and conflict checks
	if acquired := cs.VolumeGroupLocks.TryAcquire(groupSnapshotID); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, groupSnapshotID)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, groupSnapshotID)
	}
	defer cs.VolumeGroupLocks.Release(groupSnapshotID)

	mgr := NewManager(cs.Driver.GetInstanceID(), nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	groupSnapshot, err := mgr.GetVolumeGroupSnapshotByID(ctx, groupSnapshotID)
	if err != nil {
		if errors.Is(err, group.ErrRBDGroupNotFound) {
			log.ErrorLog(ctx, "VolumeGroupSnapshot %q doesn't exists", groupSnapshotID)

			return nil, status.Errorf(
				codes.NotFound,
				"failed to get volume group snapshot with id %q: %v",
				groupSnapshotID, err)
		}

		return nil, status.Errorf(
			codes.Internal,
			"could not fetch volume group snapshot with id %q: %s",
			groupSnapshotID,
			err.Error())
	}
	defer groupSnapshot.Destroy(ctx)

	csiVGS, err := groupSnapshot.ToCSI(ctx)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: csiVGS,
	}, nil
}
