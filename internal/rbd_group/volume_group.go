package rbd_group

import (
	"context"
	"errors"

	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
)

const (
	ErrRBDGroupNotConnected = errors.New("RBD group is not connected")
)

type RBDVolumeGroup interface {
	Destroy()

	SetPool(pool string)

	Create(ctx context.Context) error
	Delete(ctx context.Context) error

	AddVolume(ctx context.Context, volumeID string) error
	RemoveVolume(ctx context.Context, volumeID string) error

	CreateSnapshot(ctx context.Context, name string) error
}

type rbdVolumeGroup struct {
	name  string
	conn *util.ClusterConnection
}

func NewRBDVolumeGroup(ctx context.Context, name string, conn *util.ClusterConnection) RBDVolumeGroup {
	return &rbdVolumeGroup{
		name: name,
		conn: conn,
	}, nil
}

func (rvg *rbdVolumeGroup) Destroy() {
	if rvg.ioctx != nil {
		rvg.ioctx.Destroy()
		rvg.ioctx = nil
	}
}

func (rvg *rbdVolumeGroup) SetPool(pool string) {
	ioctx, err := rvg.conn.GetIoctx(pool)
	if err != nil {
		return err
	}

	rvg.pool = pool
	rvg.ioctx = ioctx
}

func (rvg *rbdVolumeGroup) Create(ctx context.Context) error {
	if rvg.ioctx == nil {
		return ErrRBDGroupNotConnected
	}

	// TODO: if the group already exists, resolve details and use that
	return rbd.GroupCreate(rvg.ioctx, rvg.name)
}

func (rvg *rbdVolumeGroup) Delete(ctx context.Context) error {
	return rbd.GroupRemove(rvg.ioctx, rvg.name)
}

func (rvg *rbdVolumeGroup) AddVolume(ctx context.Context, volumeID string) error {
	return rbd.GroupImageAdd(rvg.ioctx, group, rvg.ioctx, volumeID)
}

func (rvg *rbdVolumeGroup) RemoveVolume(ctx context.Context, volumeID string) error {
	return rbd.GroupImageRemove(rvg.ioctx, group, rvg.ioctx, volumeID)
}

func (rvg *rbdVolumeGroup) CreateSnapshot(ctx context.Context, snapName string) error {
	return rbd.GroupSnapCreate(rvg.ioctx, rvg.group, snapName)
}
