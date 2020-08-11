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

	klog "k8s.io/klog/v2"
)

const (
	// cephFSCloneFailed indicates that clone is in failed state.
	cephFSCloneFailed = "failed"
	// cephFSCloneCompleted indicates that clone is in in-progress state.
	cephFSCloneInprogress = "in-progress"
	// cephFSCloneComplete indicates that clone is in complete state.
	cephFSCloneComplete = "complete"
	// snapshotIsProtected string indicates that the snapshot is currently protected.
	snapshotIsProtected = "yes"
)

func createCloneFromSubvolume(ctx context.Context, volID, cloneID volumeID, volOpt, parentvolOpt *volumeOptions, cr *util.Credentials) error {
	snapshotID := cloneID
	err := createSnapshot(ctx, parentvolOpt, cr, snapshotID, volID)
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
			err = deleteSnapshot(ctx, parentvolOpt, cr, snapshotID, volID)
			if err != nil {
				util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}

		if cloneErr != nil {
			if err = purgeVolume(ctx, cloneID, cr, volOpt, true); err != nil {
				util.ErrorLog(ctx, "failed to delete volume %s: %v", cloneID, err)
			}
			if err = unprotectSnapshot(ctx, parentvolOpt, cr, snapshotID, volID); err != nil {
				// Incase the snap is already unprotected we get ErrSnapProtectionExist error code
				// in that case we are safe and we could discard this error and we are good to go
				// ahead with deletion
				if !errors.Is(err, ErrSnapProtectionExist) {
					util.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)
				}
			}
			if err = deleteSnapshot(ctx, parentvolOpt, cr, snapshotID, volID); err != nil {
				util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}
	}()
	protectErr = protectSnapshot(ctx, parentvolOpt, cr, snapshotID, volID)
	if protectErr != nil {
		util.ErrorLog(ctx, "failed to protect snapshot %s %v", snapshotID, protectErr)
		return protectErr
	}

	cloneErr = cloneSnapshot(ctx, parentvolOpt, cr, volID, snapshotID, cloneID, volOpt)
	if cloneErr != nil {
		util.ErrorLog(ctx, "failed to clone snapshot %s %s to %s %v", volID, snapshotID, cloneID, cloneErr)
		return cloneErr
	}
	var clone CloneStatus
	clone, cloneErr = getCloneInfo(ctx, volOpt, cr, cloneID)
	if cloneErr != nil {
		return cloneErr
	}

	switch clone.Status.State {
	case cephFSCloneInprogress:
		util.ErrorLog(ctx, "clone is in progress for %v", cloneID)
		return ErrCloneInProgress
	case cephFSCloneFailed:
		util.ErrorLog(ctx, "clone failed for %v", cloneID)
		cloneFailedErr := fmt.Errorf("clone %s is in %s state", cloneID, clone.Status.State)
		return cloneFailedErr
	case cephFSCloneComplete:
		// This is a work around to fix sizing issue for cloned images
		err = resizeVolume(ctx, volOpt, cr, cloneID, volOpt.Size)
		if err != nil {
			util.ErrorLog(ctx, "failed to expand volume %s: %v", cloneID, err)
			return err
		}
		// As we completed clone, remove the intermediate snap
		if err = unprotectSnapshot(ctx, parentvolOpt, cr, snapshotID, volID); err != nil {
			// Incase the snap is already unprotected we get ErrSnapProtectionExist error code
			// in that case we are safe and we could discard this error and we are good to go
			// ahead with deletion
			if !errors.Is(err, ErrSnapProtectionExist) {
				util.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)
				return err
			}
		}
		if err = deleteSnapshot(ctx, parentvolOpt, cr, snapshotID, volID); err != nil {
			util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			return err
		}
	}
	return nil
}

func cleanupCloneFromSubvolumeSnapshot(ctx context.Context, volID, cloneID volumeID, parentVolOpt *volumeOptions, cr *util.Credentials) error {
	// snapshot name is same as clone name as we need a name which can be
	// identified during PVC-PVC cloning.
	snapShotID := cloneID
	snapInfo, err := getSnapshotInfo(ctx, parentVolOpt, cr, snapShotID, volID)
	if err != nil {
		if errors.Is(err, ErrSnapNotFound) {
			return nil
		}
		return err
	}

	if snapInfo.Protected == snapshotIsProtected {
		err = unprotectSnapshot(ctx, parentVolOpt, cr, snapShotID, volID)
		if err != nil {
			util.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapShotID, err)
			return err
		}
	}
	err = deleteSnapshot(ctx, parentVolOpt, cr, snapShotID, volID)
	if err != nil {
		util.ErrorLog(ctx, "failed to delete snapshot %s %v", snapShotID, err)
		return err
	}
	return nil
}

func createCloneFromSnapshot(ctx context.Context, parentVolOpt, volOptions *volumeOptions, vID *volumeIdentifier, sID *snapshotIdentifier, cr *util.Credentials) error {
	snapID := volumeID(sID.FsSnapshotName)
	err := cloneSnapshot(ctx, parentVolOpt, cr, volumeID(sID.FsSubvolName), snapID, volumeID(vID.FsSubvolName), volOptions)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if !errors.Is(err, ErrCloneInProgress) {
				if dErr := purgeVolume(ctx, volumeID(vID.FsSubvolName), cr, volOptions, true); dErr != nil {
					util.ErrorLog(ctx, "failed to delete volume %s: %v", vID.FsSubvolName, dErr)
				}
			}
		}
	}()
	var clone = CloneStatus{}
	clone, err = getCloneInfo(ctx, volOptions, cr, volumeID(vID.FsSubvolName))
	if err != nil {
		return err
	}
	switch clone.Status.State {
	case cephFSCloneInprogress:
		return ErrCloneInProgress
	case cephFSCloneFailed:
		return fmt.Errorf("clone %s is in %s state", vID.FsSubvolName, clone.Status.State)
	case cephFSCloneComplete:
		// The clonedvolume currently does not reflect the proper size due to an issue in cephfs
		// however this is getting addressed in cephfs and the parentvolume size will be reflected
		// in the new cloned volume too. Till then we are explicitly making the size set
		err = resizeVolume(ctx, volOptions, cr, volumeID(vID.FsSubvolName), volOptions.Size)
		if err != nil {
			util.ErrorLog(ctx, "failed to expand volume %s with error: %v", vID.FsSubvolName, err)
			return err
		}
	}
	return nil
}

type CloneStatus struct {
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

func getCloneInfo(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, volID volumeID) (CloneStatus, error) {
	clone := CloneStatus{}
	args := []string{
		"fs",
		"clone",
		"status",
		volOptions.FsName,
		string(volID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
		"--format=json",
	}
	err := execCommandJSON(
		ctx,
		&clone,
		"ceph",
		args[:]...)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get subvolume clone info %s(%s) in fs %s"), string(volID), err, volOptions.FsName)
		return clone, err
	}
	return clone, nil
}
