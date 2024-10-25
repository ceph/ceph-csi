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

package group

import (
	"context"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// volumeGroupSnapshot handles all requests for 'rbd group snap' operations.
type volumeGroupSnapshot struct {
	commonVolumeGroup

	// snapshots is a list of rbd-images that are part of the group. The ID
	// of each snapshot is stored in the journal.
	snapshots []types.Snapshot

	// snapshotsToFree contains Snapshots that were resolved during
	// GetVolumeGroupSnapshot.
	snapshotsToFree []types.Snapshot
}

// verify that volumeGroupSnapshot implements the VolumeGroupSnapshot interface.
var _ types.VolumeGroupSnapshot = &volumeGroupSnapshot{}

// GetVolumeGroupSnapshot initializes a new VolumeGroupSnapshot object that can
// be used to inspect and delete a group of snapshots that was created by a
// VolumeGroup.
func GetVolumeGroupSnapshot(
	ctx context.Context,
	id string,
	csiDriver string,
	creds *util.Credentials,
	snapshotResolver types.SnapshotResolver,
) (types.VolumeGroupSnapshot, error) {
	cleanVGS := true

	vgs := &volumeGroupSnapshot{}
	err := vgs.initCommonVolumeGroup(ctx, id, csiDriver, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize volume group snapshot with id %q: %w", id, err)
	}
	defer func() {
		if cleanVGS {
			vgs.Destroy(ctx)
		}
	}()

	attrs, err := vgs.getVolumeGroupAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume attributes for id %q: %w", vgs, err)
	}

	var snapshots []types.Snapshot
	// it is needed to free the previously allocated snapshots in case of an error
	defer func() {
		// snapshotsToFree is empty in case of an error, let .Destroy() handle it otherwise
		if len(vgs.snapshotsToFree) > 0 {
			return
		}

		for _, s := range snapshots {
			s.Destroy(ctx)
		}
	}()
	for snapID := range attrs.VolumeMap {
		snap, err := snapshotResolver.GetSnapshotByID(ctx, snapID)
		if err != nil {
			// free the previously allocated snapshots
			for _, s := range snapshots {
				s.Destroy(ctx)
			}

			return nil, fmt.Errorf("failed to resolve snapshot image %q for volume group snapshot %q: %w", snapID, vgs, err)
		}

		log.DebugLog(ctx, "resolved snapshot id %q to snapshot %q", snapID, snap)

		snapshots = append(snapshots, snap)
	}

	vgs.snapshots = snapshots
	// all allocated snapshots need to be free'd at Destroy() time
	vgs.snapshotsToFree = snapshots

	cleanVGS = false
	log.DebugLog(ctx, "GetVolumeGroupSnapshot(%s) returns %+v", id, *vgs)

	return vgs, nil
}

// NewVolumeGroupSnapshot creates a new VolumeGroupSnapshot object with the
// given slice of Snapshots and adds the objectmapping to the journal.
func NewVolumeGroupSnapshot(
	ctx context.Context,
	id string,
	csiDriver string,
	creds *util.Credentials,
	snapshots []types.Snapshot,
) (types.VolumeGroupSnapshot, error) {
	cleanupVGS := true

	vgs := &volumeGroupSnapshot{}
	err := vgs.initCommonVolumeGroup(ctx, id, csiDriver, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize volume group snapshot with id %q: %w", id, err)
	}
	defer func() {
		if cleanupVGS {
			vgs.Destroy(ctx)
		}
	}()

	vgs.snapshots = snapshots
	vgs.snapshotsToFree = snapshots

	_ /* attrs */, err = vgs.getVolumeGroupAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume attributes for id %q: %w", vgs, err)
	}

	volumeMap := make(map[string]string, len(snapshots))

	// add the CSI handles of each snapshot to the journal
	for _, snapshot := range snapshots {
		handle, snapErr := snapshot.GetID(ctx)
		if snapErr != nil {
			return nil, fmt.Errorf("failed to get ID for snapshot %q of volume group snapshot %q: %w", snapshot, vgs, snapErr)
		}

		name, snapErr := snapshot.GetName(ctx)
		if snapErr != nil {
			return nil, fmt.Errorf("failed to get name for snapshot %q of volume group snapshot %q: %w", snapshot, vgs, snapErr)
		}

		volumeMap[handle] = name

		// store the CSI ID of the group in the snapshot attributes
		snapErr = snapshot.SetVolumeGroup(ctx, creds, vgs.id)
		if snapErr != nil {
			return nil, fmt.Errorf("failed to set volume group ID %q for snapshot %q: %w", vgs.id, name, snapErr)
		}
	}

	j, err := vgs.getJournal(ctx)
	if err != nil {
		return nil, err
	}

	err = j.AddVolumesMapping(ctx, vgs.pool, vgs.objectUUID, volumeMap)
	if err != nil {
		return nil, fmt.Errorf("failed to add volume mapping for volume group snapshot %q: %w", vgs, err)
	}

	// all done successfully, no need to cleanup the returned vgs
	cleanupVGS = false

	return vgs, nil
}

// ToCSI creates a CSI type for the VolumeGroupSnapshot.
func (vgs *volumeGroupSnapshot) ToCSI(ctx context.Context) (*csi.VolumeGroupSnapshot, error) {
	snapshots, err := vgs.ListSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots for volume group %q: %w", vgs, err)
	}

	csiSnapshots := make([]*csi.Snapshot, len(snapshots))
	for i, snap := range snapshots {
		csiSnapshots[i], err = snap.ToCSI(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to convert snapshot %q to CSI type: %w", snap, err)
		}
	}

	id, err := vgs.GetID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get id for volume group snapshot %q: %w", vgs, err)
	}

	ct, err := vgs.GetCreationTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get creation time for volume group snapshot %q: %w", vgs, err)
	}

	return &csi.VolumeGroupSnapshot{
		GroupSnapshotId: id,
		Snapshots:       csiSnapshots,
		CreationTime:    timestamppb.New(*ct),
		ReadyToUse:      true,
	}, nil
}

// Destroy frees the resources used by the volumeGroupSnapshot.
func (vgs *volumeGroupSnapshot) Destroy(ctx context.Context) {
	// free the volumes that were allocated in GetVolumeGroup()
	if len(vgs.snapshotsToFree) > 0 {
		for _, volume := range vgs.snapshotsToFree {
			volume.Destroy(ctx)
		}
		vgs.snapshotsToFree = make([]types.Snapshot, 0)
	}

	vgs.commonVolumeGroup.Destroy(ctx)
}

// Delete removes all snapshots and eventually the volume group snapshot.
func (vgs *volumeGroupSnapshot) Delete(ctx context.Context) error {
	for _, snapshot := range vgs.snapshots {
		log.DebugLog(ctx, "deleting snapshot image %q for volume group snapshot %q", snapshot, vgs)

		err := snapshot.Delete(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete snapshot %q as part of volume groups snapshot %q: %w", snapshot, vgs, err)
		}
	}

	return vgs.commonVolumeGroup.Delete(ctx)
}

func (vgs *volumeGroupSnapshot) ListSnapshots(ctx context.Context) ([]types.Snapshot, error) {
	return vgs.snapshots, nil
}
