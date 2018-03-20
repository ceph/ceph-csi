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
	"os/exec"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/kubernetes/pkg/util/keymutex"
	"k8s.io/kubernetes/pkg/util/mount"
)

func execCommand(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func isMountPoint(p string) (bool, error) {
	notMnt, err := mount.New("").IsLikelyNotMountPoint(p)
	if err != nil {
		return false, status.Error(codes.Internal, err.Error())
	}

	return !notMnt, nil
}

func tryLock(id string, mtx keymutex.KeyMutex, name string) error {
	// TODO uncomment this once TryLockKey gets into Kubernetes
	/*
		if !mtx.TryLockKey(id) {
			msg := fmt.Sprintf("%s has a pending operation on %s", name, req.GetVolumeId())
			glog.Infoln(msg)

			return status.Error(codes.Aborted, msg)
		}
	*/

	return nil
}

func getKeyFromCredentials(creds map[string]string) (string, error) {
	if key, ok := creds["key"]; ok {
		return key, nil
	} else {
		return "", fmt.Errorf("missing key in credentials")
	}
}
