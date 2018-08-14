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
	cephRootPrefix  = PluginFolder + "/controller/volumes/root-"
	cephVolumesRoot = "csi-volumes"

	namespacePrefix = "ns-"
)

func getCephRootPath_local(volId volumeID) string {
	return cephRootPrefix + string(volId)
}

func getCephRootVolumePath_local(volId volumeID) string {
	return path.Join(getCephRootPath_local(volId), cephVolumesRoot, string(volId))
}

func getVolumeRootPath_ceph(volId volumeID) string {
	return path.Join("/", cephVolumesRoot, string(volId))
}

func getVolumeNamespace(volId volumeID) string {
	return namespacePrefix + string(volId)
}

func setVolumeAttribute(root, attrName, attrValue string) error {
	return execCommandAndValidate("setfattr", "-n", attrName, "-v", attrValue, root)
}

func createVolume(volOptions *volumeOptions, adminCr *credentials, volId volumeID, bytesQuota int64) error {
	cephRoot := getCephRootPath_local(volId)

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

	volOptions.RootPath = getVolumeRootPath_ceph(volId)
	localVolRoot := getCephRootVolumePath_local(volId)

	if err := createMountPoint(localVolRoot); err != nil {
		return err
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.quota.max_bytes", fmt.Sprintf("%d", bytesQuota)); err != nil {
		return err
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool", volOptions.Pool); err != nil {
		return fmt.Errorf("%v\ncephfs: Does pool '%s' exist?", err, volOptions.Pool)
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool_namespace", getVolumeNamespace(volId)); err != nil {
		return err
	}

	return nil
}

func purgeVolume(volId volumeID, adminCr *credentials, volOptions *volumeOptions) error {
	var (
		cephRoot        = getCephRootPath_local(volId)
		volRoot         = getCephRootVolumePath_local(volId)
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
