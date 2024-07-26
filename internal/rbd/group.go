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

	librbd "github.com/ceph/go-ceph/rbd"

	"github.com/ceph/ceph-csi/internal/rbd/types"
)

// AddToGroup adds the image to the group. This is called from the rbd_group
// package.
func (rv *rbdVolume) AddToGroup(ctx context.Context, vg types.VolumeGroup) error {
	ioctx, err := vg.GetIOContext(ctx)
	if err != nil {
		return fmt.Errorf("could not get iocontext for volume group %q: %w", vg, err)
	}

	name, err := vg.GetName(ctx)
	if err != nil {
		return fmt.Errorf("could not get name for volume group %q: %w", vg, err)
	}

	// check if the image is already part of a group
	// "rbd: ret=-17, File exists" is returned if the image is part of ANY group
	image, err := rv.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q: %w", rv, err)
	}

	info, err := image.GetGroup()
	if err != nil {
		return fmt.Errorf("could not get group information for image %q: %w", rv, err)
	}

	if info.Name != "" && info.Name != name {
		return fmt.Errorf("image %q is already part of volume group %q", rv, info.Name)
	}

	err = librbd.GroupImageAdd(ioctx, name, rv.ioctx, rv.RbdImageName)
	if err != nil {
		return fmt.Errorf("failed to add image %q to volume group %q: %w", rv, vg, err)
	}

	return nil
}

// RemoveFromGroup removes the image from the group. This is called from the
// rbd_group package.
func (rv *rbdVolume) RemoveFromGroup(ctx context.Context, vg types.VolumeGroup) error {
	ioctx, err := vg.GetIOContext(ctx)
	if err != nil {
		return fmt.Errorf("could not get iocontext for volume group %q: %w", vg, err)
	}

	name, err := vg.GetName(ctx)
	if err != nil {
		return fmt.Errorf("could not get name for volume group %q: %w", vg, err)
	}

	return librbd.GroupImageRemove(ioctx, name, rv.ioctx, rv.RbdImageName)
}

func (rv *rbdVolume) ToMirror() (types.Mirror, error) {
	return rv, nil
}
