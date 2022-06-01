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

package kms

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/vault/api"
	loss "github.com/libopenstorage/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Parallel()
	vtc := vaultTenantConnection{}

	config := make(map[string]interface{})

	// empty config map
	err := vtc.parseConfig(config)
	if !errors.Is(err, errConfigOptionMissing) {
		t.Errorf("unexpected error (%T): %s", err, err)
	}

	// fill default options (normally done in initVaultTokensKMS)
	config["vaultAddress"] = "https://vault.default.cluster.svc"
	config["vaultNamespace"] = "default"
	config["vaultAuthNamespace"] = "company-sso"
	config["tenantConfigName"] = vaultTokensDefaultConfigName

	// parsing with all required options
	err = vtc.parseConfig(config)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case vtc.ConfigName != vaultTokensDefaultConfigName:
		t.Errorf("ConfigName contains unexpected value: %s", vtc.ConfigName)
	}

	// tenant "bob" uses a different kms.ConfigName
	bob := make(map[string]interface{})
	bob["tenantConfigName"] = "the-config-from-bob"
	bob["vaultNamespace"] = "bobs-place"
	err = vtc.parseConfig(bob)
	switch {
	case err != nil:
		t.Errorf("unexpected error: %s", err)
	case vtc.ConfigName != "the-config-from-bob":
		t.Errorf("ConfigName contains unexpected value: %s", vtc.ConfigName)
	case vtc.vaultConfig[api.EnvVaultNamespace] != "company-sso":
		t.Errorf("EnvVaultNamespace contains unexpected value: %s", vtc.vaultConfig[api.EnvVaultNamespace])
	case vtc.keyContext[loss.KeyVaultNamespace] != "bobs-place":
		t.Errorf("KeyVaultNamespace contains unexpected value: %s", vtc.keyContext[loss.KeyVaultNamespace])
	}
}

// TestInitVaultTokensKMS verifies that passing partial and complex
// configurations get applied correctly.
//
// When vault.New() is called at the end of initVaultTokensKMS(), errors will
// mention the missing VAULT_TOKEN, and that is expected.
func TestInitVaultTokensKMS(t *testing.T) {
	t.Parallel()
	if true {
		// FIXME: testing only works when KUBE_CONFIG is set to a
		// cluster that has a working Vault deployment
		return
	}

	args := ProviderInitArgs{
		Tenant:  "bob",
		Config:  make(map[string]interface{}),
		Secrets: nil,
	}

	// empty config map
	_, err := initVaultTokensKMS(args)
	if !errors.Is(err, errConfigOptionMissing) {
		t.Errorf("unexpected error (%T): %s", err, err)
	}

	// fill required options
	args.Config["vaultAddress"] = "https://vault.default.cluster.svc"

	// parsing with all required options
	_, err = initVaultTokensKMS(args)
	if err != nil && !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("unexpected error: %s", err)
	}

	// fill tenants
	tenants := make(map[string]interface{})
	args.Config["tenants"] = tenants

	// empty tenants list
	_, err = initVaultTokensKMS(args)
	if err != nil && !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("unexpected error: %s", err)
	}

	// add tenant "bob"
	bob := make(map[string]interface{})
	bob["vaultAddress"] = "https://vault.bob.example.org"
	//nolint:forcetypeassert // as its a test we dont need to check assertion here.
	args.Config["tenants"].(map[string]interface{})["bob"] = bob

	_, err = initVaultTokensKMS(args)
	if err != nil && !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("unexpected error: %s", err)
	}
}

