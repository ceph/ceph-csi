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

// Destroy frees the resources used by the rbdVolumeGroup.
func (rvgs *rbdVolumeGroupSnapshot) Destroy(ctx context.Context) {
	// nothing to do (yet)
}

func (rvgs *rbdVolumeGroupSnapshot) GetID(ctx context.Context) (string, error) {
	// FIXME: this should be the group-snapshot-handle
	return "", nil
}

func (rvgs *rbdVolumeGroupSnapshot) ListSnapshots(ctx context.Context) ([]types.Snapshot, error) {
	// TODO: use parent.journal to fetch the list of snapshots
	return nil, nil
}

func (rvgs *rbdVolumeGroupSnapshot) GetCreationTime(ctx context.Context) *time.Time {
	// TODO: fetch the creation time of the group
	// A group snapshot does not seem to have its own creation time. Use
	// the time of the most recent created snapshot.
	return nil
}

func (rvgs *rbdVolumeGroupSnapshot) GetReadyToUse(ctx context.Context) bool {
	return true
}
