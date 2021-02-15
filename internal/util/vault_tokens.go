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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/vault/api"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
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

type standardVault struct {
	KmsPROVIDER        string `json:"KMS_PROVIDER"`
	VaultADDR          string `json:"VAULT_ADDR"`
	VaultBackendPath   string `json:"VAULT_BACKEND_PATH"`
	VaultCACert        string `json:"VAULT_CACERT"`
	VaultTLSServerName string `json:"VAULT_TLS_SERVER_NAME"`
	VaultClientCert    string `json:"VAULT_CLIENT_CERT"`
	VaultClientKey     string `json:"VAULT_CLIENT_KEY"`
	VaultNamespace     string `json:"VAULT_NAMESPACE"`
	VaultSkipVerify    string `json:"VAULT_SKIP_VERIFY"`
}

type vaultTokenConf struct {
	EncryptionKMSType            string `json:"encryptionKMSType"`
	VaultAddress                 string `json:"vaultAddress"`
	VaultBackendPath             string `json:"vaultBackendPath"`
	VaultCAFromSecret            string `json:"vaultCAFromSecret"`
	VaultTLSServerName           string `json:"vaultTLSServerName"`
	VaultClientCertFromSecret    string `json:"vaultClientCertFromSecret"`
	VaultClientCertKeyFromSecret string `json:"vaultClientCertKeyFromSecret"`
	VaultNamespace               string `json:"vaultNamespace"`
	VaultCAVerify                string `json:"vaultCAVerify"`
}

func (v *vaultTokenConf) convertStdVaultToCSIConfig(s *standardVault) {
	v.EncryptionKMSType = s.KmsPROVIDER
	v.VaultAddress = s.VaultADDR
	v.VaultBackendPath = s.VaultBackendPath
	v.VaultCAFromSecret = s.VaultCACert
	v.VaultClientCertFromSecret = s.VaultClientCert
	v.VaultClientCertKeyFromSecret = s.VaultClientKey
	v.VaultNamespace = s.VaultNamespace
	v.VaultTLSServerName = s.VaultTLSServerName

	// by default the CA should get verified, only when VaultSkipVerify is
	// set, verification should be disabled
	v.VaultCAVerify = "true"
	verify, err := strconv.ParseBool(s.VaultSkipVerify)
	if err == nil {
		v.VaultCAVerify = strconv.FormatBool(!verify)
	}
}

