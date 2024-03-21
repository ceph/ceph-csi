package rbd

import (
	"context"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"

	types "github.com/ceph/ceph-csi/internal/rbd_types"
)

/////////////////////////////////////////////////////////////////////
//                                                                 //
// this is the implementation of the rbd_types.RBDVolume interface //
//                                                                 //
/////////////////////////////////////////////////////////////////////

// verify that the rbdImage type implements the RBDVolume interface
var _ types.Volume = &rbdImage{}

func (ri *rbdImage) GetID(ctx context.Context) (string, error) {
	return ri.VolID, nil
}

// AddToGroup adds the image to the group with the ioctx. This is called from
// the rbd_group package, as that can pass the ioctx of the group.
func (ri *rbdImage) AddToGroup(ctx context.Context, ioctx *rados.IOContext, group string) error {
	return librbd.GroupImageAdd(ioctx, group, ri.ioctx, ri.RbdImageName)
}

// RemoveFromGroup removes the image to the group with the ioctx. This is
// called from the rbd_group package, as that can pass the ioctx of the group.
func (ri *rbdImage) RemoveFromGroup(ctx context.Context, ioctx *rados.IOContext, group string) error {
	return librbd.GroupImageRemove(ioctx, group, ri.ioctx, ri.RbdImageName)
}
