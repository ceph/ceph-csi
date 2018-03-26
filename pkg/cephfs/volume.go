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

func execCommandAndValidate(program string, args ...string) error {
	out, err := execCommand(program, args...)
	if err != nil {
		return fmt.Errorf("cephfs: %s failed with following error: %s\ncephfs: %s output: %s", program, err, program, out)
	}

	return nil
}

func (m *fuseMounter) mount(mountPoint string, volOptions *volumeOptions) error {
	return execCommandAndValidate("ceph-fuse", mountPoint, "-n", "client."+volOptions.User, "-r", volOptions.RootPath)
}

type kernelMounter struct{}

func (m *kernelMounter) mount(mountPoint string, volOptions *volumeOptions) error {
	if err := execCommandAndValidate("modprobe", "ceph"); err != nil {
		return err
	}

	return execCommandAndValidate("mount",
		"-t", "ceph",
		fmt.Sprintf("%s:%s", volOptions.Monitors, volOptions.RootPath),
		mountPoint,
		"-o",
		fmt.Sprintf("name=%s,secretfile=%s", volOptions.User, getCephSecretPath(volOptions.User)),
	)
}

func unmountVolume(mountPoint string) error {
	return execCommandAndValidate("umount", mountPoint)
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}
