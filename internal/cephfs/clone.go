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
)

func createCloneFromSubvolume(ctx context.Context, volID, cloneID volumeID, volOpt, parentvolOpt *volumeOptions, cr *util.Credentials) error {
	snapshotID := cloneID
	err := createSnapshot(ctx, parentvolOpt, cr, snapshotID, volID)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create snapshot %s %v"), snapshotID, err)
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
				klog.Errorf(util.Log(ctx, "failed to delete snapshot %s %v"), snapshotID, err)
			}
		}

		if cloneErr != nil {
			if err = purgeVolume(ctx, cloneID, cr, volOpt, true); err != nil {
				klog.Errorf(util.Log(ctx, "failed to delete volume %s: %v"), cloneID, err)
			}
			if err = unprotectSnapshot(ctx, parentvolOpt, cr, snapshotID, volID); err != nil {
				klog.Errorf(util.Log(ctx, "failed to unprotect snapshot %s %v"), snapshotID, err)
			}
			if err = deleteSnapshot(ctx, parentvolOpt, cr, snapshotID, volID); err != nil {
				klog.Errorf(util.Log(ctx, "failed to delete snapshot %s %v"), snapshotID, err)
			}
		}
	}()
	protectErr = protectSnapshot(ctx, parentvolOpt, cr, snapshotID, volID)
	if protectErr != nil {
		klog.Errorf(util.Log(ctx, "failed to protect snapshot %s %v"), snapshotID, protectErr)
		return protectErr
	}

	cloneErr = cloneSnapshot(ctx, parentvolOpt, cr, volID, snapshotID, cloneID, volOpt)
	if cloneErr != nil {
		klog.Errorf(util.Log(ctx, "failed to clone snapshot %s %s to %s %v"), volID, snapshotID, cloneID, cloneErr)
		return cloneErr
	}
	var clone CloneStatus
	clone, cloneErr = getcloneInfo(ctx, volOpt, cr, cloneID)
	if cloneErr != nil {
		return cloneErr
	}
	if clone.Status.State == cephFSCloneInprogress {
		return ErrCloneInProgress{err: fmt.Errorf("clone is in progress for %v", cloneID)}
	}
	if clone.Status.State == cephFSCloneFailed {
		cloneErr = fmt.Errorf("clone %s is in %s state", cloneID, clone.Status.State)
	}

	return nil
}

func cleanupCloneFromSubvolumeSnapshot(ctx context.Context, volID, cloneID volumeID, parentVolOpt *volumeOptions, cr *util.Credentials) error {
	// snapshot name is same as clone name as we need a name which can be
	// identified during PVC-PVC cloning.
	snapShotID := cloneID
	_, err := getSnapshotInfo(ctx, parentVolOpt, cr, snapShotID, volID)
	if err != nil {
		var evnf util.ErrSnapNotFound
		if errors.As(err, &evnf) {
			return nil
		}
		return err
	}
	err = unprotectSnapshot(ctx, parentVolOpt, cr, snapShotID, volID)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to unprotect snapshot %s %v"), snapShotID, err)
		return err
	}
	err = deleteSnapshot(ctx, parentVolOpt, cr, snapShotID, volID)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete snapshot %s %v"), snapShotID, err)
		return err
	}
	return nil
}

func createCloneFromSnapshot(ctx context.Context, parentVolOpt, volOptions *volumeOptions, pvID, vID *volumeIdentifier, sID *snapshotIdentifier, cr *util.Credentials) error {
	snapID := volumeID(sID.FsSnapshotName)
	err := cloneSnapshot(ctx, parentVolOpt, cr, volumeID(pvID.FsSubvolName), snapID, volumeID(vID.FsSubvolName), volOptions)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			var ecip ErrCloneInProgress
			if !errors.As(err, &ecip) {
				if dErr := purgeVolume(ctx, volumeID(vID.FsSubvolName), cr, volOptions, true); dErr != nil {
					klog.Errorf(util.Log(ctx, "failed to delete volume %s: %v"), vID.FsSubvolName, dErr)
				}
			}
		}
	}()
	var clone = CloneStatus{}
	clone, err = getcloneInfo(ctx, volOptions, cr, volumeID(vID.FsSubvolName))
	if err != nil {
		return err
	}
	if clone.Status.State == cephFSCloneInprogress {
		return ErrCloneInProgress{err: fmt.Errorf("clone is in progress for %v", vID.FsSubvolName)}
	}
	if clone.Status.State == cephFSCloneFailed {
		return fmt.Errorf("clone %s is in %s state", vID.FsSubvolName, clone.Status.State)
	}
	if clone.Status.State == cephFSCloneComplete {
		// This is a work around to fix sizing issue for cloned images
		err = resizeVolume(ctx, volOptions, cr, volumeID(vID.FsSubvolName), volOptions.Size)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to expand volume %s: %v"), vID.FsSubvolName, err)
			return err
		}
	}
	return nil
}
