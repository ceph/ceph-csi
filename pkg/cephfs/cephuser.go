/*
Copyright 2018 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/pkg/util"
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

func (ent *cephEntity) toCredentials() *util.Credentials {
	return &util.Credentials{
		ID:  ent.Entity[len(cephEntityClientPrefix):],
		Key: ent.Key,
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

func genUserIDs(adminCr *util.Credentials, volID volumeID) (adminID, userID string) {
	return cephEntityClientPrefix + adminCr.ID, cephEntityClientPrefix + getCephUserName(volID)
}

func getCephUser(volOptions *volumeOptions, adminCr *util.Credentials, volID volumeID) (*cephEntity, error) {
	adminID, userID := genUserIDs(adminCr, volID)

	return getSingleCephEntity(
		"-m", volOptions.Monitors,
		"-n", adminID,
		"--key="+adminCr.Key,
		"-c", util.CephConfigPath,
		"-f", "json",
		"auth", "get", userID,
	)
}

func createCephUser(volOptions *volumeOptions, adminCr *util.Credentials, volID volumeID) (*cephEntity, error) {
	adminID, userID := genUserIDs(adminCr, volID)
	volRootPath, err := getVolumeRootPathCeph(volOptions, adminCr, volID)
	if err != nil {
		return nil, err
	}

	return getSingleCephEntity(
		"-m", volOptions.Monitors,
		"-n", adminID,
		"--key="+adminCr.Key,
		"-c", util.CephConfigPath,
		"-f", "json",
		"auth", "get-or-create", userID,
		// User capabilities
		"mds", fmt.Sprintf("allow rw path=%s", volRootPath),
		"mon", "allow r",
		"osd", fmt.Sprintf("allow rw pool=%s namespace=%s", volOptions.Pool, getVolumeNamespace(volID)),
	)
}

func deleteCephUser(volOptions *volumeOptions, adminCr *util.Credentials, volID volumeID) error {
	adminID, userID := genUserIDs(adminCr, volID)

	// TODO: Need to return success if userID is not found
	return execCommandErr("ceph",
		"-m", volOptions.Monitors,
		"-n", adminID,
		"--key="+adminCr.Key,
		"-c", util.CephConfigPath,
		"auth", "rm", userID,
	)
}
