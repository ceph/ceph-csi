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

	"github.com/ceph/go-ceph/rados"
	"github.com/csi-addons/spec/lib/go/volumegroup"

	"github.com/ceph/ceph-csi/internal/util"
)

type journalledObject interface {
	// GetID returns the ID in the backend storage for the object.
	GetID(ctx context.Context) (string, error)

	// GetName returns the name of the object in the backend storage.
	GetName(ctx context.Context) (string, error)

	// GetPool returns the name of the pool that holds the object.
	GetPool(ctx context.Context) (string, error)

	// GetClusterID returns the ID of the cluster of the object.
	GetClusterID(ctx context.Context) (string, error)
}

// VolumeGroup contains a number of volumes.
type VolumeGroup interface {
	journalledObject

	// Destroy frees the resources used by the VolumeGroup.
	Destroy(ctx context.Context)

	// GetIOContext returns the IOContext for performing librbd operations
	// on the VolumeGroup. This is used by the rbdVolume struct when it
	// needs to add/remove itself from the VolumeGroup.
	GetIOContext(ctx context.Context) (*rados.IOContext, error)

	// ToCSI creates a CSI-Addons type for the VolumeGroup.
	ToCSI(ctx context.Context) (*volumegroup.VolumeGroup, error)

	// Create makes a new group in the backend storage.
	Create(ctx context.Context) error

	// Delete removes the VolumeGroup from the backend storage.
	Delete(ctx context.Context) error

	// AddVolume adds the Volume to the VolumeGroup.
	AddVolume(ctx context.Context, volume Volume) error

	// RemoveVolume removes the Volume from the VolumeGroup.
	RemoveVolume(ctx context.Context, volume Volume) error

	// ListVolumes returns a slice with all Volumes in the VolumeGroup.
	ListVolumes(ctx context.Context) ([]Volume, error)

	// CreateSnapshots creates Snapshots of all Volume in the VolumeGroup.
	// The Snapshots are crash consistent, and created as a consistency
	// group.
	CreateSnapshots(ctx context.Context, cr *util.Credentials, name string) ([]Snapshot, error)
}