// TestStdVaultToCSIConfig converts a JSON document with standard VAULT_*
// environment variables to a vaultTokenConf structure.
func TestStdVaultToCSIConfig(t *testing.T) {
	t.Parallel()
	vaultConfigMap := `{
		"KMS_PROVIDER":"vaulttokens",
		"VAULT_ADDR":"https://vault.example.com",
		"VAULT_BACKEND":"kv-v2",
		"VAULT_BACKEND_PATH":"/secret",
		"VAULT_DESTROY_KEYS":"true",
		"VAULT_CACERT":"",
		"VAULT_TLS_SERVER_NAME":"vault.example.com",
		"VAULT_CLIENT_CERT":"",
		"VAULT_CLIENT_KEY":"",
		"VAULT_AUTH_NAMESPACE":"devops",
		"VAULT_NAMESPACE":"devops/homepage",
		"VAULT_SKIP_VERIFY":"true"
	}`

	sv := &standardVault{}
	err := json.Unmarshal([]byte(vaultConfigMap), sv)
	if err != nil {
		t.Errorf("unexpected error: %s", err)

		return
	}

	v := vaultTokenConf{}
	v.convertStdVaultToCSIConfig(sv)

	switch {
	case v.EncryptionKMSType != kmsTypeVaultTokens:
		t.Errorf("unexpected value for EncryptionKMSType: %s", v.EncryptionKMSType)
	case v.VaultAddress != "https://vault.example.com":
		t.Errorf("unexpected value for VaultAddress: %s", v.VaultAddress)
	case v.VaultBackend != "kv-v2":
		t.Errorf("unexpected value for VaultBackend: %s", v.VaultBackend)
	case v.VaultBackendPath != "/secret":
		t.Errorf("unexpected value for VaultBackendPath: %s", v.VaultBackendPath)
	case v.VaultDestroyKeys != vaultDefaultDestroyKeys:
		t.Errorf("unexpected value for VaultDestroyKeys: %s", v.VaultDestroyKeys)
	case v.VaultCAFromSecret != "":
		t.Errorf("unexpected value for VaultCAFromSecret: %s", v.VaultCAFromSecret)
	case v.VaultClientCertFromSecret != "":
		t.Errorf("unexpected value for VaultClientCertFromSecret: %s", v.VaultClientCertFromSecret)
	case v.VaultClientCertKeyFromSecret != "":
		t.Errorf("unexpected value for VaultClientCertKeyFromSecret: %s", v.VaultClientCertKeyFromSecret)
	case v.VaultAuthNamespace != "devops":
		t.Errorf("unexpected value for VaultAuthNamespace: %s", v.VaultAuthNamespace)
	case v.VaultNamespace != "devops/homepage":
		t.Errorf("unexpected value for VaultNamespace: %s", v.VaultNamespace)
	case v.VaultTLSServerName != "vault.example.com":
		t.Errorf("unexpected value for VaultTLSServerName: %s", v.VaultTLSServerName)
	case v.VaultCAVerify != "false":
		t.Errorf("unexpected value for VaultCAVerify: %s", v.VaultCAVerify)
	}
}

func TestTransformConfig(t *testing.T) {
	t.Parallel()
	cm := make(map[string]interface{})
	cm["KMS_PROVIDER"] = "vaulttokens"
	cm["VAULT_ADDR"] = "https://vault.example.com"
	cm["VAULT_BACKEND"] = "kv-v2"
	cm["VAULT_BACKEND_PATH"] = "/secret"
	cm["VAULT_DESTROY_KEYS"] = "true"
	cm["VAULT_CACERT"] = ""
	cm["VAULT_TLS_SERVER_NAME"] = "vault.example.com"
	cm["VAULT_CLIENT_CERT"] = ""
	cm["VAULT_CLIENT_KEY"] = ""
	cm["VAULT_AUTH_NAMESPACE"] = "devops"
	cm["VAULT_NAMESPACE"] = "devops/homepage"
	cm["VAULT_SKIP_VERIFY"] = "true" // inverse of "vaultCAVerify"

	config, err := transformConfig(cm)
	require.NoError(t, err)
	assert.Equal(t, config["encryptionKMSType"], cm["KMS_PROVIDER"])
	assert.Equal(t, config["vaultAddress"], cm["VAULT_ADDR"])
	assert.Equal(t, config["vaultBackend"], cm["VAULT_BACKEND"])
	assert.Equal(t, config["vaultBackendPath"], cm["VAULT_BACKEND_PATH"])
	assert.Equal(t, config["vaultDestroyKeys"], cm["VAULT_DESTROY_KEYS"])
	assert.Equal(t, config["vaultCAFromSecret"], cm["VAULT_CACERT"])
	assert.Equal(t, config["vaultTLSServerName"], cm["VAULT_TLS_SERVER_NAME"])
	assert.Equal(t, config["vaultClientCertFromSecret"], cm["VAULT_CLIENT_CERT"])
	assert.Equal(t, config["vaultClientCertKeyFromSecret"], cm["VAULT_CLIENT_KEY"])
	assert.Equal(t, config["vaultAuthNamespace"], cm["VAULT_AUTH_NAMESPACE"])
	assert.Equal(t, config["vaultNamespace"], cm["VAULT_NAMESPACE"])
	assert.Equal(t, config["vaultCAVerify"], "false")
}

