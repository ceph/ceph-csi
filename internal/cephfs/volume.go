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
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"

	fsAdmin "github.com/ceph/go-ceph/cephfs/admin"
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

	// modeAllRWX can be used for setting permissions to Read-Write-eXecute
	// for User, Group and Other.
	modeAllRWX = 0777
)

// Subvolume holds subvolume information. This includes only the needed members
// from fsAdmin.SubVolumeInfo.
type Subvolume struct {
	BytesQuota int64
	Path       string
	Features   []string
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
		util.ErrorLog(ctx, "failed to get the rootpath for the vol %s: %s (stdError: %s)", string(volID), err, stderr)
		if strings.Contains(stderr, volumeNotFound) {
			return "", util.JoinErrors(ErrVolumeNotFound, err)
		}

		return "", err
	}
	return strings.TrimSuffix(stdout, "\n"), nil
}

func (vo *volumeOptions) getSubVolumeInfo(ctx context.Context, volID volumeID) (*Subvolume, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin, can not fetch metadata pool for %s:", vo.FsName, err)
		return nil, err
	}

	info, err := fsa.SubVolumeInfo(vo.FsName, vo.SubvolumeGroup, string(volID))
	if err != nil {
		util.ErrorLog(ctx, "failed to get subvolume info for the vol %s: %s", string(volID), err)
		if strings.HasPrefix(err.Error(), volumeNotFound) {
			return nil, ErrVolumeNotFound
		}
		// Incase the error is other than invalid command return error to the caller.
		if !strings.Contains(err.Error(), invalidCommand) {
			return nil, ErrInvalidCommand
		}

		return nil, err
	}

	bc, ok := info.BytesQuota.(fsAdmin.ByteCount)
	if !ok {
		// info.BytesQuota == Infinite
		return nil, fmt.Errorf("subvolume %s has unsupported quota: %v", string(volID), info.BytesQuota)
	}

	subvol := Subvolume{
		BytesQuota: int64(bc),
		Path:       info.Path,
		Features:   make([]string, len(info.Features)),
	}
	for i, feature := range info.Features {
		subvol.Features[i] = string(feature)
	}

	return &subvol, nil
}

type localClusterState struct {
	// set true if cluster supports resize functionality.
	resizeSupported bool
	// set true once a subvolumegroup is created
	// for corresponding cluster.
	subVolumeGroupCreated bool
}

func createVolume(ctx context.Context, volOptions *volumeOptions, volID volumeID, bytesQuota int64) error {
	// verify if corresponding ClusterID key is present in the map,
	// and if not, initialize with default values(false).
	if _, keyPresent := clusterAdditionalInfo[volOptions.ClusterID]; !keyPresent {
		clusterAdditionalInfo[volOptions.ClusterID] = &localClusterState{}
	}

	ca, err := volOptions.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin, can not create subvolume %s: %s", string(volID), err)
		return err
	}

	// create subvolumegroup if not already created for the cluster.
	if !clusterAdditionalInfo[volOptions.ClusterID].subVolumeGroupCreated {
		opts := fsAdmin.SubVolumeGroupOptions{}
		err = ca.CreateSubVolumeGroup(volOptions.FsName, volOptions.SubvolumeGroup, &opts)
		if err != nil {
			util.ErrorLog(ctx, "failed to create subvolume group %s, for the vol %s: %s", volOptions.SubvolumeGroup, string(volID), err)
			return err
		}
		util.DebugLog(ctx, "cephfs: created subvolume group %s", volOptions.SubvolumeGroup)
		clusterAdditionalInfo[volOptions.ClusterID].subVolumeGroupCreated = true
	}

	opts := fsAdmin.SubVolumeOptions{
		Size: fsAdmin.ByteCount(bytesQuota),
		Mode: modeAllRWX,
	}
	if volOptions.Pool != "" {
		opts.PoolLayout = volOptions.Pool
	}

	// FIXME: check if the right credentials are used ("-n", cephEntityClientPrefix + cr.ID)
	err = ca.CreateSubVolume(volOptions.FsName, volOptions.SubvolumeGroup, string(volID), &opts)
	if err != nil {
		util.ErrorLog(ctx, "failed to create subvolume %s in fs %s: %s", string(volID), volOptions.FsName, err)
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
			util.ErrorLog(ctx, "failed to resize subvolume %s in fs %s: %s", string(volID), volOptions.FsName, err)
			return err
		}
	}
	clusterAdditionalInfo[volOptions.ClusterID].resizeSupported = false
	return createVolume(ctx, volOptions, volID, bytesQuota)
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
		util.ErrorLog(ctx, "failed to purge subvolume %s in fs %s: %s", string(volID), volOptions.FsName, err)
		if strings.Contains(err.Error(), volumeNotEmpty) {
			return util.JoinErrors(ErrVolumeHasSnapshots, err)
		}
		if strings.Contains(err.Error(), volumeNotFound) {
			return util.JoinErrors(ErrVolumeNotFound, err)
		}
		return err
	}

	return nil
}

// checkSubvolumeHasFeature verifies if the referred subvolume has
// the required feature.
func checkSubvolumeHasFeature(feature string, subVolFeatures []string) bool {
	// The subvolume "features" are based on the internal version of the subvolume.
	// Verify if subvolume supports the required feature.
	for _, subvolFeature := range subVolFeatures {
		if subvolFeature == feature {
			return true
		}
	}
	return false
}
