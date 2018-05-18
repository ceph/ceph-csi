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
	"bytes"
	"fmt"
	"os"
)

const (
	volumeMounter_fuse   = "fuse"
	volumeMounter_kernel = "kernel"
)

type volumeMounter interface {
	mount(mountPoint string, cr *credentials, volOptions *volumeOptions, volUuid string, readOnly bool) error
}

type fuseMounter struct{}

func mountFuse(mountPoint string, cr *credentials, volOptions *volumeOptions, volUuid string) error {
	args := [...]string{
		mountPoint,
		"-c", getCephConfPath(volUuid),
		"-n", cephEntityClientPrefix + cr.id,
		"--keyring", getCephKeyringPath(cr.id),
		"-r", volOptions.RootPath,
	}

	out, err := execCommand("ceph-fuse", args[:]...)
	if err != nil {
		return fmt.Errorf("cephfs: ceph-fuse failed with following error: %s\ncephfs: ceph-fuse output: %s", err, out)
	}

	if !bytes.Contains(out, []byte("starting fuse")) {
		return fmt.Errorf("cephfs: ceph-fuse failed:\ncephfs: ceph-fuse output: %s", out)
	}

	return nil
}

func (m *fuseMounter) mount(mountPoint string, cr *credentials, volOptions *volumeOptions, volUuid string, readOnly bool) error {
	if err := createMountPoint(mountPoint); err != nil {
		return err
	}

	localVolRoot := getVolumeRootPath_local(volUuid)

	if err := createMountPoint(localVolRoot); err != nil {
		return err
	}

	if err := mountFuse(localVolRoot, cr, volOptions, volUuid); err != nil {
		return err
	}

	return bindVolume(volUuid, mountPoint, readOnly)
}

type kernelMounter struct{}

func mountKernel(mountPoint string, cr *credentials, volOptions *volumeOptions) error {
	if err := execCommandAndValidate("modprobe", "ceph"); err != nil {
		return err
	}

	return execCommandAndValidate("mount",
		"-t", "ceph",
		fmt.Sprintf("%s:%s", volOptions.Monitors, volOptions.RootPath),
		mountPoint,
		"-o",
		fmt.Sprintf("name=%s,secretfile=%s", cr.id, getCephSecretPath(cr.id)),
	)
}

func (m *kernelMounter) mount(mountPoint string, cr *credentials, volOptions *volumeOptions, volUuid string, readOnly bool) error {
	if err := createMountPoint(mountPoint); err != nil {
		return err
	}

	localVolRoot := getVolumeRootPath_local(volUuid)

	if err := createMountPoint(localVolRoot); err != nil {
		return err
	}

	if err := mountKernel(localVolRoot, cr, volOptions); err != nil {
		return err
	}

	return bindVolume(volUuid, mountPoint, readOnly)
}

func bindMount(from, to string, readOnly bool) error {
	if err := execCommandAndValidate("mount", "--bind", from, to); err != nil {
		return fmt.Errorf("failed bind-mount of %s to %s: %v", from, to, err)
	}

	if readOnly {
		if err := execCommandAndValidate("mount", "-o", "remount,ro,bind", to); err != nil {
			return err
		}
	}

	return nil
}

func bindVolume(volUuid, target string, readOnly bool) error {
	volDataRoot := getVolumeDataPath_local(volUuid)

	if err := createMountPoint(volDataRoot); err != nil {
		return err
	}

	return bindMount(volDataRoot, target, readOnly)
}

func unmountVolume(mountPoint string) error {
	return execCommandAndValidate("umount", mountPoint)
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}
