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

package util

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/libopenstorage/secrets/vault"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeVaultTenantSA = "vaulttenantsa"

	// vaultTenantSAName is the default name of the ServiceAccount that
	// should be available in the Tenants namespace. This ServiceAccount
	// will be used to connect to Hashicorp Vault.
	vaultTenantSAName = "ceph-csi-vault-sa"
)

/*
VaultTenantSA represents a Hashicorp Vault KMS configuration that uses a
ServiceAccount from the Tenant that owns the volume to store/retrieve the
encryption passphrase of volumes.

Example JSON structure in the KMS config is,
{
    "vault-tenant-sa": {
        "encryptionKMSType": "vaulttenantsa",
        "vaultAddress": "http://vault.default.svc.cluster.local:8200",
        "vaultBackendPath": "secret/",
        "vaultTLSServerName": "vault.default.svc.cluster.local",
        "vaultCAFromSecret": "vault-ca",
        "vaultClientCertFromSecret": "vault-client-cert",
        "vaultClientCertKeyFromSecret": "vault-client-cert-key",
        "vaultCAVerify": "false",
        "tenantConfigName": "ceph-csi-kms-config",
        "tenantSAName": "ceph-csi-vault-sa",
        "tenants": {
            "my-app": {
                "vaultAddress": "https://vault.example.com",
                "vaultCAVerify": "true"
            },
            "an-other-app": {
                "tenantSAName": "encryped-storage-sa"
            }
	},
	...
}.
*/
type VaultTenantSA struct {
	vaultTenantConnection

	// tenantSAName is the name of the ServiceAccount in the Tenants Kubernetes Namespace
	tenantSAName string

	// saTokenDir contains the directory that holds the token to connect to Vault.
	saTokenDir string
}

var _ = RegisterKMSProvider(KMSProvider{
	UniqueID:    kmsTypeVaultTenantSA,
	Initializer: initVaultTenantSA,
})

// initVaultTenantSA returns an interface to HashiCorp Vault KMS where Tenants
// use their ServiceAccount to access the service.
func initVaultTenantSA(args KMSInitializerArgs) (EncryptionKMS, error) {
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

	kms := &VaultTenantSA{}
	err = kms.initConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault connection: %w", err)
	}

	// set default values for optional config options
	kms.ConfigName = vaultTokensDefaultConfigName
	kms.tenantSAName = vaultTenantSAName

	// TODO: should this be configurable per tenant?
	vaultRole := vaultDefaultRole
	err = setConfigString(&vaultRole, args.Config, "vaultRole")
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}
	kms.vaultConfig[vault.AuthKubernetesRole] = vaultRole

	err = kms.parseConfig(config)
	if err != nil {
		return nil, err
	}

	// fetch the configuration for the tenant
	if args.Tenant != "" {
		kms.Tenant = args.Tenant
		tenantConfig, found := fetchTenantConfig(config, args.Tenant)
		if found {
			// override connection details from the tenant
			err = kms.parseConfig(tenantConfig)
			if err != nil {
				return nil, err
			}
		}

		err = kms.parseTenantConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to parse config for tenant: %w", err)
		}

		err = kms.setServiceAccountName(config)
		if err != nil {
			return nil, fmt.Errorf("failed to set the ServiceAccount name from %s for tenant (%s): %w",
				kms.ConfigName, kms.Tenant, err)
		}
	}

	kms.vaultConfig[vault.AuthMethod] = vault.AuthMethodKubernetes
	kms.vaultConfig[vault.AuthKubernetesTokenPath], err = kms.getTokenPath()
	if err != nil {
		return nil, fmt.Errorf("failed setting up token for %s/%s: %w", kms.Tenant, kms.tenantSAName, err)
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

// Destroy removes the temporary stored token from the ServiceAccount and
// destroys the vaultTenantConnection object.
func (kms *VaultTenantSA) Destroy() {
	if kms.saTokenDir != "" {
		_ = os.RemoveAll(kms.saTokenDir)
	}

	kms.vaultTenantConnection.Destroy()
}

// setServiceAccountName stores the name of the ServiceAccount in the
// configuration if it has been set in the options.
func (kms *VaultTenantSA) setServiceAccountName(config map[string]interface{}) error {
	err := setConfigString(&kms.tenantSAName, config, "tenantSAName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	return nil
}

// getServiceAccount returns the Tenants ServiceAccount with the name
// configured in the VaultTenantSA.
func (kms *VaultTenantSA) getServiceAccount() (*corev1.ServiceAccount, error) {
	c := kms.getK8sClient()
	sa, err := c.CoreV1().ServiceAccounts(kms.Tenant).Get(context.TODO(),
		kms.tenantSAName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ServiceAccount %s/%s: %w", kms.Tenant, kms.tenantSAName, err)
	}

	return sa, nil
}

// getToken looks up the ServiceAccount and the Secrets linked from it. When it
// finds the Secret that contains the `token` field, the contents is read and
// returned.
func (kms *VaultTenantSA) getToken() (string, error) {
	sa, err := kms.getServiceAccount()
	if err != nil {
		return "", err
	}

	c := kms.getK8sClient()
	for _, secretRef := range sa.Secrets {
		secret, err := c.CoreV1().Secrets(kms.Tenant).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get Secret %s/%s: %w", kms.Tenant, secretRef.Name, err)
		}

		token, ok := secret.Data["token"]
		if ok {
			return string(token), nil
		}
	}

	return "", fmt.Errorf("failed to find token in ServiceAccount %s/%s", kms.Tenant, kms.tenantSAName)
}

// getTokenPath creates a temporary directory structure that contains the token
// linked from the ServiceAccount. This path can then be used in place of the
// standard `/var/run/secrets/kubernetes.io/serviceaccount/token` location.
func (kms *VaultTenantSA) getTokenPath() (string, error) {
	dir, err := ioutil.TempDir("", kms.tenantSAName)
	if err != nil {
		return "", fmt.Errorf("failed to create directory for ServiceAccount %s/%s: %w", kms.tenantSAName, kms.Tenant, err)
	}

	token, err := kms.getToken()
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(dir+"/token", []byte(token), 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write token for ServiceAccount %s/%s: %w", kms.tenantSAName, kms.Tenant, err)
	}

	return dir + "/token", nil
}
