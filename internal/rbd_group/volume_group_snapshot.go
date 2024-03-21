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

	"github.com/container-storage-interface/spec/lib/go/csi"

	types "github.com/ceph/ceph-csi/internal/rbd_types"
)

// verify that volumeGroupSnapshot implements the VolumeGroupSnapshot interface
var _ types.VolumeGroupSnapshot = &volumeGroupSnapshot{}

// volumeGroupSnapshot is a description of a group snapshot that was taken
// at some point in time. The rbd-group still exists, but there are no
// rbd-images in the group anymore. The list of snapshots that belongs to the
// group is stored in the journal.
type volumeGroupSnapshot struct {
	*groupObject

	parentGroup *volumeGroup

	snapshots []*groupSnapshot
}

func newVolumeGroupSnapshot(ctx context.Context, parent *volumeGroup, name string) types.VolumeGroupSnapshot {
	vgs := &volumeGroupSnapshot{
		groupObject: &groupObject{
			name: name,
		},
		parentGroup: parent,
	}

	return vgs
}

func GetVolumeGroupSnapshot(ctx context.Context, id string, secrets map[string]string) (types.VolumeGroupSnapshot, error) {
	// TODO: use the journal to resolve the volumeGroupSnapshot by id
	// TODO: for each snapshot ID in the group, use GetSnapshot() to resolve the ID to a Snapshot

	vgs := &volumeGroupSnapshot{}
	err := vgs.resolveByID(ctx, id, secrets)
	if err != nil {
		return nil, err
	}

	vgs.parentGroup = nil // TODO: use GetVolumeGroup(id) to get the volume group

	return vgs, nil
}

// Destroy frees the resources used by the rbdVolumeGroup.
func (vgs *volumeGroupSnapshot) Destroy(ctx context.Context) {
	// nothing to do (yet)

	vgs.groupObject.Destroy(ctx)
}

func (vgs *volumeGroupSnapshot) Delete(ctx context.Context) error {
	return vgs.parentGroup.deleteSnapshot(ctx, vgs.name)
}

func (vgs *volumeGroupSnapshot) ListSnapshots(ctx context.Context) ([]types.Snapshot, error) {
	// TODO: use parent.journal to fetch the list of snapshots
	return nil, nil
}

func (vgs *volumeGroupSnapshot) ToCSIVolumeGroupSnapshot(ctx context.Context) (*csi.VolumeGroupSnapshot, error) {
	groupSnapshotID, err := vgs.GetID(ctx)
	if err != nil {
		return nil, err
	}

	snapshots, err := vgs.ListSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	// assume all snapshots are ready, set to false in case a single
	// snapshot is not ready yet
	ready := true

	csiSnapshots := make([]*csi.Snapshot, len(snapshots))
	for i, snapshot := range snapshots {
		csiSnapshot, err := snapshot.ToCSISnapshot(ctx)
		if err != nil {
			return nil, err
		}

		csiSnapshots[i] = csiSnapshot

		// in case the csiSnapshot is not ready, this vgs is not ready either
		ready = ready && csiSnapshot.GetReadyToUse()
	}

	return &csi.VolumeGroupSnapshot{
		GroupSnapshotId: groupSnapshotID,
		Snapshots:       csiSnapshots,
		CreationTime:    nil,
		ReadyToUse:      ready,
	}, nil
}
