/*
Copyright 2019 The Ceph-CSI Authors.

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

package cephfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/golang/protobuf/ptypes/timestamp"
)

// volumeIdentifier structure contains an association between the CSI VolumeID to its subvolume
// name on the backing CephFS instance.
type volumeIdentifier struct {
	FsSubvolName string
	VolumeID     string
}

type snapshotIdentifier struct {
	FsSnapshotName string
	SnapshotID     string
	RequestName    string
	CreationTime   *timestamp.Timestamp
	FsSubvolName   string
}

/*
checkVolExists checks to determine if passed in RequestName in volOptions exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI driver generated volume name based locks are held, as otherwise racy
access to these omaps may end up leaving them in an inconsistent state.

These functions also cleanup omap reservations that are stale. I.e when omap entries exist and
backing subvolumes are missing, or one of the omaps exist and the next is missing. This is
because, the order of omap creation and deletion are inverse of each other, and protected by the
request name lock, and hence any stale omaps are leftovers from incomplete transactions and are
hence safe to garbage collect.
*/
// nolint:gocognit:gocyclo // TODO: reduce complexity
func checkVolExists(ctx context.Context,
	volOptions,
	parentVolOpt *volumeOptions,

	pvID *volumeIdentifier,
	sID *snapshotIdentifier,
	cr *util.Credentials) (*volumeIdentifier, error) {
	var vid volumeIdentifier
	// Connect to cephfs' default radosNamespace (csi)
	j, err := volJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	imageData, err := j.CheckReservation(
		ctx, volOptions.MetadataPool, volOptions.RequestName, volOptions.NamePrefix, "", "")
	if err != nil {
		return nil, err
	}
	if imageData == nil {
		return nil, nil
	}
	imageUUID := imageData.ImageUUID
	vid.FsSubvolName = imageData.ImageAttributes.ImageName

	if sID != nil || pvID != nil {
		cloneState, cloneStateErr := volOptions.getCloneState(ctx, volumeID(vid.FsSubvolName))
		if cloneStateErr != nil {
			if errors.Is(cloneStateErr, ErrVolumeNotFound) {
				if pvID != nil {
					err = cleanupCloneFromSubvolumeSnapshot(
						ctx, volumeID(pvID.FsSubvolName),
						volumeID(vid.FsSubvolName),
						parentVolOpt)
					if err != nil {
						return nil, err
					}
				}
				err = j.UndoReservation(ctx, volOptions.MetadataPool,
					volOptions.MetadataPool, vid.FsSubvolName, volOptions.RequestName)
				return nil, err
			}
			return nil, err
		}
		if cloneState == cephFSCloneInprogress {
			return nil, ErrCloneInProgress
		}
		if cloneState == cephFSClonePending {
			return nil, ErrClonePending
		}
		if cloneState == cephFSCloneFailed {
			err = volOptions.purgeVolume(ctx, volumeID(vid.FsSubvolName), true)
			if err != nil {
				util.ErrorLog(ctx, "failed to delete volume %s: %v", vid.FsSubvolName, err)
				return nil, err
			}
			if pvID != nil {
				err = cleanupCloneFromSubvolumeSnapshot(
					ctx, volumeID(pvID.FsSubvolName),
					volumeID(vid.FsSubvolName),
					parentVolOpt)
				if err != nil {
					return nil, err
				}
			}
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, vid.FsSubvolName, volOptions.RequestName)
			return nil, err
		}
		if cloneState != cephFSCloneComplete {
			return nil, fmt.Errorf("clone is not in complete state for %s", vid.FsSubvolName)
		}
	}
	volOptions.RootPath, err = volOptions.getVolumeRootPathCeph(ctx, volumeID(vid.FsSubvolName))
	if err != nil {
		if errors.Is(err, ErrVolumeNotFound) {
			// If the subvolume is not present, cleanup the stale snapshot
			// created for clone.
			if parentVolOpt != nil && pvID != nil {
				err = cleanupCloneFromSubvolumeSnapshot(ctx, volumeID(pvID.FsSubvolName), volumeID(vid.FsSubvolName), parentVolOpt)
				if err != nil {
					return nil, err
				}
			}
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, vid.FsSubvolName, volOptions.RequestName)
			return nil, err
		}
		return nil, err
	}

	// check if topology constraints match what is found
	// TODO: we need an API to fetch subvolume attributes (size/datapool and others), based
	// on which we can evaluate which topology this belongs to.
	// TODO: CephFS topology support is postponed till we get the same
	// TODO: size checks

	// found a volume already available, process and return it!
	vid.VolumeID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, imageUUID, volIDVersion)
	if err != nil {
		return nil, err
	}

	util.DebugLog(ctx, "Found existing volume (%s) with subvolume name (%s) for request (%s)",
		vid.VolumeID, vid.FsSubvolName, volOptions.RequestName)

	if parentVolOpt != nil && pvID != nil {
		err = cleanupCloneFromSubvolumeSnapshot(ctx, volumeID(pvID.FsSubvolName), volumeID(vid.FsSubvolName), parentVolOpt)
		if err != nil {
			return nil, err
		}
	}

	return &vid, nil
}

