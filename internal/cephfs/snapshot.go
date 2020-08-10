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
	"strings"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/golang/protobuf/ptypes/timestamp"
	klog "k8s.io/klog/v2"
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
}

func createSnapshot(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, snapID, volID volumeID) error {
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"create",
		volOptions.FsName,
		string(volID),
		string(snapID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
	}

	err := execCommandErr(
		ctx,
		"ceph",
		args[:]...)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create subvolume snapshot %s %s(%s) in fs %s"), string(snapID), string(volID), err, volOptions.FsName)
		return err
	}
	return nil
}

func deleteSnapshot(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, snapID, volID volumeID) error {
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"rm",
		volOptions.FsName,
		string(volID),
		string(snapID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
		"--force",
	}

	err := execCommandErr(
		ctx,
		"ceph",
		args[:]...)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete subvolume snapshot %s %s(%s) in fs %s"), string(snapID), string(volID), err, volOptions.FsName)
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
		if strings.Contains(err.Error(), ErrSnapNotFound.Error()) {
			return snapshotInfo{}, err
		}
		klog.Errorf(util.Log(ctx, "failed to get subvolume snapshot info %s %s(%s) in fs %s"), string(snapID), string(volID), err, volOptions.FsName)
		return snapshotInfo{}, err
	}
	return snap, nil
}

func protectSnapshot(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, snapID, volID volumeID) error {
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"protect",
		volOptions.FsName,
		string(volID),
		string(snapID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
	}

	err := execCommandErr(
		ctx,
		"ceph",
		args[:]...)
	if err != nil {
		if strings.Contains(err.Error(), ErrSnapProtectionExist.Error()) {
			return nil
		}
		klog.Errorf(util.Log(ctx, "failed to protect subvolume snapshot %s %s(%s) in fs %s"), string(snapID), string(volID), err, volOptions.FsName)
		return err
	}
	return nil
}

func unprotectSnapshot(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, snapID, volID volumeID) error {
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"unprotect",
		volOptions.FsName,
		string(volID),
		string(snapID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
	}

	err := execCommandErr(
		ctx,
		"ceph",
		args[:]...)
	if err != nil {
		// Incase the snap is already unprotected we get ErrSnapProtectionExist error code
		// in that case we are safe and we could discard this error.
		if strings.Contains(err.Error(), ErrSnapProtectionExist.Error()) {
			return nil
		}
		klog.Errorf(util.Log(ctx, "failed to unprotect subvolume snapshot %s %s(%s) in fs %s"), string(snapID), string(volID), err, volOptions.FsName)
		return err
	}
	return nil
}

func cloneSnapshot(ctx context.Context, parentVolOptions *volumeOptions, cr *util.Credentials, volID, snapID, cloneID volumeID, cloneVolOptions *volumeOptions) error {
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"clone",
		parentVolOptions.FsName,
		string(volID),
		string(snapID),
		string(cloneID),
		"--group_name",
		parentVolOptions.SubvolumeGroup,
		"--target_group_name",
		cloneVolOptions.SubvolumeGroup,
		"-m", parentVolOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
	}
	if cloneVolOptions.Pool != "" {
		args = append(args, "--pool_layout", cloneVolOptions.Pool)
	}

	err := execCommandErr(
		ctx,
		"ceph",
		args[:]...)

	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to clone subvolume snapshot %s %s(%s) in fs %s"), string(cloneID), string(volID), err, parentVolOptions.FsName)
		if strings.HasPrefix(err.Error(), ErrVolumeNotFound.Error()) {
			return ErrVolumeNotFound
		}
		return err
	}
	return nil
}

// Snapshots holds the snapshot name.
type Snapshots struct {
	Name string `json:"name"`
}

func listSnapshots(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, volID volumeID) ([]Snapshots, error) {
	snaps := []Snapshots{}
	args := []string{
		"fs",
		"subvolume",
		"snapshot",
		"ls",
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
		&snaps,
		"ceph",
		args[:]...)
	if err != nil {
		if strings.Contains(err.Error(), ErrSnapNotFound.Error()) {
			return []Snapshots{}, err
		}
		klog.Errorf(util.Log(ctx, "failed to list subvolume snapshots %s(%s) in fs %s"), string(volID), err, volOptions.FsName)
		return []Snapshots{}, err
	}
	return snaps, nil
}
