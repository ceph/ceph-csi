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
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/pkg/util"

	"k8s.io/klog"
)

const (
	csiSubvolumeGroup = "csi"
)

var (
	// cephfsInit is used to create "csi" subvolume group for the first time the csi plugin loads.
	// Subvolume group create gets called every time the plugin loads, though it doesn't result in error
	// its unnecessary
	cephfsInit = false
)

func getCephRootVolumePathLocalDeprecated(volID volumeID) string {
	return path.Join(getCephRootPathLocalDeprecated(volID), "csi-volumes", string(volID))
}

func getVolumeRootPathCephDeprecated(volID volumeID) string {
	return path.Join("/", "csi-volumes", string(volID))
}

func getCephRootPathLocalDeprecated(volID volumeID) string {
	return fmt.Sprintf("%s/controller/volumes/root-%s", PluginFolder, string(volID))
}

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
		"--keyfile="+cr.KeyFile)

	if err != nil {
		klog.Errorf("failed to get the rootpath for the vol %s(%s)", string(volID), err)
		return "", err
	}
	return strings.TrimSuffix(string(stdout), "\n"), nil
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
			"--keyfile="+cr.KeyFile)
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
		"--keyfile="+cr.KeyFile)
	if err != nil {
		klog.Errorf("failed to create subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
		return err
	}

	return nil
}

func mountCephRoot(volID volumeID, volOptions *volumeOptions, adminCr *util.Credentials) error {
	cephRoot := getCephRootPathLocalDeprecated(volID)

	// Root path is not set for dynamically provisioned volumes
	// Access to cephfs's / is required
	volOptions.RootPath = "/"

	if err := util.CreateMountPoint(cephRoot); err != nil {
		return err
	}

	m, err := newMounter(volOptions)
	if err != nil {
		return fmt.Errorf("failed to create mounter: %v", err)
	}

	if err = m.mount(cephRoot, adminCr, volOptions); err != nil {
		return fmt.Errorf("error mounting ceph root: %v", err)
	}

	return nil
}

func unmountCephRoot(volID volumeID) {
	cephRoot := getCephRootPathLocalDeprecated(volID)

	if err := unmountVolume(cephRoot); err != nil {
		klog.Errorf("failed to unmount %s with error %s", cephRoot, err)
	} else {
		if err := os.Remove(cephRoot); err != nil {
			klog.Errorf("failed to remove %s with error %s", cephRoot, err)
		}
	}
}

func purgeVolumeDeprecated(volID volumeID, adminCr *util.Credentials, volOptions *volumeOptions) error {
	if err := mountCephRoot(volID, volOptions, adminCr); err != nil {
		return err
	}
	defer unmountCephRoot(volID)

	var (
		volRoot         = getCephRootVolumePathLocalDeprecated(volID)
		volRootDeleting = volRoot + "-deleting"
	)

	if pathExists(volRoot) {
		if err := os.Rename(volRoot, volRootDeleting); err != nil {
			return fmt.Errorf("couldn't mark volume %s for deletion: %v", volID, err)
		}
	} else {
		if !pathExists(volRootDeleting) {
			klog.V(4).Infof("cephfs: volume %s not found, assuming it to be already deleted", volID)
			return nil
		}
	}

	if err := os.RemoveAll(volRootDeleting); err != nil {
		return fmt.Errorf("failed to delete volume %s: %v", volID, err)
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
		"--keyfile="+cr.KeyFile)
	if err != nil {
		klog.Errorf("failed to purge subvolume %s(%s) in fs %s", string(volID), err, volOptions.FsName)
		return err
	}

	return nil
}