// undoVolReservation is a helper routine to undo a name reservation for a CSI VolumeName.
func undoVolReservation(ctx context.Context, volOptions *volumeOptions, vid volumeIdentifier, secret map[string]string) error {
	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return err
	}
	defer cr.DeleteCredentials()

	// Connect to cephfs' default radosNamespace (csi)
	j, err := volJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(ctx, volOptions.MetadataPool,
		volOptions.MetadataPool, vid.FsSubvolName, volOptions.RequestName)

	return err
}

func updateTopologyConstraints(volOpts *volumeOptions) error {
	// update request based on topology constrained parameters (if present)
	poolName, _, topology, err := util.FindPoolAndTopology(volOpts.TopologyPools, volOpts.TopologyRequirement)
	if err != nil {
		return err
	}
	if poolName != "" {
		volOpts.Pool = poolName
		volOpts.Topology = topology
	}

	return nil
}

// reserveVol is a helper routine to request a UUID reservation for the CSI VolumeName and,
// to generate the volume identifier for the reserved UUID.
func reserveVol(ctx context.Context, volOptions *volumeOptions, secret map[string]string) (*volumeIdentifier, error) {
	var (
		vid       volumeIdentifier
		imageUUID string
		err       error
	)

	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return nil, err
	}
	defer cr.DeleteCredentials()

	err = updateTopologyConstraints(volOptions)
	if err != nil {
		return nil, err
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := volJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	imageUUID, vid.FsSubvolName, err = j.ReserveName(
		ctx, volOptions.MetadataPool, util.InvalidPoolID,
		volOptions.MetadataPool, util.InvalidPoolID, volOptions.RequestName,
		volOptions.NamePrefix, "", "", volOptions.ReservedID, "")
	if err != nil {
		return nil, err
	}

	// generate the volume ID to return to the CO system
	vid.VolumeID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, imageUUID, volIDVersion)
	if err != nil {
		return nil, err
	}

	util.DebugLog(ctx, "Generated Volume ID (%s) and subvolume name (%s) for request name (%s)",
		vid.VolumeID, vid.FsSubvolName, volOptions.RequestName)

	return &vid, nil
}

