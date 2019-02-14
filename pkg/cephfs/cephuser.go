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

func getSingleCephEntity(args ...string) (*cephEntity, error) {
	var ents []cephEntity
	if err := execCommandJSON(&ents, "ceph", args...); err != nil {
		return nil, err
	}

	if len(ents) != 1 {
		return nil, fmt.Errorf("got unexpected number of entities: expected 1, got %d", len(ents))
	}

	return &ents[0], nil
}

func getCephUser(volOptions *volumeOptions, adminCr *credentials, volID volumeID) (*cephEntity, error) {
	return getSingleCephEntity(
		"-m", volOptions.Monitors,
		"-n", cephEntityClientPrefix+adminCr.id, "--key="+adminCr.key,
		"-c", cephConfigPath,
		"-f", "json",
		"auth", "get", cephEntityClientPrefix+getCephUserName(volID),
	)
}

func createCephUser(volOptions *volumeOptions, adminCr *credentials, volID volumeID) (*cephEntity, error) {
	return getSingleCephEntity(
		"-m", volOptions.Monitors,
		"-n", cephEntityClientPrefix+adminCr.id, "--key="+adminCr.key,
		"-c", cephConfigPath,
		"-f", "json",
		"auth", "get-or-create", cephEntityClientPrefix+getCephUserName(volID),
		"mds", fmt.Sprintf("allow rw path=%s", getVolumeRootPathCeph(volID)),
		"mon", "allow r",
		"osd", fmt.Sprintf("allow rw pool=%s namespace=%s", volOptions.Pool, getVolumeNamespace(volID)),
	)
}

func deleteCephUser(volOptions *volumeOptions, adminCr *credentials, volID volumeID) error {
	return execCommandErr("ceph",
		"-m", volOptions.Monitors,
		"-n", cephEntityClientPrefix+adminCr.id, "--key="+adminCr.key,
		"-c", cephConfigPath,
		"auth", "rm", cephEntityClientPrefix+getCephUserName(volID),
	)
}
