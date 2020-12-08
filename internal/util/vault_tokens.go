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
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/vault/api"
	loss "github.com/libopenstorage/secrets"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeVaultTokens = "vaulttokens"

	// vaultTokensDefaultConfigName is the name of the Kubernetes ConfigMap
	// that contains the Vault connection configuration for the tenant.
	// This ConfigMap is located in the Kubernetes Namespace where the
	// tenant created the PVC.
	//
	// #nosec:G101, value not credential, just references token.
	vaultTokensDefaultConfigName = "ceph-csi-kms-config"

	// vaultTokensDefaultTokenName is the name of the Kubernetes Secret
	// that contains the Vault Token for the tenant.  This Secret is
	// located in the Kubernetes Namespace where the tenant created the
	// PVC.
	//
	// #nosec:G101, value not credential, just references token.
	vaultTokensDefaultTokenName = "ceph-csi-kms-token"

	// vaultTokenSecretKey refers to the key in the Kubernetes Secret that
	// contains the VAULT_TOKEN.
	vaultTokenSecretKey = "token"
)

/*
VaultTokens represents a Hashicorp Vault KMS configuration that provides a
Token per tenant.

Example JSON structure in the KMS config is,
{
    "vault-with-tokens": {
        "encryptionKMSType": "vaulttokens",
        "vaultAddress": "http://vault.default.svc.cluster.local:8200",
        "vaultBackendPath": "secret/",
        "vaultTLSServerName": "vault.default.svc.cluster.local",
        "vaultCAFromSecret": "vault-ca",
        "vaultCAVerify": "false",
        "tenantConfigName": "ceph-csi-kms-config",
        "tenantTokenName": "ceph-csi-kms-token",
        "tenants": {
            "my-app": {
                "vaultAddress": "https://vault.example.com",
                "vaultCAVerify": "true"
            },
            "an-other-app": {
                "tenantTokenName": "storage-encryption-token"
            }
	},
	...
}.
*/
type VaultTokensKMS struct {
	vaultConnection

	// Tenant is the name of the owner of the volume
	Tenant string
	// ConfigName is the name of the ConfigMap in the Tenants Kubernetes Namespace
	ConfigName string
	// TokenName is the name of the Secret in the Tenants Kubernetes Namespace
	TokenName string
}

// InitVaultTokensKMS returns an interface to HashiCorp Vault KMS.
func InitVaultTokensKMS(tenant, kmsID string, config map[string]interface{}, secrets map[string]string) (EncryptionKMS, error) {
	kms := &VaultTokensKMS{}
	err := kms.initConnection(kmsID, config, secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault connection: %w", err)
	}

	// set default values for optional config options
	kms.ConfigName = vaultTokensDefaultConfigName
	kms.TokenName = vaultTokensDefaultTokenName

	err = kms.parseConfig(config, secrets)
	if err != nil {
		return nil, err
	}

	// fetch the configuration for the tenant
	if tenant != "" {
		tenantsMap, ok := config["tenants"]
		if ok {
			// tenants is a map per tenant, containing key/values
			tenants, ok := tenantsMap.(map[string]map[string]interface{})
			if ok {
				// get the map for the tenant of the current operation
				tenantConfig, ok := tenants[tenant]
				if ok {
					// override connection details from the tenant
					err = kms.parseConfig(tenantConfig, secrets)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	// fetch the Vault Token from the Secret (TokenName) in the Kubernetes
	// Namespace (tenant)
	kms.vaultConfig[api.EnvVaultToken], err = getToken(tenant, kms.TokenName)
	if err != nil {
		return nil, fmt.Errorf("failed fetching token from %s/%s: %w", tenant, kms.TokenName, err)
	}

	// connect to the Vault service
	err = kms.connectVault()
	if err != nil {
		return nil, err
	}

	return kms, nil
}

// parseConfig updates the kms.vaultConfig with the options from config and
// secrets. This method can be called multiple times, i.e. to override
// configuration options from tenants.
func (kms *VaultTokensKMS) parseConfig(config map[string]interface{}, secrets map[string]string) error {
	err := kms.initConnection(kms.EncryptionKMSID, config, secrets)
	if err != nil {
		return err
	}

	err = setConfigString(&kms.ConfigName, config, "tenantConfigName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	err = setConfigString(&kms.TokenName, config, "tenantTokenName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	return nil
}

// GetPassphrase returns passphrase from Vault. The passphrase is stored in a
// data.data.passphrase structure.
func (kms *VaultTokensKMS) GetPassphrase(key string) (string, error) {
	s, err := kms.secrets.GetSecret(key, kms.keyContext)
	if errors.Is(err, loss.ErrInvalidSecretId) {
		return "", MissingPassphrase{err}
	} else if err != nil {
		return "", err
	}

	data, ok := s["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed parsing data for get passphrase request for %s", key)
	}
	passphrase, ok := data["passphrase"].(string)
	if !ok {
		return "", fmt.Errorf("failed parsing passphrase for get passphrase request for %s", key)
	}

	return passphrase, nil
}

// SavePassphrase saves new passphrase in Vault.
func (kms *VaultTokensKMS) SavePassphrase(key, value string) error {
	data := map[string]interface{}{
		"data": map[string]string{
			"passphrase": value,
		},
	}

	err := kms.secrets.PutSecret(key, data, kms.keyContext)
	if err != nil {
		return fmt.Errorf("saving passphrase at %s request to vault failed: %w", key, err)
	}

	return nil
}

// DeletePassphrase deletes passphrase from Vault.
func (kms *VaultTokensKMS) DeletePassphrase(key string) error {
	err := kms.secrets.DeleteSecret(key, kms.keyContext)
	if err != nil {
		return fmt.Errorf("delete passphrase at %s request to vault failed: %w", key, err)
	}

	return nil
}

func getToken(tenant, tokenName string) (string, error) {
	c := NewK8sClient()
	secret, err := c.CoreV1().Secrets(tenant).Get(context.TODO(), tokenName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	token, ok := secret.Data[vaultTokenSecretKey]
	if !ok {
		return "", errors.New("failed to parse token")
	}

	return string(token), nil
}
