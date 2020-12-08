/*
Copyright 2019 The Ceph-CSI Authors.

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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/vault/api"
	loss "github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/vault"
)

const (
	// path to service account token that will be used to authenticate with Vault
	// #nosec
	serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	// vault configuration defaults
	vaultDefaultAuthPath       = "/v1/auth/kubernetes/login"
	vaultDefaultRole           = "csi-kubernetes"
	vaultDefaultNamespace      = ""
	vaultDefaultPassphrasePath = ""
)

/*
VaultKMS represents a Hashicorp Vault KMS configuration

Example JSON structure in the KMS config is,
{
	"local_vault_unique_identifier": {
		"encryptionKMSType": "vault",
		"vaultAddress": "https://127.0.0.1:8500",
		"vaultAuthPath": "/v1/auth/kubernetes/login",
		"vaultRole": "csi-kubernetes",
		"vaultNamespace": "",
		"vaultPassphraseRoot": "/v1/secret",
		"vaultPassphrasePath": "",
		"vaultCAVerify": true,
		"vaultCAFromSecret": "vault-ca"
	},
	...
}.
*/

type vaultConnection struct {
	EncryptionKMSID string
	vaultConfig     map[string]interface{}
	keyContext      map[string]string
}

type VaultKMS struct {
	vaultConnection

	// vaultPassphrasePath (VPP) used to be added before the "key" of the
	// secret (like /v1/secret/data/<VPP>/key)
	vaultPassphrasePath string

	secrets loss.Secrets
}

func (vc *vaultConnection) initConnection(kmsID string, config, secrets map[string]string) error {
	vaultConfig := make(map[string]interface{})
	keyContext := make(map[string]string)

	vc.EncryptionKMSID = kmsID

	vaultAddress, ok := config["vaultAddress"]
	if !ok || vaultAddress == "" {
		return errors.New("missing 'vaultAddress' for Vault connection")
	}
	vaultConfig[api.EnvVaultAddress] = vaultAddress

	vaultNamespace, ok := config["vaultNamespace"]
	if !ok || vaultNamespace == "" {
		vaultNamespace = vaultDefaultNamespace
	}
	vaultConfig[api.EnvVaultNamespace] = vaultNamespace
	keyContext[loss.KeyVaultNamespace] = vaultNamespace

	verifyCA, ok := config["vaultCAVerify"]
	if ok {
		var vaultCAVerify bool
		vaultCAVerify, err := strconv.ParseBool(verifyCA)
		if err != nil {
			return fmt.Errorf("failed to parse 'vaultCAVerify': %w", err)
		}
		vaultConfig[api.EnvVaultInsecure] = !vaultCAVerify
	}

	vaultCAFromSecret, ok := config["vaultCAFromSecret"]
	if ok && vaultCAFromSecret != "" {
		caPEM, ok := secrets[vaultCAFromSecret]
		if !ok {
			return fmt.Errorf("missing vault CA in secret %s", vaultCAFromSecret)
		}

		var err error
		vaultConfig[api.EnvVaultCACert], err = createTempFile("vault-ca-cert", []byte(caPEM))
		if err != nil {
			return fmt.Errorf("failed to create temporary file for Vault CA: %w", err)
		}
		// TODO: delete f.Name() when vaultConnection is destroyed
	}

	vc.keyContext = keyContext
	vc.vaultConfig = vaultConfig

	return nil
}

