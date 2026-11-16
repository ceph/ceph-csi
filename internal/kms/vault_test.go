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
		err := vc.initCertificates(config, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, errConfigOptionInvalid)
	})

	t.Run("empty config is a no-op", func(t *testing.T) {
		t.Parallel()
		vc := &vaultConnection{vaultConfig: make(map[string]any)}
		err := vc.initCertificates(map[string]any{}, nil, nil)
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

func TestVaultCertificateSecrets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		config     map[string]any
		secrets    map[string]string
		data       map[string]map[string]string
		want       map[string]string
		calls      int
		wantErr    string
		fetchError bool
	}{
		{
			name:    "legacy CA takes precedence",
			config:  map[string]any{"vaultCAFromSecret": "ca"},
			secrets: map[string]string{"ca": "legacy-ca"},
			want:    map[string]string{api.EnvVaultCACert: "legacy-ca"},
		},
		{
			name:   "CA falls back to Kubernetes Secret",
			config: map[string]any{"vaultCAFromSecret": "ca"},
			data:   map[string]map[string]string{"ca": {"cert": "ca-pem"}},
			want:   map[string]string{api.EnvVaultCACert: "ca-pem"}, calls: 1,
		},
		{
			name:    "combined client Secret",
			config:  map[string]any{"vaultClientCertFromSecret": "tls", "vaultClientCertKeyFromSecret": "tls"},
			secrets: map[string]string{"tls": "must-not-be-used-for-client-TLS"},
			data:    map[string]map[string]string{"tls": {"cert": "client-pem", "key": "key-pem"}},
			want:    map[string]string{api.EnvVaultClientCert: "client-pem", api.EnvVaultClientKey: "key-pem"}, calls: 1,
		},
		{
			name:   "separate client Secrets",
			config: map[string]any{"vaultClientCertFromSecret": "client", "vaultClientCertKeyFromSecret": "key"},
			data:   map[string]map[string]string{"client": {"cert": "client-pem"}, "key": {"key": "key-pem"}},
			want:   map[string]string{api.EnvVaultClientCert: "client-pem", api.EnvVaultClientKey: "key-pem"}, calls: 2,
		},
		{
			name:    "missing key option fails before fetching CA",
			config:  map[string]any{"vaultCAFromSecret": "ca", "vaultClientCertFromSecret": "client"},
			wantErr: "must both be set",
		},
		{
			name:   "missing certificate option",
			config: map[string]any{"vaultClientCertKeyFromSecret": "key"}, wantErr: "must both be set",
		},
		{
			name:   "invalid certificate option",
			config: map[string]any{"vaultClientCertFromSecret": 123}, wantErr: "expected string",
		},
		{
			name:   "invalid key option",
			config: map[string]any{"vaultClientCertKeyFromSecret": 123}, wantErr: "expected string",
		},
		{
			name:   "missing CA field",
			config: map[string]any{"vaultCAFromSecret": "ca"},
			data:   map[string]map[string]string{"ca": {}}, calls: 1, wantErr: "missing or empty",
		},
		{
			name:   "empty client certificate",
			config: map[string]any{"vaultClientCertFromSecret": "tls", "vaultClientCertKeyFromSecret": "tls"},
			data:   map[string]map[string]string{"tls": {"cert": "", "key": "key-pem"}},
			calls:  1, wantErr: "missing or empty",
		},
		{
			name:   "missing key retains certificate for cleanup",
			config: map[string]any{"vaultClientCertFromSecret": "tls", "vaultClientCertKeyFromSecret": "tls"},
			data:   map[string]map[string]string{"tls": {"cert": "client-pem"}},
			want:   map[string]string{api.EnvVaultClientCert: "client-pem"}, calls: 1, wantErr: "missing or empty",
		},
		{
			name: "API error retains CA for cleanup",
			config: map[string]any{
				"vaultCAFromSecret": "ca", "vaultClientCertFromSecret": "tls", "vaultClientCertKeyFromSecret": "tls",
			},
			secrets: map[string]string{"ca": "legacy-ca"},
			want:    map[string]string{api.EnvVaultCACert: "legacy-ca"},
			calls:   1, fetchError: true, wantErr: "secret access denied",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(podNamespaceEnv, "csi-system")
			vc := &vaultConnection{vaultConfig: make(map[string]any)}
			t.Cleanup(vc.Destroy)
			calls := 0
			err := vc.initCertificates(tc.config, tc.secrets, func(name, ns string) (map[string]string, error) {
				calls++
				require.Equal(t, "csi-system", ns)
				if tc.fetchError {
					return nil, errors.New("secret access denied")
				}
				data, ok := tc.data[name]
				require.True(t, ok, "unexpected Secret lookup: %s", name)

				return data, nil
			})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.calls, calls)
			require.Len(t, vc.vaultConfig, len(tc.want))
			for env, pem := range tc.want {
				name, ok := vc.vaultConfig[env].(string)
				require.True(t, ok)
				contents, err := os.ReadFile(name)
				require.NoError(t, err)
				require.Equal(t, pem, string(contents))
			}
			vc.Destroy()
			for env := range tc.want {
				name, ok := vc.vaultConfig[env].(string)
				require.True(t, ok)
				require.NoFileExists(t, name)
			}
		})
	}
}

func TestVaultCertificatesWithoutPodNamespace(t *testing.T) {
	t.Setenv(podNamespaceEnv, "")
	vc := &vaultConnection{vaultConfig: make(map[string]any)}
	t.Cleanup(vc.Destroy)
	config := map[string]any{"vaultCAFromSecret": "ca"}
	// Existing CSI request CA secrets do not require Kubernetes API access.
	err := vc.initCertificates(config, map[string]string{"ca": "legacy-ca"}, nil)
	require.NoError(t, err)
	vc.Destroy()

	err = vc.initCertificates(config, nil, nil)
	require.ErrorContains(t, err, podNamespaceEnv)
}

func TestInitVaultKMSFailureCleansCertificates(t *testing.T) {
	t.Setenv(podNamespaceEnv, "")
	for _, tc := range []struct {
		name    string
		config  map[string]any
		wantErr string
	}{
		{
			name:    "certificate initialization failure",
			config:  map[string]any{"vaultClientCertFromSecret": "tls", "vaultClientCertKeyFromSecret": "tls"},
			wantErr: "failed to initialize Vault certificates",
		},
		{name: "later configuration failure", config: map[string]any{}, wantErr: "vaultAuthPath"},
		{
			name: "connection failure",
			config: map[string]any{
				"vaultAuthPath": vaultDefaultAuthPath, "vaultRole": vaultDefaultRole, "vaultPassphrasePath": "",
			},
			wantErr: "failed connecting to Vault",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			tc.config["vaultAddress"] = "https://vault.test.example.com"
			tc.config["vaultCAFromSecret"] = "ca"
			kms, err := initVaultKMS(ProviderInitArgs{Config: tc.config, Secrets: map[string]string{"ca": "invalid-ca"}})
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, kms)
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, files, "failed initialization must remove its temporary certificates")
		})
	}
}
