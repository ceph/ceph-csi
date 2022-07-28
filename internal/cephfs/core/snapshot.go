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
	"time"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/ceph/go-ceph/cephfs/admin"
	"github.com/ceph/go-ceph/rados"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// autoProtect points to the snapshot auto-protect feature of
// the subvolume.
const (
	autoProtect = "snapshot-autoprotect"
)

// SnapshotClient is the interface that holds the signature of snapshot methods
// that interacts with CephFS snapshot API's.
type SnapshotClient interface {
	// CreateSnapshot creates a snapshot of the subvolume.
	CreateSnapshot(ctx context.Context) error
	// DeleteSnapshot deletes the snapshot of the subvolume.
	DeleteSnapshot(ctx context.Context) error
	// GetSnapshotInfo returns the snapshot info of the subvolume.
	GetSnapshotInfo(ctx context.Context) (SnapshotInfo, error)
	// ProtectSnapshot protects the snapshot of the subvolume.
	ProtectSnapshot(ctx context.Context) error
	// UnprotectSnapshot unprotects the snapshot of the subvolume.
	UnprotectSnapshot(ctx context.Context) error
	// CloneSnapshot clones the snapshot of the subvolume.
	CloneSnapshot(ctx context.Context, cloneVolOptions *SubVolume) error
	// SetAllSnapshotMetadata set all the metadata from arg parameters on
	// subvolume snapshot.
	SetAllSnapshotMetadata(parameters map[string]string) error
	// UnsetAllSnapshotMetadata unset all the metadata from arg keys on
	// subvolume snapshot.
	UnsetAllSnapshotMetadata(keys []string) error
}

// snapshotClient is the implementation of SnapshotClient interface.
type snapshotClient struct {
	*Snapshot                              // Embedded snapshot struct.
	clusterID      string                  // Cluster ID.
	clusterName    string                  // Cluster Name.
	enableMetadata bool                    // Set metadata on volume
	conn           *util.ClusterConnection // Cluster connection.
}

// Snapshot represents a subvolume snapshot and its cluster information.
type Snapshot struct {
	SnapshotID string // subvolume snapshot id.
	*SubVolume        // parent subvolume information.
}

// NewSnapshot creates a new snapshot client.
func NewSnapshot(
	conn *util.ClusterConnection,
	snapshotID,
	clusterID,
	clusterName string,
	setMetadata bool,
	vol *SubVolume,
) SnapshotClient {
	return &snapshotClient{
		Snapshot: &Snapshot{
			SnapshotID: snapshotID,
			SubVolume:  vol,
		},
		clusterID:      clusterID,
		clusterName:    clusterName,
		enableMetadata: setMetadata,
		conn:           conn,
	}
}

// CreateSnapshot creates a snapshot of the subvolume.
func (s *snapshotClient) CreateSnapshot(ctx context.Context) error {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin: %s", err)

		return err
	}

	err = fsa.CreateSubVolumeSnapshot(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID)
	if err != nil {
		log.ErrorLog(ctx, "failed to create subvolume snapshot %s %s in fs %s: %s",
			s.SnapshotID, s.VolID, s.FsName, err)

		return err
	}

	return nil
}

// DeleteSnapshot deletes the snapshot of the subvolume.
func (s *snapshotClient) DeleteSnapshot(ctx context.Context) error {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin: %s", err)

		return err
	}

	err = fsa.ForceRemoveSubVolumeSnapshot(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID)
	if err != nil {
		log.ErrorLog(ctx, "failed to delete subvolume snapshot %s %s in fs %s: %s",
			s.SnapshotID, s.VolID, s.FsName, err)

		return err
	}

	return nil
}

type SnapshotInfo struct {
	CreatedAt        time.Time
	CreationTime     *timestamp.Timestamp
	HasPendingClones string
	Protected        string
}

// GetSnapshotInfo returns the snapshot info of the subvolume.
func (s *snapshotClient) GetSnapshotInfo(ctx context.Context) (SnapshotInfo, error) {
	snap := SnapshotInfo{}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin: %s", err)

		return snap, err
	}

	info, err := fsa.SubVolumeSnapshotInfo(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID)
	if err != nil {
		if errors.Is(err, rados.ErrNotFound) {
			return snap, cerrors.ErrSnapNotFound
		}
		log.ErrorLog(
			ctx,
			"failed to get subvolume snapshot info %s %s in fs %s with error %s",
			s.VolID,
			s.SnapshotID,
			s.FsName,
			err)

		return snap, err
	}
	snap.CreatedAt = info.CreatedAt.Time
	snap.HasPendingClones = info.HasPendingClones
	snap.Protected = info.Protected

	return snap, nil
}

// ProtectSnapshot protects the snapshot of the subvolume.
func (s *snapshotClient) ProtectSnapshot(ctx context.Context) error {
	// If "snapshot-autoprotect" feature is present, The ProtectSnapshot
	// call should be treated as a no-op.
	if checkSubvolumeHasFeature(autoProtect, s.Features) {
		return nil
	}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin: %s", err)

		return err
	}

	err = fsa.ProtectSubVolumeSnapshot(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID)
	if err != nil {
		if errors.Is(err, rados.ErrObjectExists) {
			return nil
		}
		log.ErrorLog(
			ctx,
			"failed to protect subvolume snapshot %s %s in fs %s with error: %s",
			s.VolID,
			s.SnapshotID,
			s.FsName,
			err)

		return err
	}

	return nil
}

// UnprotectSnapshot unprotects the snapshot of the subvolume.
func (s *snapshotClient) UnprotectSnapshot(ctx context.Context) error {
	// If "snapshot-autoprotect" feature is present, The UnprotectSnapshot
	// call should be treated as a no-op.
	if checkSubvolumeHasFeature(autoProtect, s.Features) {
		return nil
	}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin: %s", err)

		return err
	}

	err = fsa.UnprotectSubVolumeSnapshot(s.FsName, s.SubvolumeGroup, s.VolID,
		s.SnapshotID)
	if err != nil {
		// In case the snap is already unprotected we get ErrSnapProtectionExist error code
		// in that case we are safe and we could discard this error.
		if errors.Is(err, rados.ErrObjectExists) {
			return nil
		}
		log.ErrorLog(
			ctx,
			"failed to unprotect subvolume snapshot %s %s in fs %s with error: %s",
			s.VolID,
			s.SnapshotID,
			s.FsName,
			err)

		return err
	}

	return nil
}

// CloneSnapshot clones the snapshot of the subvolume.
func (s *snapshotClient) CloneSnapshot(
	ctx context.Context,
	cloneSubVol *SubVolume,
) error {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin: %s", err)

		return err
	}
	co := &admin.CloneOptions{
		TargetGroup: cloneSubVol.SubvolumeGroup,
	}
	if cloneSubVol.Pool != "" {
		co.PoolLayout = cloneSubVol.Pool
	}

	err = fsa.CloneSubVolumeSnapshot(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID, cloneSubVol.VolID, co)
	if err != nil {
		log.ErrorLog(
			ctx,
			"failed to clone subvolume snapshot %s %s in fs %s with error: %s",
			s.VolID,
			s.SnapshotID,
			cloneSubVol.VolID,
			s.FsName,
			err)
		if errors.Is(err, rados.ErrNotFound) {
			return cerrors.ErrVolumeNotFound
		}

		return err
	}

	return nil
}
