/*
Copyright 2021 The Ceph-CSI Authors.

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

package kms

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVaultTenantSAKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeVaultTenantSA]
	assert.True(t, ok)
}

func TestTenantSAParseConfig(t *testing.T) {
	t.Parallel()
	vts := vaultTenantSA{}

	config := make(map[string]interface{})

	// empty config map
	err := vts.parseConfig(config)
	if !errors.Is(err, errConfigOptionMissing) {
		t.Errorf("unexpected error (%T): %s", err, err)
	}

	// fill default options (normally done in initVaultTokensKMS)
	config["vaultAddress"] = "https://vault.bob.cluster.svc"
	config["vaultAuthPath"] = "/v1/auth/kube-auth/login"

	// parsing with all required options
	err = vts.parseConfig(config)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case vts.vaultConfig["VAULT_AUTH_MOUNT_PATH"] != "kube-auth":
		t.Errorf("vaultAuthPath set to unexpected value: %s", vts.vaultConfig["VAULT_AUTH_MOUNT_PATH"])
	}

	// tenant "bob" uses a different auth mount path
	bob := make(map[string]interface{})
	bob["vaultAuthPath"] = "/v1/auth/bobs-cluster/login"
	err = vts.parseConfig(bob)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case vts.vaultConfig["VAULT_AUTH_MOUNT_PATH"] != "bobs-cluster":
		t.Errorf("vaultAuthPath set to unexpected value: %s", vts.vaultConfig["VAULT_AUTH_MOUNT_PATH"])
	}

	// auth mount path can be passed like VAULT_AUTH_MOUNT_PATH too
	bob["vaultAuthPath"] = "bobs-2nd-cluster"
	err = vts.parseConfig(bob)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case vts.vaultConfig["VAULT_AUTH_MOUNT_PATH"] != "bobs-2nd-cluster":
		t.Errorf("vaultAuthPath set to unexpected value: %s", vts.vaultConfig["VAULT_AUTH_MOUNT_PATH"])
	}
}
