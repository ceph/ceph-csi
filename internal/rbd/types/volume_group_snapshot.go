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
)

// VolumeGroupSnapshot provide functions to inspect and modify a
// VolumeGroupSnapshot. The rbd.Manager can be used to create or otherwise
// obtain a VolumeGroupSnapshot struct.
type VolumeGroupSnapshot interface {
	journalledObject

	// Destroy frees the resources used by the VolumeGroupSnapshot.
	Destroy(ctx context.Context)

	// Delete removes the VolumeGroupSnapshot from the storage backend.
	Delete(ctx context.Context) error

	// ToCSI returns the VolumeGroupSnapshot struct in CSI format.
	ToCSI(ctx context.Context) (*csi.VolumeGroupSnapshot, error)

	// GetCreationTime retrurns the time when the VolumeGroupSnapshot was
	// created.
	GetCreationTime(ctx context.Context) (*time.Time, error)

	// ListSnapshots returns a slice with all Snapshots in the
	// VolumeGroupSnapshot.
	ListSnapshots(ctx context.Context) ([]Snapshot, error)
}
