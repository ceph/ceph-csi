/*
Copyright 2018 The Kubernetes Authors.

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

	"k8s.io/klog"
)

const (
	cephRootPrefix  = PluginFolder + "/controller/volumes/root-"
	cephVolumesRoot = "csi-volumes"

	namespacePrefix = "ns-"
)

func getCephRootPathLocal(volID volumeID) string {
	return cephRootPrefix + string(volID)
}

func getCephRootVolumePathLocal(volID volumeID) string {
	return path.Join(getCephRootPathLocal(volID), cephVolumesRoot, string(volID))
}

func getVolumeRootPathCeph(volID volumeID) string {
	return path.Join("/", cephVolumesRoot, string(volID))
}

func getVolumeNamespace(volID volumeID) string {
	return namespacePrefix + string(volID)
}

func setVolumeAttribute(root, attrName, attrValue string) error {
	return execCommandAndValidate("setfattr", "-n", attrName, "-v", attrValue, root)
}

func createVolume(volOptions *volumeOptions, adminCr *credentials, volID volumeID, bytesQuota int64) error {
	cephRoot := getCephRootPathLocal(volID)

	if err := createMountPoint(cephRoot); err != nil {
		return err
	}

	// RootPath is not set for a dynamically provisioned volume
	// Access to cephfs's / is required
	volOptions.RootPath = "/"

	m, err := newMounter(volOptions)
	if err != nil {
		return fmt.Errorf("failed to create mounter: %v", err)
	}

	if err = m.mount(cephRoot, adminCr, volOptions, volID); err != nil {
		return fmt.Errorf("error mounting ceph root: %v", err)
	}

	defer unmountAndRemove(cephRoot)

	volOptions.RootPath = getVolumeRootPathCeph(volID)
	localVolRoot := getCephRootVolumePathLocal(volID)

	if err := createMountPoint(localVolRoot); err != nil {
		return err
	}

	if bytesQuota > 0 {
		if err := setVolumeAttribute(localVolRoot, "ceph.quota.max_bytes", fmt.Sprintf("%d", bytesQuota)); err != nil {
			return err
		}
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool", volOptions.Pool); err != nil {
		return fmt.Errorf("%v\ncephfs: Does pool '%s' exist?", err, volOptions.Pool)
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool_namespace", getVolumeNamespace(volID)); err != nil {
		return err
	}

	return nil
}

func purgeVolume(volID volumeID, adminCr *credentials, volOptions *volumeOptions) error {
	var (
		cephRoot        = getCephRootPathLocal(volID)
		volRoot         = getCephRootVolumePathLocal(volID)
		volRootDeleting = volRoot + "-deleting"
	)

	if err := createMountPoint(cephRoot); err != nil {
		return err
	}

	// Root path is not set for dynamically provisioned volumes
	// Access to cephfs's / is required
	volOptions.RootPath = "/"

	m, err := newMounter(volOptions)
	if err != nil {
		return fmt.Errorf("failed to create mounter: %v", err)
	}

	if err = m.mount(cephRoot, adminCr, volOptions, volID); err != nil {
		return fmt.Errorf("error mounting ceph root: %v", err)
	}

	defer unmountAndRemove(cephRoot)

	if err := os.Rename(volRoot, volRootDeleting); err != nil {
		return fmt.Errorf("coudln't mark volume %s for deletion: %v", volID, err)
	}

	if err := os.RemoveAll(volRootDeleting); err != nil {
		return fmt.Errorf("failed to delete volume %s: %v", volID, err)
	}

	return nil
}

func unmountAndRemove(mountPoint string) {
	var err error
	if err = unmountVolume(mountPoint); err != nil {
		klog.Errorf("failed to unmount %s with error %s", mountPoint, err)
	}

	if err = os.Remove(mountPoint); err != nil {
		klog.Errorf("failed to remove %s with error %s", mountPoint, err)
	}
}
