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

	klog "k8s.io/klog/v2"
)

var (
	// clusterAdditionalInfo contains information regarding if resize is
	// supported in the particular cluster and subvolumegroup is
	// created or not.
	// Subvolumegroup creation and volume resize decisions are
	// taken through this additional cluster information.
	clusterAdditionalInfo = make(map[string]*localClusterState)

	inValidCommmand = "no valid command found"

	// ceph returns `Error ENOENT:` when subvolume or subvolume group doesnot
	// exist.
	errNotFoundString = "Error ENOENT:"
)

const (
	cephEntityClientPrefix = "client."
)

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
		klog.Errorf(util.Log(ctx, "failed to get the rootpath for the vol %s(%s) stdError %s"), string(volID), err, stderr)

		if strings.HasPrefix(stderr, errNotFoundString) {
			return "", util.JoinErrors(ErrVolumeNotFound, err)
		}

		return "", err
	}
	return strings.TrimSuffix(stdout, "\n"), nil
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
			klog.Errorf(util.Log(ctx, "failed to create subvolume group %s, for the vol %s(%s)"), volOptions.SubvolumeGroup, string(volID), err)
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
		klog.Errorf(util.Log(ctx, "failed to create subvolume %s(%s) in fs %s"), string(volID), err, volOptions.FsName)
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
		if !strings.Contains(err.Error(), inValidCommmand) {
			klog.Errorf(util.Log(ctx, "failed to resize subvolume %s(%s) in fs %s"), string(volID), err, volOptions.FsName)
			return err
		}
	}
	clusterAdditionalInfo[volOptions.ClusterID].resizeSupported = false
	return createVolume(ctx, volOptions, cr, volID, bytesQuota)
}

func purgeVolume(ctx context.Context, volID volumeID, cr *util.Credentials, volOptions *volumeOptions) error {
	err := execCommandErr(
		ctx,
		"ceph",
		"fs",
		"subvolume",
		"rm",
		volOptions.FsName,
		string(volID),
		"--group_name",
		volOptions.SubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix+cr.ID,
		"--keyfile="+cr.KeyFile)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to purge subvolume %s(%s) in fs %s"), string(volID), err, volOptions.FsName)

		if strings.HasPrefix(err.Error(), errNotFoundString) {
			return util.JoinErrors(ErrVolumeNotFound, err)
		}

		return err
	}

	return nil
}
