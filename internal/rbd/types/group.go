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

package types

import (
	"context"

	"github.com/csi-addons/spec/lib/go/volumegroup"
)

// VolumeGroup contains a number of volumes, and can be used to create a
// VolumeGroupSnapshot.
type VolumeGroup interface {
	// Destroy frees the resources used by the VolumeGroup.
	Destroy(ctx context.Context)

	// GetID returns the CSI-Addons VolumeGroupId of the VolumeGroup.
	GetID(ctx context.Context) (string, error)

	// ToCSI creates a CSI-Addons type for the VolumeGroup.
	ToCSI(ctx context.Context) *volumegroup.VolumeGroup

	// Delete removes the VolumeGroup from the backend storage.
	Delete(ctx context.Context) error

	// AddVolume adds the Volume to the VolumeGroup.
	AddVolume(ctx context.Context, volume Volume) error

	// RemoveVolume removes the Volume from the VolumeGroup.
	RemoveVolume(ctx context.Context, volume Volume) error

	// ListVolumes returns a slice with all Volumes in the VolumeGroup.
	ListVolumes(ctx context.Context) ([]Volume, error)
}
