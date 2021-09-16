/*
Copyright 2020 The Ceph-CSI Authors.

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

package core

import (
	"context"
	"errors"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// cephFSCloneState describes the status of the clone.
type cephFSCloneState string

const (
	// cephFSCloneError indicates that fetching the clone state returned an error.
	cephFSCloneError = cephFSCloneState("")
	// cephFSCloneFailed indicates that clone is in failed state.
	cephFSCloneFailed = cephFSCloneState("failed")
	// cephFSClonePending indicates that clone is in pending state.
	cephFSClonePending = cephFSCloneState("pending")
	// cephFSCloneInprogress indicates that clone is in in-progress state.
	cephFSCloneInprogress = cephFSCloneState("in-progress")
	// cephFSCloneComplete indicates that clone is in complete state.
	cephFSCloneComplete = cephFSCloneState("complete")

	// SnapshotIsProtected string indicates that the snapshot is currently protected.
	SnapshotIsProtected = "yes"
)

// toError checks the state of the clone if it's not cephFSCloneComplete.
func (cs cephFSCloneState) toError() error {
	switch cs {
	case cephFSCloneComplete:
		return nil
	case cephFSCloneError:
		return cerrors.ErrInvalidClone
	case cephFSCloneInprogress:
		return cerrors.ErrCloneInProgress
	case cephFSClonePending:
		return cerrors.ErrClonePending
	case cephFSCloneFailed:
		return cerrors.ErrCloneFailed
	}

	return nil
}

func CreateCloneFromSubvolume(
	ctx context.Context,
	volID, cloneID fsutil.VolumeID,
	volOpt,
	parentvolOpt *VolumeOptions) error {
	snapshotID := cloneID
	err := parentvolOpt.CreateSnapshot(ctx, snapshotID, volID)
	if err != nil {
		log.ErrorLog(ctx, "failed to create snapshot %s %v", snapshotID, err)

		return err
	}
	var (
		// if protectErr is not nil we will delete the snapshot as the protect fails
		protectErr error
		// if cloneErr is not nil we will unprotect the snapshot and delete the snapshot
		cloneErr error
	)
	defer func() {
		if protectErr != nil {
			err = parentvolOpt.DeleteSnapshot(ctx, snapshotID, volID)
			if err != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}

		if cloneErr != nil {
			if err = volOpt.PurgeVolume(ctx, cloneID, true); err != nil {
				log.ErrorLog(ctx, "failed to delete volume %s: %v", cloneID, err)
			}
			if err = parentvolOpt.UnprotectSnapshot(ctx, snapshotID, volID); err != nil {
				// In case the snap is already unprotected we get ErrSnapProtectionExist error code
				// in that case we are safe and we could discard this error and we are good to go
				// ahead with deletion
				if !errors.Is(err, cerrors.ErrSnapProtectionExist) {
					log.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)
				}
			}
			if err = parentvolOpt.DeleteSnapshot(ctx, snapshotID, volID); err != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}
	}()
	protectErr = parentvolOpt.ProtectSnapshot(ctx, snapshotID, volID)
	if protectErr != nil {
		log.ErrorLog(ctx, "failed to protect snapshot %s %v", snapshotID, protectErr)

		return protectErr
	}

	cloneErr = parentvolOpt.cloneSnapshot(ctx, volID, snapshotID, cloneID, volOpt)
	if cloneErr != nil {
		log.ErrorLog(ctx, "failed to clone snapshot %s %s to %s %v", volID, snapshotID, cloneID, cloneErr)

		return cloneErr
	}

	cloneState, cloneErr := volOpt.getCloneState(ctx, cloneID)
	if cloneErr != nil {
		log.ErrorLog(ctx, "failed to get clone state: %v", cloneErr)

		return cloneErr
	}

	if cloneState != cephFSCloneComplete {
		log.ErrorLog(ctx, "clone %s did not complete: %v", cloneID, cloneState.toError())

		return cloneState.toError()
	}
	// This is a work around to fix sizing issue for cloned images
	err = volOpt.ResizeVolume(ctx, cloneID, volOpt.Size)
	if err != nil {
		log.ErrorLog(ctx, "failed to expand volume %s: %v", cloneID, err)

		return err
	}
	// As we completed clone, remove the intermediate snap
	if err = parentvolOpt.UnprotectSnapshot(ctx, snapshotID, volID); err != nil {
		// In case the snap is already unprotected we get ErrSnapProtectionExist error code
		// in that case we are safe and we could discard this error and we are good to go
		// ahead with deletion
		if !errors.Is(err, cerrors.ErrSnapProtectionExist) {
			log.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)

			return err
		}
	}
	if err = parentvolOpt.DeleteSnapshot(ctx, snapshotID, volID); err != nil {
		log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)

		return err
	}

	return nil
}

func cleanupCloneFromSubvolumeSnapshot(
	ctx context.Context,
	volID, cloneID fsutil.VolumeID,
	parentVolOpt *VolumeOptions) error {
	// snapshot name is same as clone name as we need a name which can be
	// identified during PVC-PVC cloning.
	snapShotID := cloneID
	snapInfo, err := parentVolOpt.GetSnapshotInfo(ctx, snapShotID, volID)
	if err != nil {
		if errors.Is(err, cerrors.ErrSnapNotFound) {
			return nil
		}

		return err
	}

	if snapInfo.Protected == SnapshotIsProtected {
		err = parentVolOpt.UnprotectSnapshot(ctx, snapShotID, volID)
		if err != nil {
			log.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapShotID, err)

			return err
		}
	}
	err = parentVolOpt.DeleteSnapshot(ctx, snapShotID, volID)
	if err != nil {
		log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapShotID, err)

		return err
	}

	return nil
}

func CreateCloneFromSnapshot(
	ctx context.Context,
	parentVolOpt, volOptions *VolumeOptions,
	vID *VolumeIdentifier,
	sID *SnapshotIdentifier) error {
	snapID := fsutil.VolumeID(sID.FsSnapshotName)
	err := parentVolOpt.cloneSnapshot(
		ctx,
		fsutil.VolumeID(sID.FsSubvolName),
		snapID,
		fsutil.VolumeID(vID.FsSubvolName),
		volOptions)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if !cerrors.IsCloneRetryError(err) {
				if dErr := volOptions.PurgeVolume(ctx, fsutil.VolumeID(vID.FsSubvolName), true); dErr != nil {
					log.ErrorLog(ctx, "failed to delete volume %s: %v", vID.FsSubvolName, dErr)
				}
			}
		}
	}()

	cloneState, err := volOptions.getCloneState(ctx, fsutil.VolumeID(vID.FsSubvolName))
	if err != nil {
		log.ErrorLog(ctx, "failed to get clone state: %v", err)

		return err
	}

	if cloneState != cephFSCloneComplete {
		return cloneState.toError()
	}
	// The clonedvolume currently does not reflect the proper size due to an issue in cephfs
	// however this is getting addressed in cephfs and the parentvolume size will be reflected
	// in the new cloned volume too. Till then we are explicitly making the size set
	err = volOptions.ResizeVolume(ctx, fsutil.VolumeID(vID.FsSubvolName), volOptions.Size)
	if err != nil {
		log.ErrorLog(ctx, "failed to expand volume %s with error: %v", vID.FsSubvolName, err)

		return err
	}

	return nil
}

func (vo *VolumeOptions) getCloneState(ctx context.Context, volID fsutil.VolumeID) (cephFSCloneState, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(
			ctx,
			"could not get FSAdmin, can get clone status for volume %s with ID %s: %v",
			vo.FsName,
			string(volID),
			err)

		return cephFSCloneError, err
	}

	cs, err := fsa.CloneStatus(vo.FsName, vo.SubvolumeGroup, string(volID))
	if err != nil {
		log.ErrorLog(ctx, "could not get clone state for volume %s with ID %s: %v", vo.FsName, string(volID), err)

		return cephFSCloneError, err
	}

	return cephFSCloneState(cs.State), nil
}
