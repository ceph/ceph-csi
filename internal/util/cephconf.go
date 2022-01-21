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

package util

import (
	"os"
)

var cephConfig = []byte(`[global]
auth_cluster_required = cephx
auth_service_required = cephx
auth_client_required = cephx

# Workaround for http://tracker.ceph.com/issues/23446
fuse_set_user_groups = false

# ceph-fuse which uses libfuse2 by default has write buffer size of 2KiB
# adding 'fuse_big_writes = true' option by default to override this limit
# see https://github.com/ceph/ceph-csi/issues/1928
fuse_big_writes = true
`)

const (
	cephConfigRoot = "/etc/ceph"
	// CephConfigPath ceph configuration file.
	CephConfigPath = "/etc/ceph/ceph.conf"

	keyRing = "/etc/ceph/keyring"
)

func createCephConfigRoot() error {
	return os.MkdirAll(cephConfigRoot, 0o755) // #nosec
}

// WriteCephConfig writes out a basic ceph.conf file, making it easy to use
// ceph related CLIs.
func WriteCephConfig() error {
	var err error
	if err = createCephConfigRoot(); err != nil {
		return err
	}

	// create config file if it does not exist to support backward compatibility
	if _, err = os.Stat(CephConfigPath); os.IsNotExist(err) {
		err = os.WriteFile(CephConfigPath, cephConfig, 0o600)
	}

	if err != nil {
		return err
	}

	return createKeyRingFile()
}

/*
if any ceph commands fails it will log below error message

7f39ff02a700 -1 auth: unable to find a keyring on
/etc/ceph/ceph.client.admin.keyring,/etc/ceph/ceph.keyring,/etc/ceph/keyring,
/etc/ceph/keyring.bin,: (2) No such file or directory
*/
// createKeyRingFile creates the keyring files to fix above error message logging.
func createKeyRingFile() error {
	var err error
	// create keyring file if it does not exist to support backward compatibility
	if _, err = os.Stat(keyRing); os.IsNotExist(err) {
		_, err = os.Create(keyRing)
	}

	return err
}
