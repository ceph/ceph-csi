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

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/cephfs/core"
	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// VolJournal is used to maintain RADOS based journals for CO generated.
	// VolumeName to backing CephFS subvolumes.
	VolJournal *journal.Config

	// SnapJournal is used to maintain RADOS based journals for CO generated.
	// SnapshotName to backing CephFS subvolumes.
	SnapJournal *journal.Config
)

// VolumeIdentifier structure contains an association between the CSI VolumeID to its subvolume
// name on the backing CephFS instance.
type VolumeIdentifier struct {
	FsSubvolName string
	VolumeID     string
}

type SnapshotIdentifier struct {
	FsSnapshotName string
	SnapshotID     string
	RequestName    string
	CreationTime   *timestamp.Timestamp
	FsSubvolName   string
}

/*
CheckVolExists checks to determine if passed in RequestName in volOptions exists on the backend.

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
//nolint:gocognit,gocyclo,nestif,cyclop // TODO: reduce complexity
func CheckVolExists(ctx context.Context,
	volOptions,
	parentVolOpt *VolumeOptions,

	pvID *VolumeIdentifier,
	sID *SnapshotIdentifier,
	cr *util.Credentials,
	clusterName string,
	setMetadata bool,
) (*VolumeIdentifier, error) {
	var vid VolumeIdentifier
	// Connect to cephfs' default radosNamespace (csi)
	j, err := VolJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	kmsID, encryptionType := getEncryptionConfig(volOptions)

	imageData, err := j.CheckReservation(
		ctx, volOptions.MetadataPool, volOptions.RequestName, volOptions.NamePrefix, "", kmsID, encryptionType)
	if err != nil {
		return nil, err
	}
	if imageData == nil {
		return nil, nil
	}
	imageUUID := imageData.ImageUUID
	vid.FsSubvolName = imageData.ImageAttributes.ImageName
	volOptions.VolID = vid.FsSubvolName

	vol := core.NewSubVolume(volOptions.conn, &volOptions.SubVolume, volOptions.ClusterID, clusterName, setMetadata)
	if (sID != nil || pvID != nil) && imageData.ImageAttributes.BackingSnapshotID == "" {
		cloneState, cloneStateErr := vol.GetCloneState(ctx)
		if cloneStateErr != nil {
			if errors.Is(cloneStateErr, cerrors.ErrVolumeNotFound) {
				if pvID != nil {
					err = vol.CleanupSnapshotFromSubvolume(
						ctx, &parentVolOpt.SubVolume)
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
		err = cloneState.ToError()
		if errors.Is(err, cerrors.ErrCloneInProgress) {
			return nil, err
		}
		if errors.Is(err, cerrors.ErrClonePending) {
			return nil, err
		}
		if errors.Is(err, cerrors.ErrCloneFailed) {
			log.ErrorLog(ctx,
				"clone failed (%v), deleting subvolume clone. vol=%s, subvol=%s subvolgroup=%s",
				err,
				volOptions.FsName,
				vid.FsSubvolName,
				volOptions.SubvolumeGroup)
			err = vol.PurgeVolume(ctx, true)
			if err != nil {
				log.ErrorLog(ctx, "failed to delete volume %s: %v", vid.FsSubvolName, err)

				return nil, err
			}
			if pvID != nil {
				err = vol.CleanupSnapshotFromSubvolume(
					ctx, &parentVolOpt.SubVolume)
				if err != nil {
					return nil, err
				}
			}
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, vid.FsSubvolName, volOptions.RequestName)

			return nil, err
		}
		if err != nil {
			return nil, fmt.Errorf("clone is not in complete state for %s: %w", vid.FsSubvolName, err)
		}
	}

	if imageData.ImageAttributes.BackingSnapshotID == "" {
		volOptions.RootPath, err = vol.GetVolumeRootPathCeph(ctx)
		if err != nil {
			if errors.Is(err, cerrors.ErrVolumeNotFound) {
				// If the subvolume is not present, cleanup the stale snapshot
				// created for clone.
				if parentVolOpt != nil && pvID != nil {
					err = vol.CleanupSnapshotFromSubvolume(
						ctx, &parentVolOpt.SubVolume)
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
	}

	// check if topology constraints match what is found
	// TODO: we need an API to fetch subvolume attributes (size/datapool and others), based
	// on which we can evaluate which topology this belongs to.
	// TODO: CephFS topology support is postponed till we get the same
	// TODO: size checks

	// found a volume already available, process and return it!
	vid.VolumeID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, imageUUID, fsutil.VolIDVersion)
	if err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "Found existing volume (%s) with subvolume name (%s) for request (%s)",
		vid.VolumeID, vid.FsSubvolName, volOptions.RequestName)

	if parentVolOpt != nil && pvID != nil {
		err = vol.CleanupSnapshotFromSubvolume(
			ctx, &parentVolOpt.SubVolume)
		if err != nil {
			return nil, err
		}
	}

	return &vid, nil
}

// UndoVolReservation is a helper routine to undo a name reservation for a CSI VolumeName.
func UndoVolReservation(
	ctx context.Context,
	volOptions *VolumeOptions,
	vid VolumeIdentifier,
	secret map[string]string,
) error {
	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return err
	}
	defer cr.DeleteCredentials()

	// Connect to cephfs' default radosNamespace (csi)
	j, err := VolJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(ctx, volOptions.MetadataPool,
		volOptions.MetadataPool, vid.FsSubvolName, volOptions.RequestName)

	return err
}

func updateTopologyConstraints(volOpts *VolumeOptions) error {
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

func getEncryptionConfig(volOptions *VolumeOptions) (string, util.EncryptionType) {
	if volOptions.IsEncrypted() {
		return volOptions.Encryption.GetID(), util.EncryptionTypeFile
	}

	return "", util.EncryptionTypeNone
}

// ReserveVol is a helper routine to request a UUID reservation for the CSI VolumeName and,
// to generate the volume identifier for the reserved UUID.
func ReserveVol(ctx context.Context, volOptions *VolumeOptions, secret map[string]string) (*VolumeIdentifier, error) {
	var (
		vid       VolumeIdentifier
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
	j, err := VolJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	kmsID, encryptionType := getEncryptionConfig(volOptions)

	imageUUID, vid.FsSubvolName, err = j.ReserveName(
		ctx, volOptions.MetadataPool, util.InvalidPoolID,
		volOptions.MetadataPool, util.InvalidPoolID, volOptions.RequestName,
		volOptions.NamePrefix, "", kmsID, volOptions.ReservedID, volOptions.Owner,
		volOptions.BackingSnapshotID, encryptionType)
	if err != nil {
		return nil, err
	}
	volOptions.VolID = vid.FsSubvolName
	// generate the volume ID to return to the CO system
	vid.VolumeID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, imageUUID, fsutil.VolIDVersion)
	if err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "Generated Volume ID (%s) and subvolume name (%s) for request name (%s)",
		vid.VolumeID, vid.FsSubvolName, volOptions.RequestName)

	return &vid, nil
}

// ReserveSnap is a helper routine to request a UUID reservation for the CSI SnapName and,
// to generate the snapshot identifier for the reserved UUID.
func ReserveSnap(
	ctx context.Context,
	volOptions *VolumeOptions,
	parentSubVolName string,
	snap *SnapshotOption,
	cr *util.Credentials,
) (*SnapshotIdentifier, error) {
	var (
		vid       SnapshotIdentifier
		imageUUID string
		err       error
	)

	// Connect to cephfs' default radosNamespace (csi)
	j, err := SnapJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	kmsID, encryptionType := getEncryptionConfig(volOptions)

	imageUUID, vid.FsSnapshotName, err = j.ReserveName(
		ctx, volOptions.MetadataPool, util.InvalidPoolID,
		volOptions.MetadataPool, util.InvalidPoolID, snap.RequestName,
		snap.NamePrefix, parentSubVolName, kmsID, snap.ReservedID, "",
		volOptions.Owner, encryptionType)
	if err != nil {
		return nil, err
	}

	// generate the snapshot ID to return to the CO system
	vid.SnapshotID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, imageUUID, fsutil.VolIDVersion)
	if err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "Generated Snapshot ID (%s) for request name (%s)",
		vid.SnapshotID, snap.RequestName)

	return &vid, nil
}

// UndoSnapReservation is a helper routine to undo a name reservation for a CSI SnapshotName.
func UndoSnapReservation(
	ctx context.Context,
	volOptions *VolumeOptions,
	vid SnapshotIdentifier,
	snapName string,
	cr *util.Credentials,
) error {
	// Connect to cephfs' default radosNamespace (csi)
	j, err := SnapJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(ctx, volOptions.MetadataPool,
		volOptions.MetadataPool, vid.FsSnapshotName, snapName)

	return err
}

/*
CheckSnapExists checks to determine if passed in RequestName in volOptions exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI driver generated volume name based locks are held, as otherwise racy
access to these omaps may end up leaving them in an inconsistent state.

These functions also cleanup omap reservations that are stale. I.e. when omap entries exist and
backing subvolumes are missing, or one of the omaps exist and the next is missing. This is
because, the order of omap creation and deletion are inverse of each other, and protected by the
request name lock, and hence any stale omaps are leftovers from incomplete transactions and are
hence safe to garbage collect.
*/
func CheckSnapExists(
	ctx context.Context,
	volOptions *VolumeOptions,
	snap *SnapshotOption,
	clusterName string,
	setMetadata bool,
	cr *util.Credentials,
) (*SnapshotIdentifier, *core.SnapshotInfo, error) {
	// Connect to cephfs' default radosNamespace (csi)
	j, err := SnapJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return nil, nil, err
	}
	defer j.Destroy()

	kmsID, encryptionType := getEncryptionConfig(volOptions)

	snapData, err := j.CheckReservation(
		ctx, volOptions.MetadataPool, snap.RequestName, snap.NamePrefix, volOptions.VolID, kmsID, encryptionType)
	if err != nil {
		return nil, nil, err
	}
	if snapData == nil {
		return nil, nil, nil
	}
	sid := &SnapshotIdentifier{}
	snapUUID := snapData.ImageUUID
	snapID := snapData.ImageAttributes.ImageName
	sid.FsSnapshotName = snapData.ImageAttributes.ImageName
	snapClient := core.NewSnapshot(volOptions.conn, snapID,
		volOptions.ClusterID, clusterName, setMetadata, &volOptions.SubVolume)
	snapInfo, err := snapClient.GetSnapshotInfo(ctx)
	if err != nil {
		if errors.Is(err, cerrors.ErrSnapNotFound) {
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, snapID, snap.RequestName)

			return nil, nil, err
		}

		return nil, nil, err
	}

	defer func() {
		if err != nil {
			err = snapClient.DeleteSnapshot(ctx)
			if err != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %s: %v", snapID, err)

				return
			}
			err = j.UndoReservation(ctx, volOptions.MetadataPool,
				volOptions.MetadataPool, snapID, snap.RequestName)
			if err != nil {
				log.ErrorLog(ctx, "removing reservation failed for snapshot %s: %v", snapID, err)
			}
		}
	}()
	sid.CreationTime = timestamppb.New(snapInfo.CreatedAt)

	// found a snapshot already available, process and return it!
	sid.SnapshotID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, snapUUID, fsutil.VolIDVersion)
	if err != nil {
		return nil, nil, err
	}
	log.DebugLog(ctx, "Found existing snapshot (%s) with subvolume name (%s) for request (%s)",
		snapData.ImageAttributes.RequestName, volOptions.VolID, sid.FsSnapshotName)

	return sid, &snapInfo, nil
}
