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
)

// VolumeResolver can be used to construct a Volume from a CSI VolumeId.
type VolumeResolver interface {
	// GetVolumeByID uses the CSI VolumeId to resolve the returned Volume.
	GetVolumeByID(ctx context.Context, id string) (Volume, error)
}

// SnapshotResolver can be used to construct a Snapshot from a CSI SnapshotId.
type SnapshotResolver interface {
	// GetSnapshotByID uses the CSI SnapshotId to resolve the returned Snapshot.
	GetSnapshotByID(ctx context.Context, id string) (Snapshot, error)
}

// Manager provides a way for other packages to get Volumes and VolumeGroups.
// It handles the operations on the backend, and makes sure the journal
// reflects the expected state.
type Manager interface {
	// VolumeResolver is fully implemented by the Manager.
	VolumeResolver

	// SnapshotResolver is fully implemented by the Manager.
	SnapshotResolver

	// Destroy frees all resources that the Manager allocated.
	Destroy(ctx context.Context)

	// GetVolumeGroupByID uses the CSI-Addons VolumeGroupId to resolve the
	// returned VolumeGroup.
	GetVolumeGroupByID(ctx context.Context, id string) (VolumeGroup, error)

	// CreateVolumeGroup allocates a new VolumeGroup in the backend storage
	// and records details about it in the journal.
	CreateVolumeGroup(ctx context.Context, name string) (VolumeGroup, error)

	// GetVolumeGroupSnapshotByID resolves the VolumeGroupSnapshot from the
	// CSI id/handle.
	GetVolumeGroupSnapshotByID(ctx context.Context, id string) (VolumeGroupSnapshot, error)

	// GetVolumeGroupSnapshotByName resolves the VolumeGroupSnapshot by the
	// name (like the request-id).
	GetVolumeGroupSnapshotByName(ctx context.Context, name string) (VolumeGroupSnapshot, error)

	// CreateVolumeGroupSnapshot instructs the Manager to create a
	// VolumeGroupSnapshot from the VolumeGroup. All snapshots in the
	// returned VolumeGroupSnapshot have been taken while I/O on the
	// VolumeGroup was paused, the snapshots in the group are crash
	// consistent.
	CreateVolumeGroupSnapshot(ctx context.Context, vg VolumeGroup, name string) (VolumeGroupSnapshot, error)

	// RegenerateVolumeGroupJournal regenerate the omap data for the volume group.
	// returns the volume group handle
	RegenerateVolumeGroupJournal(ctx context.Context, groupID, requestName string, volumeIds []string) (string, error)
}
