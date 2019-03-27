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
	cephVolumesRoot = "csi-volumes"

	namespacePrefix = "ns-"
)

func getCephRootPathLocal(volID volumeID) string {
	return fmt.Sprintf("%s/controller/volumes/root-%s", PluginFolder, string(volID))
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
	return execCommandErr("setfattr", "-n", attrName, "-v", attrValue, root)
}

func createVolume(volOptions *volumeOptions, adminCr *credentials, volID volumeID, bytesQuota int64) error {
	if err := mountCephRoot(volID, volOptions, adminCr); err != nil {
		return err
	}
	defer unmountCephRoot(volID)

	var (
		volRoot         = getCephRootVolumePathLocal(volID)
		volRootCreating = volRoot + "-creating"
	)

	if pathExists(volRoot) {
		klog.V(4).Infof("cephfs: volume %s already exists, skipping creation", volID)
		return nil
	}

	if err := createMountPoint(volRootCreating); err != nil {
		return err
	}

	if bytesQuota > 0 {
		if err := setVolumeAttribute(volRootCreating, "ceph.quota.max_bytes", fmt.Sprintf("%d", bytesQuota)); err != nil {
			return err
		}
	}

	if err := setVolumeAttribute(volRootCreating, "ceph.dir.layout.pool", volOptions.Pool); err != nil {
		return fmt.Errorf("%v\ncephfs: Does pool '%s' exist?", err, volOptions.Pool)
	}

	if err := setVolumeAttribute(volRootCreating, "ceph.dir.layout.pool_namespace", getVolumeNamespace(volID)); err != nil {
		return err
	}

	if err := os.Rename(volRootCreating, volRoot); err != nil {
		return fmt.Errorf("couldn't mark volume %s as created: %v", volID, err)
	}

	return nil
}

func purgeVolume(volID volumeID, adminCr *credentials, volOptions *volumeOptions) error {
	if err := mountCephRoot(volID, volOptions, adminCr); err != nil {
		return err
	}
	defer unmountCephRoot(volID)

	var (
		volRoot         = getCephRootVolumePathLocal(volID)
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

func mountCephRoot(volID volumeID, volOptions *volumeOptions, adminCr *credentials) error {
	cephRoot := getCephRootPathLocal(volID)

	// Root path is not set for dynamically provisioned volumes
	// Access to cephfs's / is required
	volOptions.RootPath = "/"

	if err := createMountPoint(cephRoot); err != nil {
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
	cephRoot := getCephRootPathLocal(volID)

	if err := unmountVolume(cephRoot); err != nil {
		klog.Errorf("failed to unmount %s with error %s", cephRoot, err)
	} else {
		if err := os.Remove(cephRoot); err != nil {
			klog.Errorf("failed to remove %s with error %s", cephRoot, err)
		}
	}
}