func TestTransformConfigDefaults(t *testing.T) {
	t.Parallel()
	cm := make(map[string]interface{})
	cm["KMS_PROVIDER"] = kmsTypeVaultTokens

	config, err := transformConfig(cm)
	require.NoError(t, err)
	assert.Equal(t, config["encryptionKMSType"], cm["KMS_PROVIDER"])
	assert.Equal(t, config["vaultDestroyKeys"], vaultDefaultDestroyKeys)
	assert.Equal(t, config["vaultCAVerify"], strconv.FormatBool(vaultDefaultCAVerify))
}

func TestVaultTokensKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeVaultTokens]
	assert.True(t, ok)
}

func TestSetTenantAuthNamespace(t *testing.T) {
	t.Parallel()

	vaultNamespace := "tenant"

	t.Run("override vaultAuthNamespace", func(tt *testing.T) {
		tt.Parallel()

		kms := &vaultTenantConnection{}
		kms.keyContext = map[string]string{
			loss.KeyVaultNamespace: "global",
		}
		kms.vaultConfig = map[string]interface{}{
			api.EnvVaultNamespace: "global",
		}

		config := map[string]interface{}{
			"vaultNamespace": vaultNamespace,
		}

		kms.setTenantAuthNamespace(config)

		assert.Equal(tt, vaultNamespace, config["vaultAuthNamespace"])
	})

	t.Run("inherit vaultAuthNamespace", func(tt *testing.T) {
		tt.Parallel()

		vaultAuthNamespace := "configured"

		kms := &vaultTenantConnection{}
		kms.keyContext = map[string]string{
			loss.KeyVaultNamespace: vaultAuthNamespace,
		}
		kms.vaultConfig = map[string]interface{}{
			api.EnvVaultNamespace: "global",
		}

		config := map[string]interface{}{
			"vaultNamespace": vaultNamespace,
		}

		kms.setTenantAuthNamespace(config)

		// when inheriting from the global config, the config of the
		// tenant should not have vaultAuthNamespace configured
		assert.Equal(tt, nil, config["vaultAuthNamespace"])
	})

	t.Run("unset vaultAuthNamespace", func(tt *testing.T) {
		tt.Parallel()

		kms := &vaultTenantConnection{}
		kms.keyContext = map[string]string{
			// no vaultAuthNamespace configured
		}
		kms.vaultConfig = map[string]interface{}{
			api.EnvVaultNamespace: "global",
		}

		config := map[string]interface{}{
			"vaultNamespace": vaultNamespace,
		}

		kms.setTenantAuthNamespace(config)

		// global vaultAuthNamespace is not set, tenant
		// vaultAuthNamespace will be configured as vaultNamespace by
		// default
		assert.Equal(tt, nil, config["vaultAuthNamespace"])
	})

	t.Run("no vaultNamespace", func(tt *testing.T) {
		tt.Parallel()

		kms := &vaultTenantConnection{}
		kms.keyContext = map[string]string{
			// no vaultAuthNamespace configured
		}
		kms.vaultConfig = map[string]interface{}{
			// no vaultNamespace configured
		}

		config := map[string]interface{}{
			// no tenant namespaces configured
		}

		kms.setTenantAuthNamespace(config)

		assert.Equal(tt, nil, config["vaultAuthNamespace"])
	})
}
