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

package kms

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/vault/api"
	loss "github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/vault"
)

const (
	kmsTypeVault = "vault"

	// path to service account token that will be used to authenticate with Vault
	// #nosec
	serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	// vault configuration defaults.
	vaultDefaultAuthPath       = "/v1/auth/kubernetes/login"
	vaultDefaultAuthMountPath  = "kubernetes" // main component of vaultAuthPath
	vaultDefaultRole           = "csi-kubernetes"
	vaultDefaultNamespace      = ""
	vaultDefaultPassphrasePath = ""
	vaultDefaultCAVerify       = true
	vaultDefaultDestroyKeys    = "true"
)

var (
	errConfigOptionMissing = errors.New("configuration option not set")
	errConfigOptionInvalid = errors.New("configuration option not valid")
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
	secrets     loss.Secrets
	vaultConfig map[string]interface{}
	keyContext  map[string]string

	// vaultDestroyKeys will by default set to `true`, and causes secrets
	// to be deleted from Hashicorp Vault to be completely removed. Usually
	// secrets in a kv-v2 store will be soft-deleted, and recovering the
	// contents is still possible.
	//
	// This option is only valid during deletion of keys, see
	// getDeleteKeyContext() for more details.
	vaultDestroyKeys bool
}

type vaultKMS struct {
	vaultConnection
	integratedDEK

	// vaultPassphrasePath (VPP) used to be added before the "key" of the
	// secret (like /v1/secret/data/<VPP>/key)
	vaultPassphrasePath string
}

// setConfigString fetches a value from a configuration map and converts it to
// a string.
//
// If the value is not available, *option is not adjusted and
// errConfigOptionMissing is returned.
// In case the value is available, but can not be converted to a string,
// errConfigOptionInvalid is returned.
func setConfigString(option *string, config map[string]interface{}, key string) error {
	value, ok := config[key]
	if !ok {
		return fmt.Errorf("%w: %s", errConfigOptionMissing, key)
	}

	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string for %q, but got %T",
			errConfigOptionInvalid, key, value)
	}

	*option = s

	return nil
}

