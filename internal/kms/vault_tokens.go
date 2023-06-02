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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	"github.com/hashicorp/vault/api"
	loss "github.com/libopenstorage/secrets"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	VaultBackend       string `json:"VAULT_BACKEND"`
	VaultBackendPath   string `json:"VAULT_BACKEND_PATH"`
	VaultDestroyKeys   string `json:"VAULT_DESTROY_KEYS"`
	VaultCACert        string `json:"VAULT_CACERT"`
	VaultTLSServerName string `json:"VAULT_TLS_SERVER_NAME"`
	VaultClientCert    string `json:"VAULT_CLIENT_CERT"`
	VaultClientKey     string `json:"VAULT_CLIENT_KEY"`
	VaultAuthNamespace string `json:"VAULT_AUTH_NAMESPACE"`
	VaultNamespace     string `json:"VAULT_NAMESPACE"`
	VaultSkipVerify    string `json:"VAULT_SKIP_VERIFY"`
}

type vaultTokenConf struct {
	EncryptionKMSType            string `json:"encryptionKMSType"`
	VaultAddress                 string `json:"vaultAddress"`
	VaultBackend                 string `json:"vaultBackend"`
	VaultBackendPath             string `json:"vaultBackendPath"`
	VaultDestroyKeys             string `json:"vaultDestroyKeys"`
	VaultCAFromSecret            string `json:"vaultCAFromSecret"`
	VaultTLSServerName           string `json:"vaultTLSServerName"`
	VaultClientCertFromSecret    string `json:"vaultClientCertFromSecret"`
	VaultClientCertKeyFromSecret string `json:"vaultClientCertKeyFromSecret"`
	VaultAuthNamespace           string `json:"vaultAuthNamespace"`
	VaultNamespace               string `json:"vaultNamespace"`
	VaultCAVerify                string `json:"vaultCAVerify"`
}

func (v *vaultTokenConf) convertStdVaultToCSIConfig(s *standardVault) {
	v.EncryptionKMSType = s.KmsPROVIDER
	v.VaultAddress = s.VaultADDR
	v.VaultBackend = s.VaultBackend
	v.VaultBackendPath = s.VaultBackendPath
	v.VaultDestroyKeys = s.VaultDestroyKeys
	v.VaultCAFromSecret = s.VaultCACert
	v.VaultClientCertFromSecret = s.VaultClientCert
	v.VaultClientCertKeyFromSecret = s.VaultClientKey
	v.VaultAuthNamespace = s.VaultAuthNamespace
	v.VaultNamespace = s.VaultNamespace
	v.VaultTLSServerName = s.VaultTLSServerName

	// by default the CA should get verified, only when VaultSkipVerify is
	// set, verification should be disabled
	verify, err := strconv.ParseBool(s.VaultSkipVerify)
	if err == nil {
		v.VaultCAVerify = strconv.FormatBool(!verify)
	}
}

// convertConfig takes the keys/values in standard Vault environment variable
// format, and converts them to the format that is used in the configuration
// file.
// This uses JSON marshaling and unmarshalling to map the Vault environment
// configuration into bytes, then in the standardVault struct, which is passed
// through convertStdVaultToCSIConfig before converting back to a
// map[string]interface{} configuration.
//
// FIXME: this can surely be simplified?!
func transformConfig(svMap map[string]interface{}) (map[string]interface{}, error) {
	// convert the map to JSON
	data, err := json.Marshal(svMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert config %T to JSON: %w", svMap, err)
	}

	// convert the JSON back to a standardVault struct, default values are
	// set in case the configuration does not provide all options
	sv := &standardVault{
		VaultDestroyKeys: vaultDefaultDestroyKeys,
		VaultNamespace:   vaultDefaultNamespace,
		VaultSkipVerify:  strconv.FormatBool(!vaultDefaultCAVerify),
	}

	err = json.Unmarshal(data, sv)
	if err != nil {
		return nil, fmt.Errorf("failed to Unmarshal the vault configuration: %w", err)
	}

	// convert the standardVault struct to a vaultTokenConf struct
	vc := vaultTokenConf{}
	vc.convertStdVaultToCSIConfig(sv)
	data, err = json.Marshal(vc)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal the CSI vault configuration: %w", err)
	}

	// convert the vaultTokenConf struct to a map[string]interface{}
	jsonMap := make(map[string]interface{})
	err = json.Unmarshal(data, &jsonMap)
	if err != nil {
		return nil, fmt.Errorf("failed to Unmarshal the CSI vault configuration: %w", err)
	}

	return jsonMap, nil
}

