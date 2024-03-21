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

package rbd_group

import (
	"context"
	"errors"

	librbd "github.com/ceph/go-ceph/rbd"

	types "github.com/ceph/ceph-csi/internal/rbd_types"
	"github.com/ceph/ceph-csi/internal/util"
)

const (
	groupSuffix = "rbd-group"
)

var (
	ErrRBDGroupNotConnected = errors.New("RBD group is not connected")
)

// volumeGroup handles all requests for 'rbd group' operations.
type volumeGroup struct {
	*groupObject

	// required details to perform operations on the group
	poolID int64

	// volumes is a list of rbd-images that are part of the group. The ID
	// of each volume is stored in the journal.
	volumes []types.Volume
}

// verify that volumeGroup implements the VolumeGroup interface
var _ types.VolumeGroup = &volumeGroup{}

// NewVolumeGroup initializes a new VolumeGroup object that can be used
// to manage an `rbd group`.
func NewVolumeGroup(ctx context.Context, name, clusterID string, secrets map[string]string) types.VolumeGroup {
	creds, _ := util.NewUserCredentials(secrets)

	vg := &volumeGroup{
		groupObject: &groupObject{
			name:        name,
			clusterID:   clusterID,
			credentials: creds,
			secrets:     secrets,
		},
	}

	return vg
}

func GetVolumeGroup(ctx context.Context, id string, secrets map[string]string) (types.VolumeGroup, error) {
	var err error

	vg := &volumeGroup{}
	err = vg.resolveByID(ctx, id, secrets)
	if err != nil {
		return nil, err
	}

	return vg, nil
}

func (vg *volumeGroup) validate() error {
	if vg.ioctx == nil {
		return ErrRBDGroupNotConnected
	}

	if vg.journal == nil {
		return ErrRBDGroupNotConnected
	}

	return nil
}

// Destroy frees the resources used by the volumeGroup.
func (vg *volumeGroup) Destroy(ctx context.Context) {
	if vg.ioctx != nil {
		vg.ioctx.Destroy()
		vg.ioctx = nil
	}

	if vg.conn != nil {
		vg.conn.Destroy()
		vg.conn = nil
	}

	vg.groupObject.Destroy(ctx)
}

func (vg *volumeGroup) Create(ctx context.Context, prefix string) error {
	if err := vg.validate(); err != nil {
		return err
	}

	journalPoolID, poolID, err := util.GetPoolIDs(
		ctx,
		vg.monitors,
		vg.journalPool,
		vg.pool,
		vg.credentials)
	if err != nil {
		return err
	}

	id, uniqueName, err := vg.journal.ReserveName(
		ctx,
		vg.journalPool,
		journalPoolID,
		vg.name,
		prefix)
	if err != nil {
		return err
	}

	vg.id = id
	vg.poolID = poolID

	// TODO: if the group already exists, resolve details and use that
	return librbd.GroupCreate(vg.ioctx, uniqueName)
}

func (vg *volumeGroup) Delete(ctx context.Context) error {
	if err := vg.validate(); err != nil {
		return err
	}

	return librbd.GroupRemove(vg.ioctx, vg.name)
}

func (vg *volumeGroup) AddVolume(ctx context.Context, image types.Volume) error {
	if err := vg.validate(); err != nil {
		return err
	}

	err := image.AddToGroup(ctx, vg.ioctx, vg.name)
	if err != nil {
		return err
	}

	vg.volumes = append(vg.volumes, image)

	return nil
}

func (vg *volumeGroup) RemoveVolume(ctx context.Context, image types.Volume) error {
	if err := vg.validate(); err != nil {
		return err
	}

	// volume was already removed from the group
	if len(vg.volumes) == 0 {
		return nil
	}

	err := image.RemoveFromGroup(ctx, vg.ioctx, vg.name)
	if err != nil {
		return err
	}

	// toRemove contain the ID of the volume that is removed from the group
	toRemove, err := image.GetID(ctx)
	if err != nil {
		return err
	}

	// volumes is the updated list, without the volume that was removed
	volumes := make([]types.Volume, 0)
	for _, v := range vg.volumes {
		id, err := v.GetID(ctx)
		if err != nil {
			return err
		}

		if id == toRemove {
			// do not add the volume to the list
			continue
		}

		volumes = append(volumes, v)
	}

	// update the list of volumes
	vg.volumes = volumes

	return nil
}

func (vg *volumeGroup) CreateSnapshot(ctx context.Context, snapName string) (types.VolumeGroupSnapshot, error) {
	if err := vg.validate(); err != nil {
		return nil, err
	}

	err := librbd.GroupSnapCreate(vg.ioctx, vg.name, snapName)
	if err != nil {
		return nil, err
	}

	// TODO: if the snapName already exists, use that as return value

	return newVolumeGroupSnapshot(ctx, vg, snapName), nil
}

func (vg *volumeGroup) deleteSnapshot(ctx context.Context, snapName string) error {
	if err := vg.validate(); err != nil {
		return err
	}

	err := librbd.GroupSnapRemove(vg.ioctx, vg.name, snapName)
	if err != nil {
		return err
	}

	// TODO: it is not an error if the snapName was not found or does not exist

	return nil
}