// getVaultConfiguration fetches the vault configuration from the kubernetes
// configmap and converts the standard vault configuration (see json tag of
// standardVault structure) to the CSI vault configuration.
func getVaultConfiguration(namespace, name string) (map[string]interface{}, error) {
	c := NewK8sClient()
	cm, err := c.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	config := make(map[string]interface{})
	// convert the standard vault configuration to CSI vault configuration and
	// store it in the map that CSI expects
	for k, v := range cm.Data {
		sv := &standardVault{}
		err = json.Unmarshal([]byte(v), sv)
		if err != nil {
			return nil, fmt.Errorf("failed to Unmarshal the vault configuration for %q: %w", k, err)
		}
		vc := vaultTokenConf{}
		vc.convertStdVaultToCSIConfig(sv)
		data, err := json.Marshal(vc)
		if err != nil {
			return nil, fmt.Errorf("failed to Marshal the CSI vault configuration for %q: %w", k, err)
		}
		jsonMap := make(map[string]interface{})
		err = json.Unmarshal(data, &jsonMap)
		if err != nil {
			return nil, fmt.Errorf("failed to Unmarshal the CSI vault configuration for %q: %w", k, err)
		}
		config[k] = jsonMap
	}
	return config, nil
}

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
        "vaultClientCertFromSecret": "vault-client-cert",
        "vaultClientCertKeyFromSecret": "vault-client-cert-key",
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
func InitVaultTokensKMS(tenant, kmsID string, config map[string]interface{}) (EncryptionKMS, error) {
	kms := &VaultTokensKMS{}
	kms.Tenant = tenant
	err := kms.initConnection(kmsID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault connection: %w", err)
	}

	// set default values for optional config options
	kms.ConfigName = vaultTokensDefaultConfigName
	kms.TokenName = vaultTokensDefaultTokenName

	err = kms.parseConfig(config)
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
					err = kms.parseConfig(tenantConfig)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		err = kms.parseTenantConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to parse config for tenant: %w", err)
		}
	}

	// fetch the Vault Token from the Secret (TokenName) in the Kubernetes
	// Namespace (tenant)
	kms.vaultConfig[api.EnvVaultToken], err = getToken(tenant, kms.TokenName)
	if err != nil {
		return nil, fmt.Errorf("failed fetching token from %s/%s: %w", tenant, kms.TokenName, err)
	}

	err = kms.initCertificates(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault certificates: %w", err)
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
func (kms *VaultTokensKMS) parseConfig(config map[string]interface{}) error {
	err := kms.initConnection(kms.EncryptionKMSID, config)
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

// initCertificates updates the kms.vaultConfig with the options from config
// it calls the kubernetes secrets and get the required data.

// nolint:gocyclo // iterating through many config options, not complex at all.
func (kms *VaultTokensKMS) initCertificates(config map[string]interface{}) error {
	vaultConfig := make(map[string]interface{})

	csiNamespace := os.Getenv("POD_NAMESPACE")
	vaultCAFromSecret := "" // optional
	err := setConfigString(&vaultCAFromSecret, config, "vaultCAFromSecret")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// ignore errConfigOptionMissing, no default was set
	if vaultCAFromSecret != "" {
		cert, cErr := getCertificate(kms.Tenant, vaultCAFromSecret, "cert")
		if cErr != nil && !apierrs.IsNotFound(cErr) {
			return fmt.Errorf("failed to get CA certificate from secret %s: %w", vaultCAFromSecret, cErr)
		}
		// if the certificate is not present in tenant namespace get it from
		// cephcsi pod namespace
		if apierrs.IsNotFound(cErr) {
			cert, cErr = getCertificate(csiNamespace, vaultCAFromSecret, "cert")
			if cErr != nil {
				return fmt.Errorf("failed to get CA certificate from secret %s: %w", vaultCAFromSecret, cErr)
			}
		}
		vaultConfig[api.EnvVaultCACert], err = createTempFile("vault-ca-cert", []byte(cert))
		if err != nil {
			return fmt.Errorf("failed to create temporary file for Vault CA: %w", err)
		}
	}

	vaultClientCertFromSecret := "" // optional
	err = setConfigString(&vaultClientCertFromSecret, config, "vaultClientCertFromSecret")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// ignore errConfigOptionMissing, no default was set
	if vaultClientCertFromSecret != "" {
		cert, cErr := getCertificate(kms.Tenant, vaultClientCertFromSecret, "cert")
		if cErr != nil && !apierrs.IsNotFound(cErr) {
			return fmt.Errorf("failed to get client certificate from secret %s: %w", vaultClientCertFromSecret, cErr)
		}
		// if the certificate is not present in tenant namespace get it from
		// cephcsi pod namespace
		if apierrs.IsNotFound(cErr) {
			cert, cErr = getCertificate(csiNamespace, vaultClientCertFromSecret, "cert")
			if cErr != nil {
				return fmt.Errorf("failed to get client certificate from secret %s: %w", vaultCAFromSecret, cErr)
			}
		}
		vaultConfig[api.EnvVaultClientCert], err = createTempFile("vault-ca-cert", []byte(cert))
		if err != nil {
			return fmt.Errorf("failed to create temporary file for Vault client certificate: %w", err)
		}
	}

	vaultClientCertKeyFromSecret := "" // optional
	err = setConfigString(&vaultClientCertKeyFromSecret, config, "vaultClientCertKeyFromSecret")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	// ignore errConfigOptionMissing, no default was set
	if vaultClientCertKeyFromSecret != "" {
		certKey, err := getCertificate(kms.Tenant, vaultClientCertKeyFromSecret, "key")
		if err != nil && !apierrs.IsNotFound(err) {
			return fmt.Errorf("failed to get client certificate key from secret %s: %w", vaultClientCertKeyFromSecret, err)
		}
		// if the certificate is not present in tenant namespace get it from
		// cephcsi pod namespace
		if apierrs.IsNotFound(err) {
			certKey, err = getCertificate(csiNamespace, vaultClientCertKeyFromSecret, "key")
			if err != nil {
				return fmt.Errorf("failed to get client certificate key from secret %s: %w", vaultCAFromSecret, err)
			}
		}
		vaultConfig[api.EnvVaultClientKey], err = createTempFile("vault-client-cert-key", []byte(certKey))
		if err != nil {
			return fmt.Errorf("failed to create temporary file for Vault client cert key: %w", err)
		}
	}

	for key, value := range vaultConfig {
		kms.vaultConfig[key] = value
	}

	return nil
}

// GetPassphrase returns passphrase from Vault. The passphrase is stored in a
// data.data.passphrase structure.
func (kms *VaultTokensKMS) GetPassphrase(key string) (string, error) {
	s, err := kms.secrets.GetSecret(key, kms.keyContext)
	if err != nil {
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

func getCertificate(tenant, secretName, key string) (string, error) {
	c := NewK8sClient()
	secret, err := c.CoreV1().Secrets(tenant).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	cert, ok := secret.Data[key]
	if !ok {
		return "", errors.New("failed to parse certificates")
	}

	return string(cert), nil
}

// isTenantConfigOption return true if a tenant may (re)configure the option in
// their own ConfigMap, false otherwise.
func isTenantConfigOption(opt string) bool {
	switch opt {
	case "vaultAddress":
	case "vaultBackendPath":
	case "vaultTLSServerName":
	case "vaultCAFromSecret":
	case "vaultCAVerify":
	default:
		return false
	}

	return true
}

// parseTenantConfig gets the optional ConfigMap from the Tenants namespace,
// and applies the allowable options (see isTenantConfigOption) to the KMS
// configuration.
func (kms *VaultTokensKMS) parseTenantConfig() error {
	if kms.Tenant == "" || kms.ConfigName == "" {
		return nil
	}

	// fetch the ConfigMap from the tanants namespace
	c := NewK8sClient()
	cm, err := c.CoreV1().ConfigMaps(kms.Tenant).Get(context.TODO(),
		kms.ConfigName, metav1.GetOptions{})
	if apierrs.IsNotFound(err) {
		// the tenant did not (re)configure any options
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get config (%s) for tenant (%s): %w",
			kms.ConfigName, kms.Tenant, err)
	}

	// create a new map with config options, but only include the options
	// that a tenant may (re)configure
	config := make(map[string]interface{})
	for k, v := range cm.Data {
		if isTenantConfigOption(k) {
			config[k] = v
		} // else: silently ignore the option
	}
	if len(config) == 0 {
		// no options configured by the tenant
		return nil
	}

	// apply the configuration options from the tenant
	err = kms.parseConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config (%s) for tenant (%s): %w",
			kms.ConfigName, kms.Tenant, err)
	}

	return nil
}
