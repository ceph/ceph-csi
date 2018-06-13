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

// Volumes are mounted in .../controller/volumes/vol-{UUID}
// The actual user data resides in .../vol-{UUID}/volume-data
// purgeVolume moves the user data to .../vol-{UUID}/volume-deleting and only then calls os.RemoveAll

const (
	cephRootPrefix   = PluginFolder + "/controller/volumes/root-"
	cephVolumePrefix = PluginFolder + "/controller/volumes/vol-"
	cephVolumesRoot  = "csi-volumes"

	namespacePrefix = "csi-ns-"
)

func getCephRootPath_local(volUuid string) string {
	return cephRootPrefix + volUuid
}

func getCephRootVolumePath_local(volUuid string) string {
	return path.Join(getCephRootPath_local(volUuid), cephVolumesRoot, volUuid)
}

func getVolumeRootPath_local(volUuid string) string {
	return cephVolumePrefix + volUuid
}

func getVolumeRootPath_ceph(volUuid string) string {
	return path.Join("/", cephVolumesRoot, volUuid)
}

func getVolumeNamespace(volUuid string) string {
	return namespacePrefix + volUuid
}

func setVolumeAttribute(root, attrName, attrValue string) error {
	return execCommandAndValidate("setfattr", "-n", attrName, "-v", attrValue, root)
}

func createVolume(volOptions *volumeOptions, adminCr *credentials, volUuid string, bytesQuota int64) error {
	cephRoot := getCephRootPath_local(volUuid)

	if err := createMountPoint(cephRoot); err != nil {
		return err
	}

	// RootPath is not set for a dynamically provisioned volume
	// Access to cephfs's / is required
	volOptions.RootPath = "/"

	if err := mountKernel(cephRoot, adminCr, volOptions, volUuid); err != nil {
		return fmt.Errorf("error mounting ceph root: %v", err)
	}

	defer func() {
		unmountVolume(cephRoot)
		os.Remove(cephRoot)
	}()

	volOptions.RootPath = getVolumeRootPath_ceph(volUuid)
	localVolRoot := getCephRootVolumePath_local(volUuid)

	if err := createMountPoint(localVolRoot); err != nil {
		return err
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.quota.max_bytes", fmt.Sprintf("%d", bytesQuota)); err != nil {
		return err
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool", volOptions.Pool); err != nil {
		return fmt.Errorf("%v\ncephfs: Does pool '%s' exist?", err, volOptions.Pool)
	}

	if err := setVolumeAttribute(localVolRoot, "ceph.dir.layout.pool_namespace", getVolumeNamespace(volUuid)); err != nil {
		return err
	}

	return nil
}

func purgeVolume(volId string, cr *credentials, volOptions *volumeOptions) error {
	// Root path is not set for dynamically provisioned volumes
	volOptions.RootPath = "/"

	var (
		volUuid         = uuidFromVolumeId(volId)
		root            = getCephRootPath_local(volUuid)
		volRoot         = getCephRootVolumePath_local(volUuid)
		volRootDeleting = volRoot + "-deleting"
	)

	if err := createMountPoint(root); err != nil {
		return err
	}

	if err := mountKernel(root, cr, volOptions, volUuid); err != nil {
		return err
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
