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
)

const (
	cephVolumesRoot = "csi-volumes"

	namespacePrefix = "ns-"
)

var (
	cephRootPrefix = PluginFolder + "/controller/volumes/root-"
)

func getCephRootPathLocal(volId volumeID) string {
	return cephRootPrefix + string(volId)
}

func getCephRootVolumePathLocal(volId volumeID) string {
	return path.Join(getCephRootPathLocal(volId), cephVolumesRoot, string(volId))
}

func getVolumeRootPathCeph(volId volumeID) string {
	return path.Join("/", cephVolumesRoot, string(volId))
}

func getVolumeNamespace(volId volumeID) string {
	return namespacePrefix + string(volId)
}

func setVolumeAttribute(root, attrName, attrValue string) error {
	return execCommandAndValidate("setfattr", "-n", attrName, "-v", attrValue, root)
}

func createVolume(volOptions *volumeOptions, adminCr *credentials, volId volumeID, bytesQuota int64) error {
	cephRoot := getCephRootPathLocal(volId)

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

	if err = m.mount(cephRoot, adminCr, volOptions, volId); err != nil {
		return fmt.Errorf("error mounting ceph root: %v", err)
	}

	defer func() {
		unmountVolume(cephRoot)
		os.Remove(cephRoot)
	}()

	volOptions.RootPath = getVolumeRootPathCeph(volId)
	localVolRoot := getCephRootVolumePathLocal(volId)

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

	err = setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool_namespace", getVolumeNamespace(volId))

	return err
}

func purgeVolume(volId volumeID, adminCr *credentials, volOptions *volumeOptions) error {
	var (
		cephRoot        = getCephRootPathLocal(volId)
		volRoot         = getCephRootVolumePathLocal(volId)
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

	if err = m.mount(cephRoot, adminCr, volOptions, volId); err != nil {
		return fmt.Errorf("error mounting ceph root: %v", err)
	}

	defer func() {
		unmountVolume(volRoot)
		os.Remove(volRoot)
	}()

	if err := os.Rename(volRoot, volRootDeleting); err != nil {
		return fmt.Errorf("coudln't mark volume %s for deletion: %v", volId, err)
	}

	if err := os.RemoveAll(volRootDeleting); err != nil {
		return fmt.Errorf("failed to delete volume %s: %v", volId, err)
	}

	return nil
}
