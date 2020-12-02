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
	"strings"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/ceph/go-ceph/cephfs/admin"
	"github.com/ceph/go-ceph/rados"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// autoProtect points to the snapshot auto-protect feature of
// the subvolume.
const (
	autoProtect = "snapshot-autoprotect"
)

// cephfsSnapshot represents a CSI snapshot and its cluster information.
type cephfsSnapshot struct {
	NamePrefix string
	Monitors   string
	// MetadataPool & Pool fields are not used atm. But its definitely good to have it in this struct
	// so keeping it here
	MetadataPool string
	Pool         string
	ClusterID    string
	RequestName  string
	// ReservedID represents the ID reserved for a snapshot
	ReservedID string
}

func (vo *volumeOptions) createSnapshot(ctx context.Context, snapID, volID volumeID) error {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin: %s", err)
		return err
	}

	err = fsa.CreateSubVolumeSnapshot(vo.FsName, vo.SubvolumeGroup, string(volID), string(snapID))
	if err != nil {
		util.ErrorLog(ctx, "failed to create subvolume snapshot %s %s in fs %s: %s",
			string(snapID), string(volID), vo.FsName, err)
		return err
	}
	return nil
}

func (vo *volumeOptions) deleteSnapshot(ctx context.Context, snapID, volID volumeID) error {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin: %s", err)
		return err
	}

	err = fsa.ForceRemoveSubVolumeSnapshot(vo.FsName, vo.SubvolumeGroup, string(volID), string(snapID))
	if err != nil {
		util.ErrorLog(ctx, "failed to delete subvolume snapshot %s %s in fs %s: %s",
			string(snapID), string(volID), vo.FsName, err)
		return err
	}
	return nil
}

type snapshotInfo struct {
	CreatedAt        string `json:"created_at"`
	CreationTime     *timestamp.Timestamp
	DataPool         string `json:"data_pool"`
	HasPendingClones string `json:"has_pending_clones"`
	Protected        string `json:"protected"`
	Size             int    `json:"size"`
}

func getSnapshotInfo(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, snapID, volID volumeID) (snapshotInfo, error) {
	snap := snapshotInfo{}
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"info",
		volOptions.FsName,
		string(volID),
		string(snapID),
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
		&snap,
		"ceph",
		args[:]...)
	if err != nil {
		if strings.Contains(err.Error(), snapNotFound) {
			return snapshotInfo{}, ErrSnapNotFound
		}
		util.ErrorLog(ctx, "failed to get subvolume snapshot info %s %s(%s) in fs %s", string(snapID), string(volID), err, volOptions.FsName)
		return snapshotInfo{}, err
	}
	return snap, nil
}

func (vo *volumeOptions) protectSnapshot(ctx context.Context, snapID, volID volumeID) error {
	// If "snapshot-autoprotect" feature is present, The ProtectSnapshot
	// call should be treated as a no-op.
	if checkSubvolumeHasFeature(autoProtect, vo.Features) {
		return nil
	}
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin: %s", err)
		return err
	}

	err = fsa.ProtectSubVolumeSnapshot(vo.FsName, vo.SubvolumeGroup, string(volID),
		string(snapID))
	if err != nil {
		if errors.Is(err, rados.ErrObjectExists) {
			return nil
		}
		util.ErrorLog(ctx, "failed to protect subvolume snapshot %s %s in fs %s with error: %s", string(volID), string(snapID), vo.FsName, err)
		return err
	}
	return nil
}

func (vo *volumeOptions) unprotectSnapshot(ctx context.Context, snapID, volID volumeID) error {
	// If "snapshot-autoprotect" feature is present, The UnprotectSnapshot
	// call should be treated as a no-op.
	if checkSubvolumeHasFeature(autoProtect, vo.Features) {
		return nil
	}
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin: %s", err)
		return err
	}

	err = fsa.UnprotectSubVolumeSnapshot(vo.FsName, vo.SubvolumeGroup, string(volID),
		string(snapID))
	if err != nil {
		// In case the snap is already unprotected we get ErrSnapProtectionExist error code
		// in that case we are safe and we could discard this error.
		if errors.Is(err, rados.ErrObjectExists) {
			return nil
		}
		util.ErrorLog(ctx, "failed to unprotect subvolume snapshot %s %s in fs %s with error: %s", string(volID), string(snapID), vo.FsName, err)
		return err
	}
	return nil
}

func (vo *volumeOptions) cloneSnapshot(ctx context.Context, volID, snapID, cloneID volumeID, cloneVolOptions *volumeOptions) error {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin: %s", err)
		return err
	}
	co := &admin.CloneOptions{
		TargetGroup: cloneVolOptions.SubvolumeGroup,
	}
	if cloneVolOptions.Pool != "" {
		co.PoolLayout = cloneVolOptions.Pool
	}

	err = fsa.CloneSubVolumeSnapshot(vo.FsName, vo.SubvolumeGroup, string(volID), string(snapID), string(cloneID), co)
	if err != nil {
		util.ErrorLog(ctx, "failed to clone subvolume snapshot %s %s in fs %s with error: %s", string(volID), string(snapID), string(cloneID), vo.FsName, err)
		if errors.Is(err, rados.ErrNotFound) {
			return ErrVolumeNotFound
		}
		return err
	}
	return nil
}
