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
)

const (
	volumeMounter_fuse   = "fuse"
	volumeMounter_kernel = "kernel"
)

type volumeMounter interface {
	mount(mountPoint string, volOptions *volumeOptions) error
}

type fuseMounter struct{}

func (m *fuseMounter) mount(mountPoint string, volOptions *volumeOptions) error {
	out, err := execCommand("ceph-fuse", mountPoint, "-n", "client."+volOptions.User, "-r", volOptions.RootPath)
	if err != nil {
		return fmt.Errorf("cephfs: ceph-fuse failed with following error: %s\ncephfs: cephf-fuse output: %s", err, out)
	}

	return nil
}

type kernelMounter struct{}

func (m *kernelMounter) mount(mountPoint string, volOptions *volumeOptions) error {
	out, err := execCommand("modprobe", "ceph")
	if err != nil {
		return fmt.Errorf("cephfs: modprobe failed with following error, %s\ncephfs: modprobe output: %s", err, out)
	}

	args := [...]string{
		"-t", "ceph",
		fmt.Sprintf("%s:%s", volOptions.Monitors, volOptions.RootPath),
		mountPoint,
		"-o",
		fmt.Sprintf("name=%s,secretfile=%s", volOptions.User, getCephSecretPath(volOptions.User)),
	}

	out, err = execCommand("mount", args[:]...)
	if err != nil {
		return fmt.Errorf("cephfs: mount.ceph failed with following error: %s\ncephfs: mount.ceph output: %s", err, out)
	}

	return nil
}

func unmountVolume(mountPoint string) error {
	out, err := execCommand("umount", mountPoint)
	if err != nil {
		return fmt.Errorf("cephfs: umount failed with following error: %v\ncephfs: umount output: %s", err, out)
	}

	return nil
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}