// initConnection sets VAULT_* environment variables in the vc.vaultConfig map,
// these settings will be used when connecting to the Vault service with
// vc.connectVault().
//
//nolint:gocyclo,cyclop // iterating through many config options, not complex at all.
func (vc *vaultConnection) initConnection(config map[string]interface{}) error {
	vaultConfig := make(map[string]interface{})
	keyContext := make(map[string]string)

	firstInit := (vc.vaultConfig == nil)

	vaultAddress := "" // required
	err := setConfigString(&vaultAddress, config, "vaultAddress")
	switch {
	case errors.Is(err, errConfigOptionInvalid):
		return err
	case firstInit && errors.Is(err, errConfigOptionMissing):
		return err
	case !errors.Is(err, errConfigOptionMissing):
		vaultConfig[api.EnvVaultAddress] = vaultAddress
	}
	// default: !firstInit

	vaultBackend := "" // optional
	err = setConfigString(&vaultBackend, config, "vaultBackend")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// set the option if the value was not invalid
	if !errors.Is(err, errConfigOptionMissing) {
		vaultConfig[vault.VaultBackendKey] = vaultBackend
	}

	vaultBackendPath := "" // optional
	err = setConfigString(&vaultBackendPath, config, "vaultBackendPath")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// set the option if the value was not invalid
	if !errors.Is(err, errConfigOptionMissing) {
		vaultConfig[vault.VaultBackendPathKey] = vaultBackendPath
	}

	// always set the default to prevent recovering kv-v2 keys
	vaultDestroyKeys := vaultDefaultDestroyKeys
	err = setConfigString(&vaultDestroyKeys, config, "vaultDestroyKeys")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	if firstInit || !errors.Is(err, errConfigOptionMissing) {
		if vaultDestroyKeys == vaultDefaultDestroyKeys {
			vc.vaultDestroyKeys = true
		} else {
			vc.vaultDestroyKeys = false
		}
	}

	vaultTLSServerName := "" // optional
	err = setConfigString(&vaultTLSServerName, config, "vaultTLSServerName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// set the option if the value was not invalid
	if !errors.Is(err, errConfigOptionMissing) {
		vaultConfig[api.EnvVaultTLSServerName] = vaultTLSServerName
	}

	vaultNamespace := vaultDefaultNamespace // optional
	err = setConfigString(&vaultNamespace, config, "vaultNamespace")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// set the option if the value was not invalid
	if firstInit || !errors.Is(err, errConfigOptionMissing) {
		keyContext[loss.KeyVaultNamespace] = vaultNamespace
	}

	vaultAuthNamespace := ""
	err = setConfigString(&vaultAuthNamespace, config, "vaultAuthNamespace")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// if the vaultAuthNamespace key is present and value is empty in config, set
	// the optional vaultNamespace.
	if vaultAuthNamespace == "" {
		vaultAuthNamespace = vaultNamespace
	}
	// set the option if the value was not invalid
	if firstInit || !errors.Is(err, errConfigOptionMissing) {
		vaultConfig[api.EnvVaultNamespace] = vaultAuthNamespace
	}

	verifyCA := strconv.FormatBool(vaultDefaultCAVerify) // optional
	err = setConfigString(&verifyCA, config, "vaultCAVerify")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	if firstInit || !errors.Is(err, errConfigOptionMissing) {
		var vaultCAVerify bool
		vaultCAVerify, err = strconv.ParseBool(verifyCA)
		if err != nil {
			return fmt.Errorf("failed to parse 'vaultCAVerify': %w", err)
		}
		vaultConfig[api.EnvVaultInsecure] = strconv.FormatBool(!vaultCAVerify)
	}

	vaultCAFromSecret := "" // optional
	err = setConfigString(&vaultCAFromSecret, config, "vaultCAFromSecret")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	// update the existing config only if no config is available yet
	if vc.keyContext != nil {
		for key, value := range keyContext {
			vc.keyContext[key] = value
		}
	} else {
		vc.keyContext = keyContext
	}
	if vc.vaultConfig != nil {
		for key, value := range vaultConfig {
			vc.vaultConfig[key] = value
		}
	} else {
		vc.vaultConfig = vaultConfig
	}

	return nil
}

// initCertificates sets VAULT_* environment variables in the vc.vaultConfig map,
// these settings will be used when connecting to the Vault service with
// vc.connectVault().
func (vc *vaultConnection) initCertificates(config map[string]interface{}, secrets map[string]string) error {
	vaultConfig := make(map[string]interface{})

	vaultCAFromSecret := "" // optional
	err := setConfigString(&vaultCAFromSecret, config, "vaultCAFromSecret")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// ignore errConfigOptionMissing, no default was set
	if vaultCAFromSecret != "" {
		caPEM, ok := secrets[vaultCAFromSecret]
		if !ok {
			return fmt.Errorf("missing vault CA in secret %s", vaultCAFromSecret)
		}

		vaultConfig[api.EnvVaultCACert], err = createTempFile("vault-ca-cert", []byte(caPEM))
		if err != nil {
			return fmt.Errorf("failed to create temporary file for Vault CA: %w", err)
		}
		// update the existing config
		for key, value := range vaultConfig {
			vc.vaultConfig[key] = value
		}
	}

	return nil
}

// connectVault creates a new connection to Vault. This should be called after
// filling vc.vaultConfig.
func (vc *vaultConnection) connectVault() error {
	v, err := vault.New(vc.vaultConfig)
	if err != nil {
		return fmt.Errorf("failed connecting to Vault: %w", err)
	}
	vc.secrets = v

	return nil
}

// Destroy frees allocated resources. For a vaultConnection that means removing
// the created temporary files.
func (vc *vaultConnection) Destroy() {
	if vc.vaultConfig != nil {
		tmpFile, ok := vc.vaultConfig[api.EnvVaultCACert]
		if ok {
			// ignore error on failure to remove tmpfile (gosec complains)
			//nolint:forcetypeassert // ignore error on failure to remove tmpfile
			_ = os.Remove(tmpFile.(string))
		}
	}
}

// getDeleteKeyContext creates a new KeyContext that has an optional value set
// to destroy the contents of secrets. This is configurable with the
// `vaultDestroyKeys` configuration parameter.
//
// Setting the option `DestroySecret` option in the KeyContext while creating
// new keys, causes failures, so the option only needs to be set in the
// RemoveDEK() calls.
func (vc *vaultConnection) getDeleteKeyContext() map[string]string {
	keyContext := map[string]string{}
	for k, v := range vc.keyContext {
		keyContext[k] = v
	}
	if vc.vaultDestroyKeys {
		keyContext[loss.DestroySecret] = vaultDefaultDestroyKeys
	}

	return keyContext
}

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeVault,
	Initializer: initVaultKMS,
})

