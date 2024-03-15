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

package rbd_types

import (
	"context"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// VolumeGroup contains a number of volumes, and can be used to create a
// VolumeGroupSnapshot.
type VolumeGroup interface {
	// Destroy frees the resources used by the VolumeGroup.
	Destroy(ctx context.Context)

	GetID(ctx context.Context) (string, error)

	// SetMonitors connects to the Ceph cluster.
	SetMonitors(ctx context.Context, monitors string) error
	// SetPool uses the connection to the Ceph cluster to create an
	// IOContext to the pool.
	SetPool(ctx context.Context, pool string) error

	SetJournalNamespace(ctx context.Context, pool, namespace string) error

	Create(ctx context.Context, prefix string) error
	Delete(ctx context.Context) error

	AddVolume(ctx context.Context, volume Volume) error
	RemoveVolume(ctx context.Context, volume Volume) error

	CreateSnapshot(ctx context.Context, name string) (VolumeGroupSnapshot, error)
}

// VolumeGroupSnapshot is an instance of a group of snapshots that was taken
// from om a VolumeGroup.
type VolumeGroupSnapshot interface {
	// Destroy frees the resources used by the VolumeGroupSnapshot.
	Destroy(ctx context.Context)

	Delete(ctx context.Context) error

	GetID(ctx context.Context) (string, error)

	ListSnapshots(ctx context.Context) ([]Snapshot, error)

	GetCreationTime(ctx context.Context) (*time.Time, error)
	GetReadyToUse(ctx context.Context) (bool, error)

	ToCSIVolumeGroupSnapshot(ctx context.Context) (*csi.VolumeGroupSnapshot, error)
}
