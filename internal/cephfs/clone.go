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

package cephfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util"
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

	// snapshotIsProtected string indicates that the snapshot is currently protected.
	snapshotIsProtected = "yes"
)

func createCloneFromSubvolume(ctx context.Context, volID, cloneID volumeID, volOpt, parentvolOpt *volumeOptions) error {
	snapshotID := cloneID
	err := parentvolOpt.createSnapshot(ctx, snapshotID, volID)
	if err != nil {
		util.ErrorLog(ctx, "failed to create snapshot %s %v", snapshotID, err)
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
			err = parentvolOpt.deleteSnapshot(ctx, snapshotID, volID)
			if err != nil {
				util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}

		if cloneErr != nil {
			if err = volOpt.purgeVolume(ctx, cloneID, true); err != nil {
				util.ErrorLog(ctx, "failed to delete volume %s: %v", cloneID, err)
			}
			if err = parentvolOpt.unprotectSnapshot(ctx, snapshotID, volID); err != nil {
				// In case the snap is already unprotected we get ErrSnapProtectionExist error code
				// in that case we are safe and we could discard this error and we are good to go
				// ahead with deletion
				if !errors.Is(err, ErrSnapProtectionExist) {
					util.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)
				}
			}
			if err = parentvolOpt.deleteSnapshot(ctx, snapshotID, volID); err != nil {
				util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}
	}()
	protectErr = parentvolOpt.protectSnapshot(ctx, snapshotID, volID)
	if protectErr != nil {
		util.ErrorLog(ctx, "failed to protect snapshot %s %v", snapshotID, protectErr)
		return protectErr
	}

	cloneErr = parentvolOpt.cloneSnapshot(ctx, volID, snapshotID, cloneID, volOpt)
	if cloneErr != nil {
		util.ErrorLog(ctx, "failed to clone snapshot %s %s to %s %v", volID, snapshotID, cloneID, cloneErr)
		return cloneErr
	}

	cloneState, cloneErr := volOpt.getCloneState(ctx, cloneID)
	if cloneErr != nil {
		return cloneErr
	}

	switch cloneState {
	case cephFSCloneInprogress:
		util.ErrorLog(ctx, "clone is in progress for %v", cloneID)
		return ErrCloneInProgress
	case cephFSClonePending:
		util.ErrorLog(ctx, "clone is pending for %v", cloneID)
		return ErrClonePending
	case cephFSCloneFailed:
		util.ErrorLog(ctx, "clone failed for %v", cloneID)
		cloneFailedErr := fmt.Errorf("clone %s is in %s state", cloneID, cloneState)
		return cloneFailedErr
	case cephFSCloneComplete:
		// This is a work around to fix sizing issue for cloned images
		err = volOpt.resizeVolume(ctx, cloneID, volOpt.Size)
		if err != nil {
			util.ErrorLog(ctx, "failed to expand volume %s: %v", cloneID, err)
			return err
		}
		// As we completed clone, remove the intermediate snap
		if err = parentvolOpt.unprotectSnapshot(ctx, snapshotID, volID); err != nil {
			// In case the snap is already unprotected we get ErrSnapProtectionExist error code
			// in that case we are safe and we could discard this error and we are good to go
			// ahead with deletion
			if !errors.Is(err, ErrSnapProtectionExist) {
				util.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)
				return err
			}
		}
		if err = parentvolOpt.deleteSnapshot(ctx, snapshotID, volID); err != nil {
			util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			return err
		}
	}
	return nil
}

func cleanupCloneFromSubvolumeSnapshot(ctx context.Context, volID, cloneID volumeID, parentVolOpt *volumeOptions) error {
	// snapshot name is same as clone name as we need a name which can be
	// identified during PVC-PVC cloning.
	snapShotID := cloneID
	snapInfo, err := parentVolOpt.getSnapshotInfo(ctx, snapShotID, volID)
	if err != nil {
		if errors.Is(err, ErrSnapNotFound) {
			return nil
		}
		return err
	}

	if snapInfo.Protected == snapshotIsProtected {
		err = parentVolOpt.unprotectSnapshot(ctx, snapShotID, volID)
		if err != nil {
			util.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapShotID, err)
			return err
		}
	}
	err = parentVolOpt.deleteSnapshot(ctx, snapShotID, volID)
	if err != nil {
		util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapShotID, err)
		return err
	}
	return nil
}

// isCloneRetryError returns true if the clone error is pending,in-progress
// error.
func isCloneRetryError(err error) bool {
	return errors.Is(err, ErrCloneInProgress) || errors.Is(err, ErrClonePending)
}

func createCloneFromSnapshot(ctx context.Context, parentVolOpt, volOptions *volumeOptions, vID *volumeIdentifier, sID *snapshotIdentifier) error {
	snapID := volumeID(sID.FsSnapshotName)
	err := parentVolOpt.cloneSnapshot(ctx, volumeID(sID.FsSubvolName), snapID, volumeID(vID.FsSubvolName), volOptions)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if !isCloneRetryError(err) {
				if dErr := volOptions.purgeVolume(ctx, volumeID(vID.FsSubvolName), true); dErr != nil {
					util.ErrorLog(ctx, "failed to delete volume %s: %v", vID.FsSubvolName, dErr)
				}
			}
		}
	}()

	cloneState, err := volOptions.getCloneState(ctx, volumeID(vID.FsSubvolName))
	if err != nil {
		return err
	}

	switch cloneState {
	case cephFSCloneInprogress:
		return ErrCloneInProgress
	case cephFSClonePending:
		return ErrClonePending
	case cephFSCloneFailed:
		return fmt.Errorf("clone %s is in %s state", vID.FsSubvolName, cloneState)
	case cephFSCloneComplete:
		// The clonedvolume currently does not reflect the proper size due to an issue in cephfs
		// however this is getting addressed in cephfs and the parentvolume size will be reflected
		// in the new cloned volume too. Till then we are explicitly making the size set
		err = volOptions.resizeVolume(ctx, volumeID(vID.FsSubvolName), volOptions.Size)
		if err != nil {
			util.ErrorLog(ctx, "failed to expand volume %s with error: %v", vID.FsSubvolName, err)
			return err
		}
	}
	return nil
}

func (vo *volumeOptions) getCloneState(ctx context.Context, volID volumeID) (cephFSCloneState, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin, can get clone status for volume %s with ID %s: %v", vo.FsName, string(volID), err)
		return cephFSCloneError, err
	}

	cs, err := fsa.CloneStatus(vo.FsName, vo.SubvolumeGroup, string(volID))
	if err != nil {
		util.ErrorLog(ctx, "could not get clone state for volume %s with ID %s: %v", vo.FsName, string(volID), err)
		return cephFSCloneError, err
	}

	return cephFSCloneState(cs.State), nil
}
