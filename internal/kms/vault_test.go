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
	"errors"
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
	loss "github.com/libopenstorage/secrets"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/util/file"
)

func TestDetectAuthMountPath(t *testing.T) {
	t.Parallel()
	authMountPath, err := detectAuthMountPath(vaultDefaultAuthPath)
	if err != nil {
		t.Errorf("detectAuthMountPath() failed: %s", err)
	}
	if authMountPath != "kubernetes" {
		t.Errorf("authMountPath should be set to 'kubernetes', but is: %s", authMountPath)
	}

	authMountPath, err = detectAuthMountPath("kubernetes")
	if err != nil {
		t.Errorf("detectAuthMountPath() failed: %s", err)
	}
	if authMountPath != "kubernetes" {
		t.Errorf("authMountPath should be set to 'kubernetes', but is: %s", authMountPath)
	}
}

func TestSetConfigString(t *testing.T) {
	t.Parallel()
	const defaultValue = "default-value"
	options := make(map[string]any)

	// noSuchOption: no default value, option unavailable
	noSuchOption := ""
	err := setConfigString(&noSuchOption, options, "nonexistent")
	switch {
	case err == nil:
		t.Error("did not get an error when one was expected")
	case !errors.Is(err, errConfigOptionMissing):
		t.Errorf("expected errConfigOptionMissing, but got %T: %s", err, err)
	case noSuchOption != "":
		t.Error("value should not have been modified")
	}

	// noOptionDefault: default value, option unavailable
	noOptionDefault := defaultValue
	err = setConfigString(&noOptionDefault, options, "nonexistent")
	switch {
	case err == nil:
		t.Error("did not get an error when one was expected")
	case !errors.Is(err, errConfigOptionMissing):
		t.Errorf("expected errConfigOptionMissing, but got %T: %s", err, err)
	case noOptionDefault != defaultValue:
		t.Error("value should not have been modified")
	}

	// optionDefaultOverload: default value, option available
	optionDefaultOverload := defaultValue
	options["set-me"] = "non-default"
	err = setConfigString(&optionDefaultOverload, options, "set-me")
	switch {
	case err != nil:
		t.Errorf("unexpected error returned: %s", err)
	case optionDefaultOverload != "non-default":
		t.Error("optionDefaultOverload should have been updated")
	}
}

func TestDefaultVaultDestroyKeys(t *testing.T) {
	t.Parallel()

	vc := &vaultConnection{}
	config := make(map[string]any)
	config["vaultAddress"] = "https://vault.test.example.com"
	err := vc.initConnection(config)
	require.NoError(t, err)
	keyContext := vc.getDeleteKeyContext()
	destroySecret, ok := keyContext[loss.DestroySecret]
	require.NotEmpty(t, destroySecret)
	require.True(t, ok)

	// setting vaultDestroyKeys to !true should remove the loss.DestroySecret entry
	config["vaultDestroyKeys"] = "false"
	err = vc.initConnection(config)
	require.NoError(t, err)
	keyContext = vc.getDeleteKeyContext()
	_, ok = keyContext[loss.DestroySecret]
	require.False(t, ok)
}

func TestVaultKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeVault]
	require.True(t, ok)
}

func TestInitCertificatesConfigParsing(t *testing.T) {
	t.Parallel()

	t.Run("invalid config type returns error", func(t *testing.T) {
		t.Parallel()
		vc := &vaultConnection{vaultConfig: make(map[string]any)}
		config := map[string]any{"vaultCAFromSecret": 12345} // wrong type
		err := vc.initCertificates(config, nil)
		require.Error(t, err)
		require.True(t, errors.Is(err, errConfigOptionInvalid))
	})

	t.Run("empty config is a no-op", func(t *testing.T) {
		t.Parallel()
		vc := &vaultConnection{vaultConfig: make(map[string]any)}
		err := vc.initCertificates(map[string]any{}, nil)
		require.NoError(t, err)
		_, hasCA := vc.vaultConfig[api.EnvVaultCACert]
		require.False(t, hasCA)
		_, hasCert := vc.vaultConfig[api.EnvVaultClientCert]
		require.False(t, hasCert)
		_, hasKey := vc.vaultConfig[api.EnvVaultClientKey]
		require.False(t, hasKey)
	})
}

func TestDestroyRemovesAllTempFiles(t *testing.T) {
	t.Parallel()

	caFile, err := file.CreateTempFile("vault-ca-cert", "fake-ca")
	require.NoError(t, err)
	certFile, err := file.CreateTempFile("vault-client-cert", "fake-cert")
	require.NoError(t, err)
	keyFile, err := file.CreateTempFile("vault-client-cert-key", "fake-key")
	require.NoError(t, err)

	vc := &vaultConnection{
		vaultConfig: map[string]any{
			api.EnvVaultCACert:     caFile.Name(),
			api.EnvVaultClientCert: certFile.Name(),
			api.EnvVaultClientKey:  keyFile.Name(),
		},
	}
	vc.Destroy()

	_, err = os.Stat(caFile.Name())
	require.True(t, os.IsNotExist(err), "CA cert temp file should be removed")
	_, err = os.Stat(certFile.Name())
	require.True(t, os.IsNotExist(err), "client cert temp file should be removed")
	_, err = os.Stat(keyFile.Name())
	require.True(t, os.IsNotExist(err), "client key temp file should be removed")
}
