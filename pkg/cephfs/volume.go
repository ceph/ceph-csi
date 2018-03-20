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

type volume struct {
	RootPath string
	User     string
}

func (vol *volume) mount(mountPoint string) error {
	out, err := execCommand("ceph-fuse", mountPoint, "-n", "client."+vol.User, "-r", vol.RootPath)
	if err != nil {
		return fmt.Errorf("cephfs: ceph-fuse failed with following error: %s\ncephfs: cephf-fuse output: %s", err, out)
	}

	return nil
}

func (vol *volume) unmount() error {
	out, err := execCommand("fusermount", "-u", vol.RootPath)
	if err != nil {
		return fmt.Errorf("cephfs: fusermount failed with following error: %v\ncephfs: fusermount output: %s", err, out)
	}

	return nil
}

func unmountVolume(root string) error {
	out, err := execCommand("fusermount", "-u", root)
	if err != nil {
		return fmt.Errorf("cephfs: fusermount failed with following error: %v\ncephfs: fusermount output: %s", err, out)
	}

	return nil
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}
