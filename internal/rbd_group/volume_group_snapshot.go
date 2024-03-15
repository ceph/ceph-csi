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

package rbd_group

import (
	"context"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"

	types "github.com/ceph/ceph-csi/internal/rbd_types"
)

// verify that rbdVolumeGroupSnapshot implements the VolumeGroupSnapshot interface
var _ types.VolumeGroupSnapshot = &rbdVolumeGroupSnapshot{}

// rbdVolumeGroupSnapshot is a description of a group snapshot that was taken
// at some point in time. The rbd-group still exists, but there are no
// rbd-images in the group anymore. The list of snapshots that belongs to the
// group is stored in the journal.
type rbdVolumeGroupSnapshot struct {
	parentGroup *rbdVolumeGroup
	name        string

	snapshots []types.Snapshot
}

func newVolumeGroupSnapshot(ctx context.Context, parent *rbdVolumeGroup, name string) types.VolumeGroupSnapshot {
	return &rbdVolumeGroupSnapshot{
		parentGroup: parent,
		name:        name,
	}
}

func GetVolumeGroupSnapshot(ctx context.Context, id string, secrets map[string]string) (types.VolumeGroupSnapshot, error) {
	// TODO: resolve the VolumeGroupSnapshot by id
	return nil, nil
}

// Destroy frees the resources used by the rbdVolumeGroup.
func (rvgs *rbdVolumeGroupSnapshot) Destroy(ctx context.Context) {
	// nothing to do (yet)
}

func (rvgs *rbdVolumeGroupSnapshot) Delete(ctx context.Context) error {
	return nil
}

func (rvgs *rbdVolumeGroupSnapshot) GetID(ctx context.Context) (string, error) {
	// FIXME: this should be the group-snapshot-handle
	return "", nil
}

func (rvgs *rbdVolumeGroupSnapshot) ListSnapshots(ctx context.Context) ([]types.Snapshot, error) {
	// TODO: use parent.journal to fetch the list of snapshots
	return nil, nil
}

func (rvgs *rbdVolumeGroupSnapshot) GetCreationTime(ctx context.Context) (*time.Time, error) {
	// TODO: fetch the creation time of the group
	// A group snapshot does not seem to have its own creation time. Use
	// the time of the most recent created snapshot.
	return nil, nil
}

// GetReadyToUse checks if all snapshots that are part if the group are ready
// to use.
func (rvgs *rbdVolumeGroupSnapshot) GetReadyToUse(ctx context.Context) (bool, error) {
	for _, snapshot := range rvgs.snapshots {
		ready, err := snapshot.GetReadyToUse(ctx)
		if err != nil {
			return false, err
		}

		if !ready {
			// if this snapshot is not ready, no need to check
			// other snapshots
			return false, nil
		}
	}

	return true, nil
}

func (rvgs *rbdVolumeGroupSnapshot) ToCSIVolumeGroupSnapshot(ctx context.Context) (*csi.VolumeGroupSnapshot, error) {
	groupSnapshotID, err := rvgs.GetID(ctx)
	if err != nil {
		return nil, err
	}

	snapshots, err := rvgs.ListSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	csiSnapshots := make([]*csi.Snapshot, len(snapshots))
	for i, snapshot := range snapshots {
		csiSnapshot, err := snapshot.ToCSISnapshot(ctx)
		if err != nil {
			return nil, err
		}

		csiSnapshots[i] = csiSnapshot
	}

	ready, err := rvgs.GetReadyToUse(ctx)
	if err != nil {
		return nil, err
	}

	return &csi.VolumeGroupSnapshot{
		GroupSnapshotId: groupSnapshotID,
		Snapshots:       csiSnapshots,
		CreationTime:    nil,
		ReadyToUse:      ready,
	}, nil
}
