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

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}

func deleteVolumePath(volPath string) error {
	return os.RemoveAll(volPath)
}

func mountFuse(root string) error {
	out, err := execCommand("ceph-fuse", root)
	if err != nil {
		return fmt.Errorf("cephfs: ceph-fuse failed with following error: %v\ncephfs: ceph-fuse output: %s", err, out)
	}

	return nil
}

func unmountFuse(root string) error {
	out, err := execCommand("fusermount", "-u", root)
	if err != nil {
		return fmt.Errorf("cephfs: fusermount failed with following error: %v\ncephfs: fusermount output: %s", err, out)
	}

	return nil
}

func setVolAttributes(volPath string /*opts *fsVolumeOptions*/, maxBytes int64) error {
	out, err := execCommand("setfattr", "-n", "ceph.quota.max_bytes",
		"-v", fmt.Sprintf("%d", maxBytes), volPath)
	if err != nil {
		return fmt.Errorf("cephfs: setfattr failed with following error: %v\ncephfs: setfattr output: %s", err, out)
	}

	return nil
}