// InitVaultKMS returns an interface to HashiCorp Vault KMS.
func initVaultKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	kms := &vaultKMS{}
	err := kms.initConnection(args.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault connection: %w", err)
	}

	err = kms.initCertificates(args.Config, args.Secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault certificates: %w", err)
	}

	vaultAuthPath := vaultDefaultAuthPath
	err = setConfigString(&vaultAuthPath, args.Config, "vaultAuthPath")
	if err != nil {
		return nil, err
	}

	kms.vaultConfig[vault.AuthMountPath], err = detectAuthMountPath(vaultAuthPath)
	if err != nil {
		return nil, fmt.Errorf("failed to set \"vaultAuthPath\" in Vault config: %w", err)
	}

	vaultRole := vaultDefaultRole
	err = setConfigString(&vaultRole, args.Config, "vaultRole")
	if err != nil {
		return nil, err
	}
	kms.vaultConfig[vault.AuthKubernetesRole] = vaultRole

	// vault.VaultBackendPathKey is "secret/" by default, use vaultPassphraseRoot if configured
	vaultPassphraseRoot := ""
	err = setConfigString(&vaultPassphraseRoot, args.Config, "vaultPassphraseRoot")
	if err == nil {
		// the old example did have "/v1/secret/", convert that format
		if strings.HasPrefix(vaultPassphraseRoot, "/v1/") {
			kms.vaultConfig[vault.VaultBackendPathKey] = strings.TrimPrefix(vaultPassphraseRoot, "/v1/")
		} else {
			kms.vaultConfig[vault.VaultBackendPathKey] = vaultPassphraseRoot
		}
	} else if !errors.Is(err, errConfigOptionMissing) {
		return nil, err
	}

	kms.vaultPassphrasePath = vaultDefaultPassphrasePath
	err = setConfigString(&kms.vaultPassphrasePath, args.Config, "vaultPassphrasePath")
	if err != nil {
		return nil, err
	}

	// FIXME: vault.AuthKubernetesTokenPath is not enough? EnvVaultToken needs to be set?
	kms.vaultConfig[vault.AuthMethod] = vault.AuthMethodKubernetes
	kms.vaultConfig[vault.AuthKubernetesTokenPath] = serviceAccountTokenPath

	err = kms.connectVault()
	if err != nil {
		return nil, err
	}

	return kms, nil
}

// FetchDEK returns passphrase from Vault. The passphrase is stored in a
// data.data.passphrase structure.
func (kms *vaultKMS) FetchDEK(key string) (string, error) {
	s, err := kms.secrets.GetSecret(filepath.Join(kms.vaultPassphrasePath, key), kms.keyContext)
	if err != nil {
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

// StoreDEK saves new passphrase in Vault.
func (kms *vaultKMS) StoreDEK(key, value string) error {
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

// RemoveDEK deletes passphrase from Vault.
func (kms *vaultKMS) RemoveDEK(key string) error {
	pathKey := filepath.Join(kms.vaultPassphrasePath, key)
	err := kms.secrets.DeleteSecret(pathKey, kms.getDeleteKeyContext())
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
// the filename (see os.CreateTemp for details).
func createTempFile(pattern string, data []byte) (string, error) {
	t, err := os.CreateTemp("", pattern)
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
