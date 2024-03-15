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
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"

	types "github.com/ceph/ceph-csi/internal/rbd_types"
)

// verify that rbdSnapshot type implements the Snapshot interface
var _ types.Snapshot = &rbdGroupSnapshot{}

// rbdGroupSnapshot describes a single snapshot that was taken as part of a group.
type rbdGroupSnapshot struct {
	parent   types.Volume
	snapName string
	snapID   uint64 // not needed now, may be used for cloning in the future

	// group is the optional value for a VolumeGroup that was used for
	group types.VolumeGroup
}

func newGroupSnapshot(group, name string, snapID uint64) types.Snapshot {
	return &rbdGroupSnapshot{
		//groupName: group,
		snapName: name,
		snapID:   snapID,
	}
}

func (rgs *rbdGroupSnapshot) Destroy(ctx context.Context) {
	// nothing to do yet
}

// String returns the image-spec of the snapshot.
func (rgs *rbdGroupSnapshot) String() string {
	return fmt.Sprintf("%s@%s", rgs.parent, rgs.snapName)
}

func (rgs *rbdGroupSnapshot) GetCreationTime(ctx context.Context) (*time.Time, error) {
	return nil, nil
}

func (rgs *rbdGroupSnapshot) GetReadyToUse(ctx context.Context) (bool, error) {
	return false, nil
}

func (rgs *rbdGroupSnapshot) ToCSISnapshot(ctx context.Context) (*csi.Snapshot, error) {
	parentID, err := rgs.parent.GetID(ctx)
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

func (rgs *rbdGroupSnapshot) Map(ctx context.Context) (string, error) {
	return "/dev/rbd123", nil
}
