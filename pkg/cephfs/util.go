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
	"encoding/json"
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

func execCommandAndValidate(program string, args ...string) error {
	out, err := execCommand(program, args...)
	if err != nil {
		return fmt.Errorf("cephfs: %s failed with following error: %s\ncephfs: %s output: %s", program, err, program, out)
	}

	return nil
}

func execCommandJson(v interface{}, program string, args ...string) error {
	cmd := exec.Command(program, args...)
	out, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("cephfs: %s failed with following error: %s\ncephfs: %s output: %s", program, err, program, out)
	}

	return json.NewDecoder(bytes.NewReader(out)).Decode(v)
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

func storeCephUserCredentials(volUuid string, cr *credentials, volOptions *volumeOptions) error {
	keyringData := cephKeyringData{
		UserId:     cr.id,
		Key:        cr.key,
		RootPath:   volOptions.RootPath,
		VolumeUuid: volUuid,
	}

	if volOptions.ProvisionVolume {
		keyringData.Pool = volOptions.Pool
		keyringData.Namespace = getVolumeNamespace(volUuid)
	}

	return storeCephCredentials(volUuid, cr, &keyringData)
}

func storeCephAdminCredentials(volUuid string, cr *credentials) error {
	return storeCephCredentials(volUuid, cr, &cephFullCapsKeyringData{UserId: cr.id, Key: cr.key, VolumeUuid: volUuid})
}

func storeCephCredentials(volUuid string, cr *credentials, keyringData cephConfigWriter) error {
	if err := keyringData.writeToFile(); err != nil {
		return err
	}

	secret := cephSecretData{
		UserId:     cr.id,
		Key:        cr.key,
		VolumeUuid: volUuid,
	}

	if err := secret.writeToFile(); err != nil {
		return err
	}

	return nil
}

func newMounter(volOptions *volumeOptions) volumeMounter {
	mounter := volOptions.Mounter

	if mounter == "" {
		mounter = DefaultVolumeMounter
	}

	switch mounter {
	case volumeMounter_fuse:
		return &fuseMounter{}
	case volumeMounter_kernel:
		return &kernelMounter{}
	}

	return nil
}
