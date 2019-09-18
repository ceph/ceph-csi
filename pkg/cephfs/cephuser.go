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
	"context"

	"github.com/ceph/ceph-csi/pkg/util"
)

const (
	cephUserPrefix         = "user-"
	cephEntityClientPrefix = "client."
)

func genUserIDs(adminCr *util.Credentials, volID volumeID) (adminID, userID string) {
	return cephEntityClientPrefix + adminCr.ID, cephEntityClientPrefix + getCephUserName(volID)
}

func getCephUserName(volID volumeID) string {
	return cephUserPrefix + string(volID)
}

func deleteCephUserDeprecated(ctx context.Context, volOptions *volumeOptions, adminCr *util.Credentials, volID volumeID) error {
	adminID, userID := genUserIDs(adminCr, volID)

	// TODO: Need to return success if userID is not found
	return execCommandErr(ctx, "ceph",
		"-m", volOptions.Monitors,
		"-n", adminID,
		"--keyfile="+adminCr.KeyFile,
		"-c", util.CephConfigPath,
		"auth", "rm", userID,
	)
}
