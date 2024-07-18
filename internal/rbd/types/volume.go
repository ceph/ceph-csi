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

	"github.com/container-storage-interface/spec/lib/go/csi"
)

type Volume interface {
	// Destroy frees the resources used by the Volume.
	Destroy(ctx context.Context)

	// Delete removes the volume from the storage backend.
	Delete(ctx context.Context) error

	// GetID returns the CSI VolumeID for the volume.
	GetID(ctx context.Context) (string, error)

	// ToCSI creates a CSI protocol formatted struct of the volume.
	ToCSI(ctx context.Context) (*csi.Volume, error)

	// AddToGroup adds the Volume to the VolumeGroup.
	AddToGroup(ctx context.Context, vg VolumeGroup) error

	// RemoveFromGroup removes the Volume from the VolumeGroup.
	RemoveFromGroup(ctx context.Context, vg VolumeGroup) error
}
