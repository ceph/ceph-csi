/*
Copyright 2022 The Ceph-CSI Authors.

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

package store

import (
	"context"
	"fmt"

	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util/log"
	"github.com/ceph/ceph-csi/internal/util/reftracker"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/ceph/ceph-csi/internal/util/reftracker/reftype"
)

func fmtBackingSnapshotReftrackerName(backingSnapID string) string {
	return fmt.Sprintf("rt-backingsnapshot-%s", backingSnapID)
}

func AddSnapshotBackedVolumeRef(
	ctx context.Context,
	volOptions *VolumeOptions,
	clusterName string,
	setMetadata bool,
	secrets map[string]string,
) error {
	ioctx, err := volOptions.conn.GetIoctx(volOptions.MetadataPool)
	if err != nil {
		log.ErrorLog(ctx, "failed to create RADOS ioctx: %s", err)

		return err
	}
	defer ioctx.Destroy()

	ioctx.SetNamespace(fsutil.RadosNamespace)

	var (
		backingSnapID = volOptions.BackingSnapshotID
		ioctxW        = radoswrapper.NewIOContext(ioctx)
	)

	created, err := reftracker.Add(
		ioctxW,
		fmtBackingSnapshotReftrackerName(backingSnapID),
		map[string]struct{}{
			backingSnapID:    {},
			volOptions.VolID: {},
		},
	)
	if err != nil {
		log.ErrorLog(ctx, "failed to add refs for backing snapshot %s: %v",
			backingSnapID, err)

		return err
	}

	defer func() {
		if err == nil {
			return
		}

		// Clean up after failure.

		var deleted bool
		deleted, err = reftracker.Remove(
			ioctxW,
			fmtBackingSnapshotReftrackerName(backingSnapID),
			map[string]reftype.RefType{
				backingSnapID:    reftype.Normal,
				volOptions.VolID: reftype.Normal,
			},
		)
		if err != nil {
			log.ErrorLog(ctx, "failed to remove refs in cleanup procedure for backing snapshot %s: %v",
				backingSnapID, err)
		}

		if created && !deleted {
			log.ErrorLog(ctx, "orphaned reftracker object %s (pool %s, namespace %s)",
				backingSnapID, volOptions.MetadataPool, fsutil.RadosNamespace)
		}
	}()

	// There may have been a race between adding a ref to the reftracker and
	// deleting the backing snapshot. Make sure the snapshot still exists by
	// trying to retrieve it again.
	_, _, _, err = NewSnapshotOptionsFromID(ctx,
		volOptions.BackingSnapshotID, volOptions.conn.Creds, secrets, clusterName, setMetadata)
	if err != nil {
		log.ErrorLog(ctx, "failed to get backing snapshot %s: %v", volOptions.BackingSnapshotID, err)
	}

	return err
}

func UnrefSnapshotBackedVolume(
	ctx context.Context,
	volOptions *VolumeOptions,
) (bool, error) {
	ioctx, err := volOptions.conn.GetIoctx(volOptions.MetadataPool)
	if err != nil {
		log.ErrorLog(ctx, "failed to create RADOS ioctx: %s", err)

		return false, err
	}
	defer ioctx.Destroy()

	ioctx.SetNamespace(fsutil.RadosNamespace)

	var (
		backingSnapID = volOptions.BackingSnapshotID
		ioctxW        = radoswrapper.NewIOContext(ioctx)
	)

	deleted, err := reftracker.Remove(
		ioctxW,
		fmtBackingSnapshotReftrackerName(backingSnapID),
		map[string]reftype.RefType{
			volOptions.VolID: reftype.Normal,
		},
	)
	if err != nil {
		log.ErrorLog(ctx, "failed to remove refs for backing snapshot %s: %v",
			backingSnapID, err)

		return false, err
	}

	return deleted, err
}

// UnrefSelfInSnapshotBackedVolumes removes (masks) snapshot ID in the
// reftracker for volumes backed by this snapshot. The returned boolean
// value signals whether the snapshot is not referenced by any such volumes
// and needs to be removed.
func UnrefSelfInSnapshotBackedVolumes(
	ctx context.Context,
	snapParentVolOptions *VolumeOptions,
	snapshotID string,
) (bool, error) {
	ioctx, err := snapParentVolOptions.conn.GetIoctx(snapParentVolOptions.MetadataPool)
	if err != nil {
		log.ErrorLog(ctx, "failed to create RADOS ioctx: %s", err)

		return false, err
	}
	defer ioctx.Destroy()

	ioctx.SetNamespace(fsutil.RadosNamespace)

	return reftracker.Remove(
		radoswrapper.NewIOContext(ioctx),
		fmtBackingSnapshotReftrackerName(snapshotID),
		map[string]reftype.RefType{
			snapshotID: reftype.Mask,
		},
	)
}
