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
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/pkg/util"

	"k8s.io/klog"
)

const (
	namespacePrefix   = "fsvolumens_"
	csiSubvolumeGroup = "csi"
)

var (
	// cephfsInit is used to create "csi" subvolume group for the first time the csi plugin loads.
	// Subvolume group create gets called every time the plugin loads, though it doesn't result in error
	// its unnecessary
	cephfsInit = false
)

func getVolumeRootPathCeph(volOptions *volumeOptions, cr *util.Credentials, volID volumeID) (string, error) {
	stdout, _, err := util.ExecCommand(
		"ceph",
		"fs",
		"subvolume",
		"getpath",
		volOptions.FsName,
		string(volID),
		"--group_name",
		csiSubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix+cr.ID,
		"--key="+cr.Key)

	if err != nil {
		klog.Errorf("failed to get the rootpath for the vol %s(%s)", string(volID), err)
		return "", err
	}
	return strings.TrimSuffix(string(stdout), "\n"), nil
}

func getVolumeNamespace(volID volumeID) string {
	return namespacePrefix + string(volID)
}

func createVolume(volOptions *volumeOptions, cr *util.Credentials, volID volumeID, bytesQuota int64) error {
	//TODO: When we support multiple fs, need to hande subvolume group create for all fs's
	if !cephfsInit {
		err := execCommandErr(
			"ceph",
			"fs",
			"subvolumegroup",
			"create",
			volOptions.FsName,
			csiSubvolumeGroup,
			"--mode",
			"777",
			"--pool_layout",
			volOptions.Pool,
			"-m", volOptions.Monitors,
			"-c", util.CephConfigPath,
			"-n", cephEntityClientPrefix+cr.ID,
			"--key="+cr.Key)
		if err != nil {
			klog.Errorf("failed to create subvolume group csi, for the vol %s(%s)", string(volID), err)
			return err
		}
		klog.V(4).Infof("cephfs: created subvolume group csi")
		cephfsInit = true
	}
	err := execCommandErr(
		"ceph",
		"fs",
		"subvolume",
		"create",
		volOptions.FsName,
		string(volID),
		strconv.FormatInt(bytesQuota, 10),
		"--group_name",
		csiSubvolumeGroup,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix+cr.ID,
		"--key="+cr.Key)
	if err != nil {
		klog.Errorf("failed to create subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
		return err
	}

	return nil
}

func purgeVolume(volID volumeID, cr *util.Credentials, volOptions *volumeOptions) error {
	err := execCommandErr(
		"ceph",
		"fs",
		"subvolume",
		"rm",
		volOptions.FsName,
		string(volID),
		"--group_name",
		csiSubvolumeGroup,
		"--force",
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix+cr.ID,
		"--key="+cr.Key)
	if err != nil {
		klog.Errorf("failed to purge subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
		return err
	}

	return nil
}
