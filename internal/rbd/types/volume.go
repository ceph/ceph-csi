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
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/util"
)

type snapshottableVolume interface {
	// NewSnapshotByID creates a new Snapshot object based on the details of the Volume.
	NewSnapshotByID(ctx context.Context, cr *util.Credentials, name string, id uint64) (Snapshot, error)

	// PrepareVolumeForSnapshot prepares the volume for snapshot by
	// checking snapshots limit and clone depth limit and flatten it
	// if required.
	PrepareVolumeForSnapshot(ctx context.Context, cr *util.Credentials) error
}

type csiAddonsVolume interface {
	// AddToGroup adds the Volume to the VolumeGroup.
	AddToGroup(ctx context.Context, vg VolumeGroup) error

	// RemoveFromGroup removes the Volume from the VolumeGroup.
	RemoveFromGroup(ctx context.Context, vg VolumeGroup) error

	// RotateEncryptionKey processes the key rotation for the RBD Volume.
	RotateEncryptionKey(ctx context.Context) error

	// Sparsify tries to free unused blocks of the volume from the CSI-Addons Controller.
	Sparsify(ctx context.Context) error

	// HandleParentImageExistence checks the image's parent.
	// if the parent image does not exist and is not in trash, it returns nil.
	// if the flattenMode is FlattenModeForce, it flattens the image itself.
	// if the parent image is in trash, it returns an error.
	// if the parent image exists and is not enabled for mirroring, it returns an error.
	HandleParentImageExistence(ctx context.Context, flattenMode FlattenMode) error

	// RepairResyncedImageID updates the existing image ID with new one in OMAP.
	RepairResyncedImageID(ctx context.Context, ready bool) error

	// ToMirror converts the Volume to a Mirror.
	ToMirror() (Mirror, error)
}

type Volume interface {
	journalledObject
	snapshottableVolume
	csiAddonsVolume

	// Destroy frees the resources used by the Volume.
	Destroy(ctx context.Context)

	// Delete removes the volume from the storage backend.
	Delete(ctx context.Context) error

	// ToCSI creates a CSI protocol formatted struct of the volume.
	ToCSI(ctx context.Context) (*csi.Volume, error)

	// GetCreationTime returns the creation time of the volume.
	GetCreationTime(ctx context.Context) (*time.Time, error)

	// GetMetadata returns the value of the metadata key from the volume.
	GetMetadata(key string) (string, error)
	// SetMetadata sets the value of the metadata key on the volume.
	SetMetadata(key, value string) error
}
