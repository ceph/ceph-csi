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
	"fmt"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/ceph/go-ceph/cephfs/admin"
)

// cephFSCloneState describes the status of the clone.
type cephFSCloneState struct {
	state    admin.CloneState
	errno    string
	errorMsg string
}

const (
	// SnapshotIsProtected string indicates that the snapshot is currently protected.
	SnapshotIsProtected = "yes"
)

// CephFSCloneError indicates that fetching the clone state returned an error.
var CephFSCloneError = cephFSCloneState{}

// ToError checks the state of the clone if it's not cephFSCloneComplete.
func (cs cephFSCloneState) ToError() error {
	switch cs.state {
	case admin.CloneComplete:
		return nil
	case CephFSCloneError.state:
		return fmt.Errorf("%w: %s (%s)", cerrors.ErrInvalidClone, cs.errorMsg, cs.errno)
	case admin.CloneInProgress:
		return cerrors.ErrCloneInProgress
	case admin.ClonePending:
		return cerrors.ErrClonePending
	case admin.CloneFailed:
		return fmt.Errorf("%w: %s (%s)", cerrors.ErrCloneFailed, cs.errorMsg, cs.errno)
	}

	return nil
}

// CreateCloneFromSubvolume creates a clone from a subvolume.
func (s *subVolumeClient) CreateCloneFromSubvolume(
	ctx context.Context,
	parentvolOpt *SubVolume,
) error {
	snapshotID := s.VolID
	snapClient := NewSnapshot(s.conn, snapshotID, s.clusterID, s.clusterName, s.enableMetadata, parentvolOpt)
	err := snapClient.CreateSnapshot(ctx)
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
			err = snapClient.DeleteSnapshot(ctx)
			if err != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}

		if cloneErr != nil {
			if err = s.PurgeVolume(ctx, true); err != nil {
				log.ErrorLog(ctx, "failed to delete volume %s: %v", s.VolID, err)
			}
			if err = snapClient.UnprotectSnapshot(ctx); err != nil {
				// In case the snap is already unprotected we get ErrSnapProtectionExist error code
				// in that case we are safe and we could discard this error and we are good to go
				// ahead with deletion
				if !errors.Is(err, cerrors.ErrSnapProtectionExist) {
					log.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)
				}
			}
			if err = snapClient.DeleteSnapshot(ctx); err != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)
			}
		}
	}()
	protectErr = snapClient.ProtectSnapshot(ctx)
	if protectErr != nil {
		log.ErrorLog(ctx, "failed to protect snapshot %s %v", snapshotID, protectErr)

		return protectErr
	}
	cloneErr = snapClient.CloneSnapshot(ctx, s.SubVolume)
	if cloneErr != nil {
		log.ErrorLog(ctx, "failed to clone snapshot %s %s to %s %v", parentvolOpt.VolID, snapshotID, s.VolID, cloneErr)

		return cloneErr
	}

	cloneState, cloneErr := s.GetCloneState(ctx)
	if cloneErr != nil {
		log.ErrorLog(ctx, "failed to get clone state: %v", cloneErr)

		return cloneErr
	}

	err = cloneState.ToError()
	if err != nil {
		log.ErrorLog(ctx, "clone %s did not complete: %v", s.VolID, err)

		return err
	}

	err = s.ExpandVolume(ctx, s.Size)
	if err != nil {
		log.ErrorLog(ctx, "failed to expand volume %s: %v", s.VolID, err)

		return err
	}

	// As we completed clone, remove the intermediate snap
	if err = snapClient.UnprotectSnapshot(ctx); err != nil {
		// In case the snap is already unprotected we get ErrSnapProtectionExist error code
		// in that case we are safe and we could discard this error and we are good to go
		// ahead with deletion
		if !errors.Is(err, cerrors.ErrSnapProtectionExist) {
			log.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapshotID, err)

			return err
		}
	}
	if err = snapClient.DeleteSnapshot(ctx); err != nil {
		log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapshotID, err)

		return err
	}

	return nil
}

// CleanupSnapshotFromSubvolume	removes the snapshot from the subvolume.
func (s *subVolumeClient) CleanupSnapshotFromSubvolume(
	ctx context.Context, parentVol *SubVolume,
) error {
	// snapshot name is same as clone name as we need a name which can be
	// identified during PVC-PVC cloning.
	snapShotID := s.VolID
	snapClient := NewSnapshot(s.conn, snapShotID, s.clusterID, s.clusterName, s.enableMetadata, parentVol)
	snapInfo, err := snapClient.GetSnapshotInfo(ctx)
	if err != nil {
		if errors.Is(err, cerrors.ErrSnapNotFound) {
			return nil
		}

		return err
	}

	if snapInfo.Protected == SnapshotIsProtected {
		err = snapClient.UnprotectSnapshot(ctx)
		if err != nil {
			log.ErrorLog(ctx, "failed to unprotect snapshot %s %v", snapShotID, err)

			return err
		}
	}
	err = snapClient.DeleteSnapshot(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to delete snapshot %s %v", snapShotID, err)

		return err
	}

	return nil
}

// CreateSnapshotFromSubvolume creates a clone from subvolume snapshot.
func (s *subVolumeClient) CreateCloneFromSnapshot(
	ctx context.Context, snap Snapshot,
) error {
	snapID := snap.SnapshotID
	snapClient := NewSnapshot(s.conn, snapID, s.clusterID, s.clusterName, s.enableMetadata, snap.SubVolume)
	err := snapClient.CloneSnapshot(ctx, s.SubVolume)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if !cerrors.IsCloneRetryError(err) {
				if dErr := s.PurgeVolume(ctx, true); dErr != nil {
					log.ErrorLog(ctx, "failed to delete volume %s: %v", s.VolID, dErr)
				}
			}
		}
	}()

	cloneState, err := s.GetCloneState(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to get clone state: %v", err)

		return err
	}

	err = cloneState.ToError()
	if err != nil {
		return err
	}

	err = s.ExpandVolume(ctx, s.Size)
	if err != nil {
		log.ErrorLog(ctx, "failed to expand volume %s with error: %v", s.VolID, err)

		return err
	}

	return nil
}

// GetCloneState returns the clone state of the subvolume.
func (s *subVolumeClient) GetCloneState(ctx context.Context) (cephFSCloneState, error) {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(
			ctx,
			"could not get FSAdmin, can get clone status for volume %s with ID %s: %v",
			s.FsName,
			s.VolID,
			err)

		return CephFSCloneError, err
	}

	cs, err := fsa.CloneStatus(s.FsName, s.SubvolumeGroup, s.VolID)
	if err != nil {
		log.ErrorLog(ctx, "could not get clone state for volume %s with ID %s: %v", s.FsName, s.VolID, err)

		return CephFSCloneError, err
	}

	errno := ""
	errStr := ""
	if failure := cs.GetFailure(); failure != nil {
		errno = failure.Errno
		errStr = failure.ErrStr
	}

	state := cephFSCloneState{
		state:    cs.State,
		errno:    errno,
		errorMsg: errStr,
	}

	return state, nil
}
