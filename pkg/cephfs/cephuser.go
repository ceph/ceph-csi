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

func getCephUserName(volId volumeID) string {
	return cephUserPrefix + string(volId)
}

func getCephUser(userId string) (*cephEntity, error) {
	entityName := cephEntityClientPrefix + userId
	var ents []cephEntity

	if err := execCommandJson(&ents, "ceph", "auth", "get", entityName); err != nil {
		return nil, err
	}

	if len(ents) != 1 {
		return nil, fmt.Errorf("error retrieving entity %s", entityName)
	}

	return &ents[0], nil
}

func createCephUser(volOptions *volumeOptions, cr *credentials, volId volumeID) (*cephEntity, error) {
	caps := cephEntityCaps{
		Mds: fmt.Sprintf("allow rw path=%s", getVolumeRootPath_ceph(volId)),
		Mon: "allow r",
		Osd: fmt.Sprintf("allow rw pool=%s namespace=%s", volOptions.Pool, getVolumeNamespace(volId)),
	}

	var ents []cephEntity
	args := [...]string{
		"auth", "-f", "json", "-c", getCephConfPath(volId), "-n", cephEntityClientPrefix + cr.id,
		"get-or-create", cephEntityClientPrefix + getCephUserName(volId),
		"mds", caps.Mds,
		"mon", caps.Mon,
		"osd", caps.Osd,
	}

	if err := execCommandJson(&ents, "ceph", args[:]...); err != nil {
		return nil, fmt.Errorf("error creating ceph user: %v", err)
	}

	return &ents[0], nil
}

func deleteCephUser(adminCr *credentials, volId volumeID) error {
	userId := getCephUserName(volId)

	args := [...]string{
		"-c", getCephConfPath(volId), "-n", cephEntityClientPrefix + adminCr.id,
		"auth", "rm", cephEntityClientPrefix + userId,
	}

	if err := execCommandAndValidate("ceph", args[:]...); err != nil {
		return err
	}

	os.Remove(getCephKeyringPath(volId, userId))
	os.Remove(getCephSecretPath(volId, userId))

	return nil
}
