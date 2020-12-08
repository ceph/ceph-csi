/*
Copyright 2020 The Ceph-CSI Authors.

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
	"errors"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	kms := VaultTokensKMS{}

	config := make(map[string]interface{})
	secrets := make(map[string]string)

	// empty config map
	err := kms.parseConfig(config, secrets)
	if !errors.Is(err, errConfigOptionMissing) {
		t.Errorf("unexpected error (%T): %s", err, err)
	}

	// fill default options (normally done in InitVaultTokensKMS)
	config["vaultAddress"] = "https://vault.default.cluster.svc"
	config["tenantConfigName"] = vaultTokensDefaultConfigName
	config["tenantTokenName"] = vaultTokensDefaultTokenName

	// parsing with all required options
	err = kms.parseConfig(config, secrets)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case kms.ConfigName != vaultTokensDefaultConfigName:
		t.Errorf("ConfigName contains unexpected value: %s", kms.ConfigName)
	case kms.TokenName != vaultTokensDefaultTokenName:
		t.Errorf("TokenName contains unexpected value: %s", kms.TokenName)
	}

	// tenant "bob" uses a different kms.ConfigName
	bob := make(map[string]interface{})
	bob["tenantConfigName"] = "the-config-from-bob"
	err = kms.parseConfig(bob, secrets)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case kms.ConfigName != "the-config-from-bob":
		t.Errorf("ConfigName contains unexpected value: %s", kms.ConfigName)
	}
}

// TestInitVaultTokensKMS verifies that passing partial and complex
// configurations get applied correctly.
//
// When vault.New() is called at the end of InitVaultTokensKMS(), errors will
// mention the missing VAULT_TOKEN, and that is expected.
func TestInitVaultTokensKMS(t *testing.T) {
	if true {
		// FIXME: testing only works when KUBE_CONFIG is set to a
		// cluster that has a working Vault deployment
		return
	}

	config := make(map[string]interface{})
	secrets := make(map[string]string)

	// empty config map
	_, err := InitVaultTokensKMS("bob", "vault-tokens-config", config, secrets)
	if !errors.Is(err, errConfigOptionMissing) {
		t.Errorf("unexpected error (%T): %s", err, err)
	}

	// fill required options
	config["vaultAddress"] = "https://vault.default.cluster.svc"

	// parsing with all required options
	_, err = InitVaultTokensKMS("bob", "vault-tokens-config", config, secrets)
	if err != nil && !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("unexpected error: %s", err)
	}

	// fill tenants
	tenants := make(map[string]interface{})
	config["tenants"] = tenants

	// empty tenants list
	_, err = InitVaultTokensKMS("bob", "vault-tokens-config", config, secrets)
	if err != nil && !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("unexpected error: %s", err)
	}

	// add tenant "bob"
	bob := make(map[string]interface{})
	config["tenants"].(map[string]interface{})["bob"] = bob
	bob["vaultAddress"] = "https://vault.bob.example.org"

	_, err = InitVaultTokensKMS("bob", "vault-tokens-config", config, secrets)
	if err != nil && !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("unexpected error: %s", err)
	}
}
