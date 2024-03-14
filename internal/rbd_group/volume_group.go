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

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"

	"github.com/ceph/ceph-csi/internal/journal"
	types "github.com/ceph/ceph-csi/internal/rbd_types"
	"github.com/ceph/ceph-csi/internal/util"
)

const (
	groupSuffix = "rbd-group"
	groupPrefix = "rbd-group"
)

var (
	ErrRBDGroupNotConnected = errors.New("RBD group is not connected")
)

type rbdVolumeGroup struct {
	name        string
	clusterID   string
	credentials *util.Credentials
	secrets     map[string]string

	monitors    string
	pool        string
	poolID      int64
	conn        *util.ClusterConnection
	ioctx       *rados.IOContext
	journal     journal.VolumeGroupJournal
	journalPool string

	id      string
	volumes []types.Volume
}

// verify that rbdVolumeGroup implements the VolumeGroup interface
var _ types.VolumeGroup = &rbdVolumeGroup{}

// NewVolumeGroup initializes a new VolumeGroup object that can be used
// to manage an `rbd group`.
func NewVolumeGroup(ctx context.Context, name, clusterID string, secrets map[string]string) types.VolumeGroup {
	creds, _ := util.NewUserCredentials(secrets)

	return &rbdVolumeGroup{
		name:        name,
		clusterID:   clusterID,
		credentials: creds,
		secrets:     secrets,
	}
}

func (rvg *rbdVolumeGroup) validate() error {
	if rvg.ioctx == nil {
		return ErrRBDGroupNotConnected
	}

	if rvg.journal == nil {
		return ErrRBDGroupNotConnected
	}

	return nil
}

// Destroy frees the resources used by the rbdVolumeGroup.
func (rvg *rbdVolumeGroup) Destroy(ctx context.Context) {
	if rvg.ioctx != nil {
		rvg.ioctx.Destroy()
		rvg.ioctx = nil
	}

	if rvg.conn != nil {
		rvg.conn.Destroy()
		rvg.conn = nil
	}

	if rvg.journal != nil {
		rvg.journal.Destroy()
		rvg.journal = nil
	}

	if rvg.credentials != nil {
		rvg.credentials.DeleteCredentials()
		rvg.credentials = nil
	}
}

func (rvg *rbdVolumeGroup) GetID(ctx context.Context) (string, error) {
	// FIXME: this should be the group-snapshot-handle
	if rvg.id != "" {
		return rvg.id, nil
	}

	return rvg.id, nil
}

// SetMonitors connects to the Ceph cluster.
func (rvg *rbdVolumeGroup) SetMonitors(ctx context.Context, monitors string) error {
	conn := &util.ClusterConnection{}
	err := conn.Connect(monitors, rvg.credentials)
	if err != nil {
		return err
	}

	rvg.conn = conn
	rvg.monitors = monitors

	return nil
}

// SetPool uses the connection to the Ceph cluster to create an IOContext to
// the pool.
func (rvg *rbdVolumeGroup) SetPool(ctx context.Context, pool string) error {
	if rvg.conn == nil {
		return ErrRBDGroupNotConnected
	}

	ioctx, err := rvg.conn.GetIoctx(pool)
	if err != nil {
		return err
	}

	rvg.pool = pool
	rvg.ioctx = ioctx

	return nil
}

func (rvg *rbdVolumeGroup) SetJournalNamespace(ctx context.Context, pool, namespace string) error {
	if rvg.conn == nil {
		return ErrRBDGroupNotConnected
	}

	vgj := journal.NewCSIVolumeGroupJournal(groupSuffix)
	vgj.SetNamespace(namespace)
	err := vgj.Connect(rvg.monitors, namespace, rvg.credentials)
	if err != nil {
		return err
	}

	rvg.journal = vgj
	rvg.journalPool = pool

	return nil
}

func (rvg *rbdVolumeGroup) Create(ctx context.Context) error {
	if err := rvg.validate(); err != nil {
		return err
	}

	journalPoolID, poolID, err := util.GetPoolIDs(
		ctx,
		rvg.monitors,
		rvg.journalPool,
		rvg.pool,
		rvg.credentials)
	if err != nil {
		return err
	}

	id, uniqueName, err := rvg.journal.ReserveName(
		ctx,
		rvg.journalPool,
		journalPoolID,
		rvg.name,
		groupPrefix)
	if err != nil {
		return err
	}

	rvg.id = id
	rvg.poolID = poolID

	// TODO: if the group already exists, resolve details and use that
	return librbd.GroupCreate(rvg.ioctx, uniqueName)
}

func (rvg *rbdVolumeGroup) Delete(ctx context.Context) error {
	if err := rvg.validate(); err != nil {
		return err
	}

	return librbd.GroupRemove(rvg.ioctx, rvg.name)
}

func (rvg *rbdVolumeGroup) AddVolume(ctx context.Context, image types.Volume) error {
	if err := rvg.validate(); err != nil {
		return err
	}

	return image.AddToGroup(ctx, rvg.ioctx, rvg.name)
}

func (rvg *rbdVolumeGroup) RemoveVolume(ctx context.Context, image types.Volume) error {
	if err := rvg.validate(); err != nil {
		return err
	}

	return image.RemoveFromGroup(ctx, rvg.ioctx, rvg.name)
}

func (rvg *rbdVolumeGroup) CreateSnapshot(ctx context.Context, snapName string) (types.VolumeGroupSnapshot, error) {
	if err := rvg.validate(); err != nil {
		return nil, err
	}

	err := librbd.GroupSnapCreate(rvg.ioctx, rvg.name, snapName)
	if err != nil {
		return nil, err
	}

	// TODO: if the snapName already exists, use that as return value

	return newVolumeGroupSnapshot(ctx, rvg, snapName), nil
}

func (rvg *rbdVolumeGroup) DeleteSnapshot(ctx context.Context, snapName string) error {
	if err := rvg.validate(); err != nil {
		return err
	}

	err := librbd.GroupSnapRemove(rvg.ioctx, rvg.name, snapName)
	if err != nil {
		return err
	}

	// TODO: it is not an error if the snapName was not found or does not exist

	return nil
}