/*
VaultTokens represents a Hashicorp Vault KMS configuration that provides a
Token per tenant.

Example JSON structure in the KMS config is,

	{
	    "vault-with-tokens": {
	        "encryptionKMSType": "vaulttokens",
	        "vaultAddress": "http://vault.default.svc.cluster.local:8200",
	        "vaultBackend": "kv-v2",
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
type vaultTenantConnection struct {
	vaultConnection
	integratedDEK

	client *kubernetes.Clientset

	// Tenant is the name of the owner of the volume
	Tenant string
	// ConfigName is the name of the ConfigMap in the Tenants Kubernetes Namespace
	ConfigName string

	// tenantConfigOptionFilter ise used to filter configuration options
	// for the KMS that are provided by the ConfigMap in the Tenants
	// Namespace. It defaults to isTenantConfigOption() as setup by the
	// init() function.
	tenantConfigOptionFilter func(string) bool
}

type vaultTokensKMS struct {
	vaultTenantConnection

	// TokenName is the name of the Secret in the Tenants Kubernetes Namespace
	TokenName string
}

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeVaultTokens,
	Initializer: initVaultTokensKMS,
})

// InitVaultTokensKMS returns an interface to HashiCorp Vault KMS.
func initVaultTokensKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	var err error

	config := args.Config
	if _, ok := config[kmsProviderKey]; ok {
		// configuration comes from the ConfigMap, needs to be
		// converted to vaultTokenConf type
		config, err = transformConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to convert configuration: %w", err)
		}
	}

	kms := &vaultTokensKMS{}
	kms.vaultTenantConnection.init()
	err = kms.initConnection(config)
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

	err = kms.setTokenName(config)
	if err != nil && !errors.Is(err, errConfigOptionMissing) {
		return nil, fmt.Errorf("failed to set the TokenName from global config %q: %w",
			kms.ConfigName, err)
	}

	// fetch the configuration for the tenant
	if args.Tenant != "" {
		err = kms.configureTenant(config, args.Tenant)
		if err != nil {
			return nil, err
		}
	}

	// fetch the Vault Token from the Secret (TokenName) in the Kubernetes
	// Namespace (tenant)
	kms.vaultConfig[api.EnvVaultToken], err = kms.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed fetching token from %s/%s: %w", args.Tenant, kms.TokenName, err)
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

func (kms *vaultTokensKMS) configureTenant(config map[string]interface{}, tenant string) error {
	kms.Tenant = tenant
	tenantConfig, found := fetchTenantConfig(config, tenant)
	if found {
		// override connection details from the tenant
		err := kms.parseConfig(tenantConfig)
		if err != nil {
			return err
		}

		err = kms.setTokenName(tenantConfig)
		if err != nil {
			return fmt.Errorf("failed to set the TokenName for tenant (%s): %w",
				kms.Tenant, err)
		}
	}

	// get the ConfigMap from the Tenant and apply the options
	tenantConfig, err := kms.parseTenantConfig()
	if err != nil {
		return fmt.Errorf("failed to parse config for tenant: %w", err)
	} else if tenantConfig != nil {
		err = kms.parseConfig(tenantConfig)
		if err != nil {
			return fmt.Errorf("failed to parse config (%s) for tenant (%s): %w",
				kms.ConfigName, kms.Tenant, err)
		}

		err = kms.setTokenName(tenantConfig)
		if err != nil {
			return fmt.Errorf("failed to set the TokenName from %s for tenant (%s): %w",
				kms.ConfigName, kms.Tenant, err)
		}
	}

	return nil
}

func (vtc *vaultTenantConnection) init() {
	vtc.tenantConfigOptionFilter = isTenantConfigOption
}

// parseConfig updates the kms.vaultConfig with the options from config and
// secrets. This method can be called multiple times, i.e. to override
// configuration options from tenants.
func (vtc *vaultTenantConnection) parseConfig(config map[string]interface{}) error {
	err := vtc.initConnection(config)
	if err != nil {
		return err
	}

	err = setConfigString(&vtc.ConfigName, config, "tenantConfigName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	return nil
}

// setTokenName updates the kms.TokenName with the options from config. This
// method can be called multiple times, i.e. to override configuration options
// from tenants.
func (kms *vaultTokensKMS) setTokenName(config map[string]interface{}) error {
	err := setConfigString(&kms.TokenName, config, "tenantTokenName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	return nil
}

// initCertificates updates the kms.vaultConfig with the options from config
// it calls the kubernetes secrets and get the required data.

//nolint:gocyclo,cyclop // iterating through many config options, not complex at all.
func (vtc *vaultTenantConnection) initCertificates(config map[string]interface{}) error {
	vaultConfig := make(map[string]interface{})

	csiNamespace := os.Getenv("POD_NAMESPACE")
	vaultCAFromSecret := "" // optional
	err := setConfigString(&vaultCAFromSecret, config, "vaultCAFromSecret")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}
	// ignore errConfigOptionMissing, no default was set
	if vaultCAFromSecret != "" {
		cert, cErr := vtc.getCertificate(vtc.Tenant, vaultCAFromSecret, "cert")
		if cErr != nil && !apierrs.IsNotFound(cErr) {
			return fmt.Errorf("failed to get CA certificate from secret %s: %w", vaultCAFromSecret, cErr)
		}
		// if the certificate is not present in tenant namespace get it from
		// cephcsi pod namespace
		if apierrs.IsNotFound(cErr) {
			cert, cErr = vtc.getCertificate(csiNamespace, vaultCAFromSecret, "cert")
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
		cert, cErr := vtc.getCertificate(vtc.Tenant, vaultClientCertFromSecret, "cert")
		if cErr != nil && !apierrs.IsNotFound(cErr) {
			return fmt.Errorf("failed to get client certificate from secret %s: %w", vaultClientCertFromSecret, cErr)
		}
		// if the certificate is not present in tenant namespace get it from
		// cephcsi pod namespace
		if apierrs.IsNotFound(cErr) {
			cert, cErr = vtc.getCertificate(csiNamespace, vaultClientCertFromSecret, "cert")
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
		certKey, err := vtc.getCertificate(vtc.Tenant, vaultClientCertKeyFromSecret, "key")
		if err != nil && !apierrs.IsNotFound(err) {
			return fmt.Errorf(
				"failed to get client certificate key from secret %s: %w",
				vaultClientCertKeyFromSecret,
				err)
		}
		// if the certificate is not present in tenant namespace get it from
		// cephcsi pod namespace
		if apierrs.IsNotFound(err) {
			certKey, err = vtc.getCertificate(csiNamespace, vaultClientCertKeyFromSecret, "key")
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
		vtc.vaultConfig[key] = value
	}

	return nil
}

func (vtc *vaultTenantConnection) getK8sClient() (*kubernetes.Clientset, error) {
	if vtc.client == nil {
		client, err := k8s.NewK8sClient()
		if err != nil {
			return nil, err
		}
		vtc.client = client
	}

	return vtc.client, nil
}

// FetchDEK returns passphrase from Vault. The passphrase is stored in a
// data.data.passphrase structure.
func (vtc *vaultTenantConnection) FetchDEK(key string) (string, error) {
	s, err := vtc.secrets.GetSecret(key, vtc.keyContext)
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

// StoreDEK saves new passphrase in Vault.
func (vtc *vaultTenantConnection) StoreDEK(key, value string) error {
	data := map[string]interface{}{
		"data": map[string]string{
			"passphrase": value,
		},
	}

	err := vtc.secrets.PutSecret(key, data, vtc.keyContext)
	if err != nil {
		return fmt.Errorf("saving passphrase at %s request to vault failed: %w", key, err)
	}

	return nil
}

// RemoveDEK deletes passphrase from Vault.
func (vtc *vaultTenantConnection) RemoveDEK(key string) error {
	err := vtc.secrets.DeleteSecret(key, vtc.getDeleteKeyContext())
	if err != nil {
		return fmt.Errorf("delete passphrase at %s request to vault failed: %w", key, err)
	}

	return nil
}

func (kms *vaultTokensKMS) getToken() (string, error) {
	c, err := kms.getK8sClient()
	if err != nil {
		return "", err
	}

	secret, err := c.CoreV1().Secrets(kms.Tenant).Get(context.TODO(), kms.TokenName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	token, ok := secret.Data[vaultTokenSecretKey]
	if !ok {
		return "", errors.New("failed to parse token")
	}

	return string(token), nil
}

func (vtc *vaultTenantConnection) getCertificate(tenant, secretName, key string) (string, error) {
	c, err := vtc.getK8sClient()
	if err != nil {
		return "", err
	}

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
	case "vaultBackend":
	case "vaultBackendPath":
	case "vaultAuthNamespace":
	case "vaultNamespace":
	case "vaultDestroyKeys":
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
func (vtc *vaultTenantConnection) parseTenantConfig() (map[string]interface{}, error) {
	if vtc.Tenant == "" || vtc.ConfigName == "" {
		return nil, nil
	}

	// fetch the ConfigMap from the tenants namespace
	c, err := vtc.getK8sClient()
	if err != nil {
		return nil, err
	}

	cm, err := c.CoreV1().ConfigMaps(vtc.Tenant).Get(context.TODO(),
		vtc.ConfigName, metav1.GetOptions{})
	if apierrs.IsNotFound(err) {
		// the tenant did not (re)configure any options
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get config (%s) for tenant (%s): %w",
			vtc.ConfigName, vtc.Tenant, err)
	}

	// create a new map with config options, but only include the options
	// that a tenant may (re)configure
	config := make(map[string]interface{})
	for k, v := range cm.Data {
		if vtc.tenantConfigOptionFilter(k) {
			config[k] = v
		} // else: silently ignore the option
	}
	if len(config) == 0 {
		// no options configured by the tenant
		return nil, nil
	}

	vtc.setTenantAuthNamespace(config)

	return config, nil
}

// setTenantAuthNamespace configures the vaultAuthNamespace for the tenant.
// vaultAuthNamespace defaults to vaultNamespace from the global configuration,
// even if the tenant has vaultNamespace configured. Users expect to have the
// vaultAuthNamespace updated when they configure vaultNamespace, if
// vaultAuthNamespace was not explicitly set in the global configuration.
func (vtc *vaultTenantConnection) setTenantAuthNamespace(tenantConfig map[string]interface{}) {
	vaultAuthNamespace, ok := vtc.keyContext[loss.KeyVaultNamespace]
	if !ok {
		// nothing to do, global connection config does not have the
		// vaultAuthNamespace set
		return
	}

	vaultNamespace, ok := vtc.vaultConfig[api.EnvVaultNamespace]
	if !ok {
		// nothing to do, global connection config does not have the
		// vaultNamespace set, not overriding vaultAuthNamespace with
		// vaultNamespace from the tenant
		return
	}

	if vaultAuthNamespace != vaultNamespace {
		// vaultAuthNamespace and vaultNamespace have been configured
		// differently in the global connection. Not going to override
		// those pre-defined options if the tenantConfig does not have
		// them set.
		return
	}

	// if we reached here, we need to make sure that the vaultAuthNamespace
	// gets configured for the tenant, in case the tenant config has
	// vaultNamespace set

	_, ok = tenantConfig["vaultAuthNamespace"]
	if ok {
		// the tenant already has vaultAuthNamespace configured, no
		// action needed
		return
	}

	tenantNamespace, ok := tenantConfig["vaultNamespace"]
	if !ok {
		// the tenant does not have vaultNamespace configured, no need
		// to set vaultAuthNamespace either
		return
	}

	// the tenant has vaultNamespace configured, use that for
	// vaultAuthNamespace as well
	tenantConfig["vaultAuthNamespace"] = tenantNamespace
}

// fetchTenantConfig fetches the configuration for the tenant if it exists.
func fetchTenantConfig(config map[string]interface{}, tenant string) (map[string]interface{}, bool) {
	tenantsMap, ok := config["tenants"]
	if !ok {
		return nil, false
	}
	// tenants is a map per tenant, containing key/values
	tenants, ok := tenantsMap.(map[string]map[string]interface{})
	if !ok {
		return nil, false
	}
	// get the map for the tenant of the current operation
	tenantConfig, ok := tenants[tenant]
	if !ok {
		return nil, false
	}

	return tenantConfig, true
}
