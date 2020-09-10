/*
Copyright 2018 The Ceph-CSI Authors.

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
	"path"
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
)

var (
	// clusterAdditionalInfo contains information regarding if resize is
	// supported in the particular cluster and subvolumegroup is
	// created or not.
	// Subvolumegroup creation and volume resize decisions are
	// taken through this additional cluster information.
	clusterAdditionalInfo = make(map[string]*localClusterState)
)

const (
	cephEntityClientPrefix = "client."
)

// Subvolume holds subvolume information.
type Subvolume struct {
	BytesQuota    int      `json:"bytes_quota"`
	DataPool      string   `json:"data_pool"`
	Features      []string `json:"features"`
	GID           int      `json:"gid"`
	Mode          int      `json:"mode"`
	MonAddrs      []string `json:"mon_addrs"`
	Path          string   `json:"path"`
	PoolNamespace string   `json:"pool_namespace"`
	// The subvolume "state" is based on the current state of the subvolume.
	// It contains one of the following values:
	// * "complete": subvolume is ready for all operations.
	// * "snapshot-retained": subvolume is removed but its snapshots are retained.
	State string `json:"state"`
	Type  string `json:"type"`
	UID   int    `json:"uid"`
}

func getVolumeRootPathCephDeprecated(volID volumeID) string {
	return path.Join("/", "csi-volumes", string(volID))
}

func getVolumeRootPathCeph(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, volID volumeID) (string, error) {
	stdout, stderr, err := util.ExecCommand(
		ctx,
		"ceph",
		"fs",
		"subvolume",
		"getpath",
		volOptions.FsName,
		string(volID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix+cr.ID,
		"--keyfile="+cr.KeyFile)

	if err != nil {
		util.ErrorLog(ctx, "failed to get the rootpath for the vol %s(%s) stdError %s", string(volID), err, stderr)
		if strings.Contains(stderr, volumeNotFound) {
			return "", util.JoinErrors(ErrVolumeNotFound, err)
		}

		return "", err
	}
	return strings.TrimSuffix(stdout, "\n"), nil
}

func getSubVolumeInfo(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, volID volumeID) (Subvolume, error) {
	info := Subvolume{}
	err := execCommandJSON(
		ctx,
		&info,
		"ceph",
		"fs",
		"subvolume",
		"info",
		volOptions.FsName,
		string(volID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix+cr.ID,
		"--keyfile="+cr.KeyFile)
	if err != nil {
		util.ErrorLog(ctx, "failed to get subvolume info for the vol %s(%s)", string(volID), err)
		if strings.HasPrefix(err.Error(), volumeNotFound) {
			return info, ErrVolumeNotFound
		}
		// Incase the error is other than invalid command return error to the caller.
		if !strings.Contains(err.Error(), invalidCommand) {
			return info, ErrInvalidCommand
		}

		return info, err
	}
	return info, nil
}

type localClusterState struct {
	// set true if cluster supports resize functionality.
	resizeSupported bool
	// set true once a subvolumegroup is created
	// for corresponding cluster.
	subVolumeGroupCreated bool
}

func createVolume(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, volID volumeID, bytesQuota int64) error {
	// verify if corresponding ClusterID key is present in the map,
	// and if not, initialize with default values(false).
	if _, keyPresent := clusterAdditionalInfo[volOptions.ClusterID]; !keyPresent {
		clusterAdditionalInfo[volOptions.ClusterID] = &localClusterState{}
	}

	// create subvolumegroup if not already created for the cluster.
	if !clusterAdditionalInfo[volOptions.ClusterID].subVolumeGroupCreated {
		err := execCommandErr(
			ctx,
			"ceph",
			"fs",
			"subvolumegroup",
			"create",
			volOptions.FsName,
			volOptions.SubvolumeGroup,
			"-m", volOptions.Monitors,
			"-c", util.CephConfigPath,
			"-n", cephEntityClientPrefix+cr.ID,
			"--keyfile="+cr.KeyFile)
		if err != nil {
			util.ErrorLog(ctx, "failed to create subvolume group %s, for the vol %s(%s)", volOptions.SubvolumeGroup, string(volID), err)
			return err
		}
		util.DebugLog(ctx, "cephfs: created subvolume group %s", volOptions.SubvolumeGroup)
		clusterAdditionalInfo[volOptions.ClusterID].subVolumeGroupCreated = true
	}

	args := []string{
		"fs",
		"subvolume",
		"create",
		volOptions.FsName,
		string(volID),
		strconv.FormatInt(bytesQuota, 10),
		"--group_name",
		volOptions.SubvolumeGroup,
		"--mode", "777",
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
	}

	if volOptions.Pool != "" {
		args = append(args, "--pool_layout", volOptions.Pool)
	}

	err := execCommandErr(
		ctx,
		"ceph",
		args[:]...)
	if err != nil {
		util.ErrorLog(ctx, "failed to create subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
		return err
	}

	return nil
}

// resizeVolume will try to use ceph fs subvolume resize command to resize the
// subvolume. If the command is not available as a fallback it will use
// CreateVolume to resize the subvolume.
func resizeVolume(ctx context.Context, volOptions *volumeOptions, cr *util.Credentials, volID volumeID, bytesQuota int64) error {
	// keyPresent checks whether corresponding clusterID key is present in clusterAdditionalInfo
	var keyPresent bool
	// verify if corresponding ClusterID key is present in the map,
	// and if not, initialize with default values(false).
	if _, keyPresent = clusterAdditionalInfo[volOptions.ClusterID]; !keyPresent {
		clusterAdditionalInfo[volOptions.ClusterID] = &localClusterState{}
	}
	// resize subvolume when either it's supported, or when corresponding
	// clusterID key was not present.
	if clusterAdditionalInfo[volOptions.ClusterID].resizeSupported || !keyPresent {
		args := []string{
			"fs",
			"subvolume",
			"resize",
			volOptions.FsName,
			string(volID),
			strconv.FormatInt(bytesQuota, 10),
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

		if err == nil {
			clusterAdditionalInfo[volOptions.ClusterID].resizeSupported = true
			return nil
		}
		// Incase the error is other than invalid command return error to the caller.
		if !strings.Contains(err.Error(), invalidCommand) {
			util.ErrorLog(ctx, "failed to resize subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
			return err
		}
	}
	clusterAdditionalInfo[volOptions.ClusterID].resizeSupported = false
	return createVolume(ctx, volOptions, cr, volID, bytesQuota)
}

func purgeVolume(ctx context.Context, volID volumeID, cr *util.Credentials, volOptions *volumeOptions, force bool) error {
	arg := []string{
		"fs",
		"subvolume",
		"rm",
		volOptions.FsName,
		string(volID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID,
		"--keyfile=" + cr.KeyFile,
	}
	if force {
		arg = append(arg, "--force")
	}
	if checkSubvolumeHasFeature("snapshot-retention", volOptions.Features) {
		arg = append(arg, "--retain-snapshots")
	}

	err := execCommandErr(ctx, "ceph", arg...)
	if err != nil {
		util.ErrorLog(ctx, "failed to purge subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
		if strings.Contains(err.Error(), volumeNotFound) {
			return util.JoinErrors(ErrVolumeNotFound, err)
		}
		return err
	}

	return nil
}
