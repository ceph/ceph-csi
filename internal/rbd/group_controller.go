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

	"github.com/ceph/ceph-csi/internal/rbd_group"
	types "github.com/ceph/ceph-csi/internal/rbd_types"
	"github.com/ceph/ceph-csi/internal/util"
)

// cephConfig contains the configuration parameters for the Ceph cluster.
type cephConfig struct {
	clusterID       string
	mons            string
	pool            string
	journalPool     string
	namespace       string
	groupNamePrefix string
}

func getCephConfig(ctx context.Context, params, secrets map[string]string) (*cephConfig, error) {
	clusterID, err := util.GetClusterID(params)
	if err != nil {
		return nil, err
	}

	mons, _, err := util.GetMonsAndClusterID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}

	pool := params["pool"]
	if pool == "" {
		return nil, errors.New("missing required parameter: pool")
	}

	journalPool := params["journalPool"]
	if journalPool == "" {
		journalPool = pool
	}

	namespace := params["radosNamespace"]
	if namespace == "" {
		return nil, errors.New("missing required parameter: radosNamespace")
	}

	namePrefix := params["groupNamePrefix"]
	if namePrefix == "" {
		return nil, errors.New("missing required parameter: groupNamePrefix")
	}

	return &cephConfig{
		clusterID:       clusterID,
		mons:            mons,
		pool:            pool,
		journalPool:     journalPool,
		namespace:       namespace,
		groupNamePrefix: namePrefix,
	}, nil
}

func (cs *ControllerServer) GroupControllerGetCapabilities(context.Context, *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	return &csi.GroupControllerGetCapabilitiesResponse{
		Capabilities: []*csi.GroupControllerServiceCapability{{
			Type: &csi.GroupControllerServiceCapability_Rpc{
				Rpc: &csi.GroupControllerServiceCapability_RPC{
					Type: csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
				},
			},
		}},
	}, nil
}

func getVolumesForGroup(ctx context.Context, volumeIDs []string, secrets map[string]string) ([]types.Volume, error) {
	creds, err := util.NewUserCredentials(secrets)
	if err != nil {
		return nil, err
	}
	defer creds.DeleteCredentials()

	volumes := make([]types.Volume, len(volumeIDs))
	for i, id := range volumeIDs {
		volume, err := GenVolFromVolID(ctx, id, creds, secrets)
		if err != nil {
 			return nil, err
		}

		volumes[i] = volume
	}

	return volumes, nil
}

func initVolumeGroup(ctx context.Context, config *cephConfig, name string, secrets map[string]string) (types.VolumeGroup, error) {
	group := rbd_group.NewVolumeGroup(ctx, name, config.clusterID, secrets)

	err := group.SetMonitors(ctx, config.mons)
	if err != nil {
		return nil, err
	}

	err = group.SetPool(ctx, config.pool)
	if err != nil {
		return nil, err
	}

	err = group.SetJournalNamespace(ctx, config.journalPool, config.namespace)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func (cs *ControllerServer) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (*csi.CreateVolumeGroupSnapshotResponse, error) {

	// 1. resolve each rbd-image from the volume-id
	// 2. create a RBDVolumeGroup
	// 3. add each rbd-image to the RBDVolumeGroup
	// 4. create a GroupSnapshot
	// 5. remove all rbd-images from the RBDVolumeGroup
	// 6. return the RBDVolumeGroup-name and list of snapshots

	volumes, err := getVolumesForGroup(ctx, req.GetSourceVolumeIds(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	for _, v := range volumes {
		defer v.Destroy()
	}

	config, err := getCephConfig(ctx, req.GetParameters(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	group, err := initVolumeGroup(ctx, config, req.GetName(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer group.Destroy(ctx)

	// TODO: take a lock on the request

	err = group.Create(ctx, config.groupNamePrefix)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}

	// add images to the group
	for _, v := range volumes {
		err = group.AddVolume(ctx, v)
		if err != nil {
			return nil, status.Error(codes.Aborted, err.Error())
		}
	}

	groupSnapshot, err := group.CreateSnapshot(ctx, req.GetName())
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}
	defer groupSnapshot.Destroy(ctx)

	// remove images from the group
	for _, v := range volumes {
		err = group.RemoveVolume(ctx, v)
		if err != nil {
			return nil, status.Error(codes.Aborted, err.Error())
		}
	}

	csiGroupSnapshot, err := groupSnapshot.ToCSIVolumeGroupSnapshot(ctx)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &csi.CreateVolumeGroupSnapshotResponse{
		GroupSnapshot: csiGroupSnapshot,
	}, nil
}

func (cs *ControllerServer) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (*csi.DeleteVolumeGroupSnapshotResponse, error) {

	// 1. verify that all snapshots in the request are all snapshots in the group
	// 2. delete the group

	snapshot, err := rbd_group.GetVolumeGroupSnapshot(ctx, req.GetGroupSnapshotId(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer snapshot.Destroy(ctx)

	err = snapshot.Delete(ctx)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

func (cs *ControllerServer) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (*csi.GetVolumeGroupSnapshotResponse, error) {
	snapshot, err := rbd_group.GetVolumeGroupSnapshot(ctx, req.GetGroupSnapshotId(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer snapshot.Destroy(ctx)

	csiGroupSnapshot, err := snapshot.ToCSIVolumeGroupSnapshot(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: csiGroupSnapshot,
	}, nil
}