// reserveSnap is a helper routine to request a UUID reservation for the CSI SnapName and,
// to generate the snapshot identifier for the reserved UUID.
func reserveSnap(ctx context.Context, volOptions *volumeOptions, parentSubVolName string, snap *cephfsSnapshot, cr *util.Credentials) (*snapshotIdentifier, error) {
	var (
		vid       snapshotIdentifier
		imageUUID string
		err       error
	)

	// Connect to cephfs' default radosNamespace (csi)
	j, err := snapJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	imageUUID, vid.FsSnapshotName, err = j.ReserveName(
		ctx, volOptions.MetadataPool, util.InvalidPoolID,
		volOptions.MetadataPool, util.InvalidPoolID, snap.RequestName,
		snap.NamePrefix, parentSubVolName, "", snap.ReservedID, "")
	if err != nil {
		return nil, err
	}

	// generate the snapshot ID to return to the CO system
	vid.SnapshotID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, imageUUID, volIDVersion)
	if err != nil {
		return nil, err
	}

	util.DebugLog(ctx, "Generated Snapshot ID (%s) for request name (%s)",
		vid.SnapshotID, snap.RequestName)

	return &vid, nil
}

// undoSnapReservation is a helper routine to undo a name reservation for a CSI SnapshotName.
func undoSnapReservation(ctx context.Context, volOptions *volumeOptions, vid snapshotIdentifier, snapName string, cr *util.Credentials) error {
	// Connect to cephfs' default radosNamespace (csi)
	j, err := snapJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(ctx, volOptions.MetadataPool,
		volOptions.MetadataPool, vid.FsSnapshotName, snapName)
	return err
}

/*
checkSnapExists checks to determine if passed in RequestName in volOptions exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI driver generated volume name based locks are held, as otherwise racy
access to these omaps may end up leaving them in an inconsistent state.

These functions also cleanup omap reservations that are stale. I.e when omap entries exist and
backing subvolumes are missing, or one of the omaps exist and the next is missing. This is
because, the order of omap creation and deletion are inverse of each other, and protected by the
request name lock, and hence any stale omaps are leftovers from incomplete transactions and are
hence safe to garbage collect.
*/
func checkSnapExists(
	ctx context.Context,
	volOptions *volumeOptions,
	parentSubVolName string,
	snap *cephfsSnapshot,
	cr *util.Credentials) (*snapshotIdentifier, *snapshotInfo, error) {
	// Connect to cephfs' default radosNamespace (csi)
	j, err := snapJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return nil, nil, err
	}
	defer j.Destroy()

	snapData, err := j.CheckReservation(
		ctx, volOptions.MetadataPool, snap.RequestName, snap.NamePrefix, parentSubVolName, "")
	if err != nil {
		return nil, nil, err
	}
	if snapData == nil {
		return nil, nil, nil
	}
	sid := &snapshotIdentifier{}
	snapUUID := snapData.ImageUUID
	snapID := snapData.ImageAttributes.ImageName
	sid.FsSnapshotName = snapData.ImageAttributes.ImageName
	snapInfo, err := volOptions.getSnapshotInfo(ctx, volumeID(snapID), volumeID(parentSubVolName))
	if err != nil {
		if errors.Is(err, ErrSnapNotFound) {
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, snapID, snap.RequestName)
			return nil, nil, err
		}
		return nil, nil, err
	}

	defer func() {
		if err != nil {
			err = volOptions.deleteSnapshot(ctx, volumeID(snapID), volumeID(parentSubVolName))
			if err != nil {
				util.ErrorLog(ctx, "failed to delete snapshot %s: %v", snapID, err)
				return
			}
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, snapID, snap.RequestName)
			if err != nil {
				util.ErrorLog(ctx, "removing reservation failed for snapshot %s: %v", snapID, err)
			}
		}
	}()
	tm, err := parseTime(ctx, snapInfo.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	sid.CreationTime = tm
	// found a snapshot already available, process and return it!
	sid.SnapshotID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, snapUUID, volIDVersion)
	if err != nil {
		return nil, nil, err
	}
	util.DebugLog(ctx, "Found existing snapshot (%s) with subvolume name (%s) for request (%s)",
		snapData.ImageAttributes.RequestName, parentSubVolName, sid.FsSnapshotName)

	return sid, &snapInfo, nil
}
