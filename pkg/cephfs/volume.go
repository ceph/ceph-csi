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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

const (
	// from https://github.com/kubernetes-incubator/external-storage/tree/master/ceph/cephfs/cephfs_provisioner
	provisionerCmd = "/cephfs_provisioner.py"
	userPrefix     = "user-"
)

type volume struct {
	Root string `json:"path"`
	User string `json:"user"`
	Key  string `json:"key"`
}

func newVolume(volId *volumeIdentifier, volOpts *volumeOptions) (*volume, error) {
	cmd := exec.Command(provisionerCmd, "-n", volId.id, "-u", userPrefix+volId.id)
	cmd.Env = []string{
		"CEPH_CLUSTER_NAME=" + volOpts.ClusterName,
		"CEPH_MON=" + volOpts.Monitor,
		"CEPH_AUTH_ID=" + volOpts.AdminId,
		"CEPH_AUTH_KEY=" + volOpts.AdminSecret,
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cephfs: an error occurred while creating the volume: %v\ncephfs: %s", err, out)
	}

	fmt.Printf("\t\tcephfs_provisioner.py: %s\n", out)

	vol := &volume{}
	if err = json.Unmarshal(out, vol); err != nil {
		return nil, fmt.Errorf("cephfs: malformed json output: %s", err)
	}

	return vol, nil
}

func (vol *volume) mount(mountPoint string) error {
	out, err := execCommand("ceph-fuse", mountPoint, "-n", vol.User, "-r", vol.Root)
	if err != nil {
		return fmt.Errorf("cephfs: ceph-fuse failed with following error: %s\ncephfs: cephf-fuse output: %s", err, out)
	}

	return nil
}

func (vol *volume) unmount() error {
	out, err := execCommand("fusermount", "-u", vol.Root)
	if err != nil {
		return fmt.Errorf("cephfs: fusermount failed with following error: %v\ncephfs: fusermount output: %s", err, out)
	}

	return nil
}

func (vol *volume) makeMap() map[string]string {
	return map[string]string{
		"path": vol.Root,
		"user": vol.User,
	}
}

func unmountVolume(root string) error {
	out, err := execCommand("fusermount", "-u", root)
	if err != nil {
		return fmt.Errorf("cephfs: fusermount failed with following error: %v\ncephfs: fusermount output: %s", err, out)
	}

	return nil
}

func deleteVolume(volId, user string) error {
	out, err := execCommand(provisionerCmd, "--remove", "-n", volId, "-u", user)
	if err != nil {
		return fmt.Errorf("cephfs: failed to delete volume %s following error: %v\ncephfs: output: %s", volId, err, out)
	}

	return nil
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}
