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
	"io/ioutil"
	"os"
)

var cephConfig = []byte(`[global]
auth_cluster_required = cephx
auth_service_required = cephx
auth_client_required = cephx

# Workaround for http://tracker.ceph.com/issues/23446
fuse_set_user_groups = false
`)

const (
	cephConfigRoot = "/etc/ceph"
	cephConfigPath = "/etc/ceph/ceph.conf"
)

func createCephConfigRoot() error {
	return os.MkdirAll(cephConfigRoot, 0755) // #nosec
}

func writeCephConfig() error {
	if err := createCephConfigRoot(); err != nil {
		return err
	}

	return ioutil.WriteFile(cephConfigPath, cephConfig, 0640)
}
