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
	"os"

	"github.com/ceph/ceph-csi/pkg/util"

	"k8s.io/klog"
)

const (
	cephUserPrefix         = "user-"
	cephEntityClientPrefix = "client."
)

type cephEntityCaps struct {
	Mds string `json:"mds"`
	Mon string `json:"mon"`
	Osd string `json:"osd"`
}

type cephEntity struct {
	Entity string         `json:"entity"`
	Key    string         `json:"key"`
	Caps   cephEntityCaps `json:"caps"`
}

func (ent *cephEntity) toCredentials() *credentials {
	return &credentials{
		id:  ent.Entity[len(cephEntityClientPrefix):],
		key: ent.Key,
	}
}

func getCephUserName(volID volumeID) string {
	return cephUserPrefix + string(volID)
}

func getCephUser(adminCr *credentials, volID volumeID) (*cephEntity, error) {
	entityName := cephEntityClientPrefix + getCephUserName(volID)

	var ents []cephEntity
	args := [...]string{
		"auth", "-f", "json", "-c", getCephConfPath(volID), "-n", cephEntityClientPrefix + adminCr.id,
		"get", entityName,
	}

	out, err := util.ExecCommand("ceph", args[:]...)
	if err != nil {
		return nil, fmt.Errorf("cephfs: ceph failed with following error: %s\ncephfs: ceph output: %s", err, out)
	}

	// Workaround for output from `ceph auth get`
	// Contains non-json data: "exported keyring for ENTITY\n\n"
	offset := bytes.Index(out, []byte("[{"))

	if err = json.NewDecoder(bytes.NewReader(out[offset:])).Decode(&ents); err != nil {
		return nil, fmt.Errorf("failed to decode json: %v", err)
	}

	if len(ents) != 1 {
		return nil, fmt.Errorf("got unexpected number of entities for %s: expected 1, got %d", entityName, len(ents))
	}

	return &ents[0], nil
}

func createCephUser(volOptions *volumeOptions, adminCr *credentials, volID volumeID) (*cephEntity, error) {
	caps := cephEntityCaps{
		Mds: fmt.Sprintf("allow rw path=%s", getVolumeRootPathCeph(volID)),
		Mon: "allow r",
		Osd: fmt.Sprintf("allow rw pool=%s namespace=%s", volOptions.Pool, getVolumeNamespace(volID)),
	}

	var ents []cephEntity
	args := [...]string{
		"auth", "-f", "json", "-c", getCephConfPath(volID), "-n", cephEntityClientPrefix + adminCr.id,
		"get-or-create", cephEntityClientPrefix + getCephUserName(volID),
		"mds", caps.Mds,
		"mon", caps.Mon,
		"osd", caps.Osd,
	}

	if err := execCommandJSON(&ents, args[:]...); err != nil {
		return nil, fmt.Errorf("error creating ceph user: %v", err)
	}

	return &ents[0], nil
}

func deleteCephUser(adminCr *credentials, volID volumeID) error {
	userID := getCephUserName(volID)

	args := [...]string{
		"-c", getCephConfPath(volID), "-n", cephEntityClientPrefix + adminCr.id,
		"auth", "rm", cephEntityClientPrefix + userID,
	}

	var err error
	if err = execCommandAndValidate("ceph", args[:]...); err != nil {
		return err
	}

	keyringPath := getCephKeyringPath(volID, userID)
	if err = os.Remove(keyringPath); err != nil {
		klog.Errorf("failed to remove keyring file %s with error %s", keyringPath, err)
	}

	secretPath := getCephSecretPath(volID, userID)
	if err = os.Remove(secretPath); err != nil {
		klog.Errorf("failed to remove secret file %s with error %s", secretPath, err)
	}

	return nil
}
