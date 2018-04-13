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
	cephUserPrefix         = "csi-user-"
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

func getCephUserName(volUuid string) string {
	return cephUserPrefix + volUuid
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

func (e *cephEntity) create() error {
	return execCommandJson(e, "ceph", "auth", "get-or-create", e.Entity, "mds", e.Caps.Mds, "osd", e.Caps.Osd, "mon", e.Caps.Mon)

}

func createCephUser(volOptions *volumeOptions, volUuid string, readOnly bool) (*cephEntity, error) {
	access := "rw"
	if readOnly {
		access = "r"
	}

	caps := cephEntityCaps{
		Mds: fmt.Sprintf("allow %s path=%s", access, getVolumeRootPath_ceph(volUuid)),
		Mon: "allow r",
		Osd: fmt.Sprintf("allow %s pool=%s namespace=%s", access, volOptions.Pool, getVolumeNamespace(volUuid)),
	}

	var ents []cephEntity
	args := [...]string{
		"auth", "-f", "json",
		"get-or-create", cephEntityClientPrefix + getCephUserName(volUuid),
		"mds", caps.Mds,
		"mon", caps.Mon,
		"osd", caps.Osd,
	}

	if err := execCommandJson(&ents, "ceph", args[:]...); err != nil {
		return nil, fmt.Errorf("error creating ceph user: %v", err)
	}

	return &ents[0], nil
}

func deleteCephUser(volUuid string) error {
	userId := getCephUserName(volUuid)

	if err := execCommandAndValidate("ceph", "auth", "rm", cephEntityClientPrefix+userId); err != nil {
		return err
	}

	os.Remove(getCephKeyringPath(userId))
	os.Remove(getCephSecretPath(userId))

	return nil
}