// InitVaultKMS returns an interface to HashiCorp Vault KMS.
func InitVaultKMS(kmsID string, config, secrets map[string]string) (EncryptionKMS, error) {
	var (
		ok  bool
		err error
	)

	kms := &VaultKMS{}
	err = kms.initConnection(kmsID, config, secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault connection: %w", err)
	}

	vaultAuthPath, ok := config["vaultAuthPath"]
	if !ok || vaultAuthPath == "" {
		vaultAuthPath = vaultDefaultAuthPath
	}
	kms.vaultConfig[vault.AuthMountPath], err = detectAuthMountPath(vaultAuthPath)
	if err != nil {
		return nil, fmt.Errorf("failed to set %s in Vault config: %w", vault.AuthMountPath, err)
	}

	vaultRole, ok := config["vaultRole"]
	if !ok || vaultRole == "" {
		vaultRole = vaultDefaultRole
	}
	kms.vaultConfig[vault.AuthKubernetesRole] = vaultRole

	// vault.VaultBackendPathKey is "secret/" by default, use vaultPassphraseRoot if configured
	vaultPassphraseRoot, ok := config["vaultPassphraseRoot"]
	if ok && vaultPassphraseRoot != "" {
		// the old example did have "/v1/secret/", convert that format
		if strings.HasPrefix(vaultPassphraseRoot, "/v1/") {
			kms.vaultConfig[vault.VaultBackendPathKey] = strings.TrimPrefix(vaultPassphraseRoot, "/v1/")
		} else {
			kms.vaultConfig[vault.VaultBackendPathKey] = vaultPassphraseRoot
		}
	}

	kms.vaultPassphrasePath, ok = config["vaultPassphrasePath"]
	if !ok || kms.vaultPassphrasePath == "" {
		kms.vaultPassphrasePath = vaultDefaultPassphrasePath
	}

	// FIXME: vault.AuthKubernetesTokenPath is not enough? EnvVaultToken needs to be set?
	kms.vaultConfig[vault.AuthMethod] = vault.AuthMethodKubernetes
	kms.vaultConfig[vault.AuthKubernetesTokenPath] = serviceAccountTokenPath

	v, err := vault.New(kms.vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed creating new Vault Secrets: %w", err)
	}
	kms.secrets = v

	return kms, nil
}

// GetID is returning correlation ID to KMS configuration.
func (vc *vaultConnection) GetID() string {
	return vc.EncryptionKMSID
}

// GetPassphrase returns passphrase from Vault. The passphrase is stored in a
// data.data.passphrase structure.
func (kms *VaultKMS) GetPassphrase(key string) (string, error) {
	s, err := kms.secrets.GetSecret(filepath.Join(kms.vaultPassphrasePath, key), kms.keyContext)
	if errors.Is(err, loss.ErrInvalidSecretId) {
		return "", MissingPassphrase{err}
	} else if err != nil {
		return "", err
	}

	data, ok := s["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed parsing data for get passphrase request for %q", key)
	}
	passphrase, ok := data["passphrase"].(string)
	if !ok {
		return "", fmt.Errorf("failed parsing passphrase for get passphrase request for %q", key)
	}

	return passphrase, nil
}

// SavePassphrase saves new passphrase in Vault.
func (kms *VaultKMS) SavePassphrase(key, value string) error {
	data := map[string]interface{}{
		"data": map[string]string{
			"passphrase": value,
		},
	}

	pathKey := filepath.Join(kms.vaultPassphrasePath, key)
	err := kms.secrets.PutSecret(pathKey, data, kms.keyContext)
	if err != nil {
		return fmt.Errorf("saving passphrase at %s request to vault failed: %w", pathKey, err)
	}

	return nil
}

// DeletePassphrase deletes passphrase from Vault.
func (kms *VaultKMS) DeletePassphrase(key string) error {
	pathKey := filepath.Join(kms.vaultPassphrasePath, key)
	err := kms.secrets.DeleteSecret(pathKey, kms.keyContext)
	if err != nil {
		return fmt.Errorf("delete passphrase at %s request to vault failed: %w", pathKey, err)
	}

	return nil
}

// detectAuthMountPath takes the vaultAuthPath configuration option that
// defaults to "/v1/auth/kubernetes/login" and makes it a vault.AuthMountPath
// like "kubernetes".
func detectAuthMountPath(path string) (string, error) {
	var authMountPath string

	if path == "" {
		return "", errors.New("path is empty")
	}

	// add all components between "login" and "auth" to authMountPath
	match := false
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "auth" {
			match = true
			continue
		}
		if part == "login" {
			break
		}
		if match && authMountPath == "" {
			authMountPath = part
		} else if match {
			authMountPath += "/" + part
		}
	}

	// in case authMountPath is empty, return original path as it was
	if authMountPath == "" {
		authMountPath = path
	}

	return authMountPath, nil
}

// createTempFile writes data to a temporary file that contains the pattern in
// the filename (see ioutil.TempFile for details).
func createTempFile(pattern string, data []byte) (string, error) {
	t, err := ioutil.TempFile("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}

	// delete the tmpfile on error
	defer func() {
		if err != nil {
			// ignore error on failure to remove tmpfile (gosec complains)
			_ = os.Remove(t.Name())
		}
	}()

	s, err := t.Write(data)
	if err != nil || s != len(data) {
		return "", fmt.Errorf("failed to write temporary file: %w", err)
	}
	err = t.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close temporary file: %w", err)
	}

	return t.Name(), nil
}
