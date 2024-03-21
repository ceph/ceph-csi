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
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"

	types "github.com/ceph/ceph-csi/internal/rbd_types"
)

// verify that rbdSnapshot type implements the Snapshot interface
var _ types.Snapshot = &groupSnapshot{}

// groupSnapshot describes a single snapshot that was taken as part of a group.
type groupSnapshot struct {
	*groupObject

	// parent is the parent RBD-image that was snapshotted
	parent types.Volume

	// snapID is the (RBD) ID of the snapshot, may be used for cloning in the future
	snapID uint64

	// group is the optional value for a VolumeGroup that was used for
	group *volumeGroup
}

func newGroupSnapshot(group *volumeGroup, parent types.Volume, name string, snapID uint64) types.Snapshot {
	gs := &groupSnapshot{
		groupObject: &groupObject{
			name: name,
		},
		parent: parent,
		snapID: snapID,
		group:  group,
	}

	return gs
}

// GetSnapshot returns a Snapshot by the given id.
func GetSnapshot(ctx context.Context, id string, secrets map[string]string) (types.Snapshot, error) {
	// TODO: use the journal to resolve the groupSnapshot by ID

	gs := &groupSnapshot{}
	err := gs.resolveByID(ctx, id, secrets)
	if err != nil {
		return nil, err
	}

	// TODO: resolve more attributes from the journal
	gs.parent = nil
	gs.group = nil

	return gs, nil
}

// String returns the image-spec of the snapshot.
func (gs *groupSnapshot) String() string {
	return fmt.Sprintf("%s@%s", gs.parent, gs.name)
}

func (gs *groupSnapshot) Destroy(ctx context.Context) {
	// nothing to do yet

	gs.groupObject.Destroy(ctx)
}

func (gs *groupSnapshot) Delete(ctx context.Context) error {
	// TODO: fail in case the parent group still exists
	// TODO: remove object from the journal
	return nil
}

func (gs *groupSnapshot) ToCSISnapshot(ctx context.Context) (*csi.Snapshot, error) {
	parentID, err := gs.parent.GetID(ctx)
	if err != nil {
		return nil, err
	}

	return &csi.Snapshot{
		SizeBytes:       0,
		SnapshotId:      "",
		SourceVolumeId:  "",
		CreationTime:    nil,
		ReadyToUse:      false,
		GroupSnapshotId: parentID,
	}, nil
}
