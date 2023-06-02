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

package kms

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/libopenstorage/secrets/vault"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
type vaultTenantSA struct {
	vaultTenantConnection

	// tenantSAName is the name of the ServiceAccount in the Tenants Kubernetes Namespace
	tenantSAName string

	// saTokenDir contains the directory that holds the token to connect to Vault.
	saTokenDir string
}

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeVaultTenantSA,
	Initializer: initVaultTenantSA,
})

// initVaultTenantSA returns an interface to HashiCorp Vault KMS where Tenants
// use their ServiceAccount to access the service.
func initVaultTenantSA(args ProviderInitArgs) (EncryptionKMS, error) {
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

	kms := &vaultTenantSA{}
	kms.vaultTenantConnection.init()
	kms.tenantConfigOptionFilter = isTenantSAConfigOption

	err = kms.initConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault connection: %w", err)
	}

	// set default values for optional config options
	kms.ConfigName = vaultTokensDefaultConfigName
	kms.tenantSAName = vaultTenantSAName

	// "vaultAuthPath" is configurable per tenant
	kms.vaultConfig[vault.AuthMountPath] = vaultDefaultAuthMountPath

	// "vaultRole" is configurable per tenant
	kms.vaultConfig[vault.AuthKubernetesRole] = vaultDefaultRole

	err = kms.parseConfig(config)
	if err != nil {
		return nil, err
	}

	// fetch the configuration for the tenant
	if args.Tenant != "" {
		err = kms.configureTenant(config, args.Tenant)
		if err != nil {
			return nil, err
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
func (kms *vaultTenantSA) Destroy() {
	if kms.saTokenDir != "" {
		_ = os.RemoveAll(kms.saTokenDir)
	}

	kms.vaultTenantConnection.Destroy()
}

func (kms *vaultTenantSA) configureTenant(config map[string]interface{}, tenant string) error {
	kms.Tenant = tenant
	tenantConfig, found := fetchTenantConfig(config, tenant)
	if found {
		// override connection details from the tenant
		err := kms.parseConfig(tenantConfig)
		if err != nil {
			return err
		}
	}

	// get the ConfigMap from the Tenant and apply the options
	tenantConfig, err := kms.parseTenantConfig()
	if err != nil {
		return fmt.Errorf("failed to parse config for tenant: %w", err)
	} else if tenantConfig != nil {
		err = kms.parseConfig(tenantConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

// parseConfig calls vaultTenantConnection.parseConfig() and also set
// additional config options specific to vaultTenantSA. This function is called
// multiple times, for the different nested configuration layers.
// parseTenantConfig() calls this as well, with a reduced set of options,
// filtered by isTenantConfigOption().
func (kms *vaultTenantSA) parseConfig(config map[string]interface{}) error {
	err := kms.vaultTenantConnection.parseConfig(config)
	if err != nil {
		return err
	}

	err = kms.setServiceAccountName(config)
	if err != nil {
		return fmt.Errorf("failed to set the ServiceAccount name from %s for tenant (%s): %w",
			kms.ConfigName, kms.Tenant, err)
	}

	// default vaultAuthPath is set in initVaultTenantSA()
	var vaultAuthPath string
	err = setConfigString(&vaultAuthPath, config, "vaultAuthPath")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	} else if err == nil {
		kms.vaultConfig[vault.AuthMountPath], err = detectAuthMountPath(vaultAuthPath)
		if err != nil {
			return fmt.Errorf("failed to set \"vaultAuthPath\" in Vault config: %w", err)
		}
	}

	// default vaultRole is set in initVaultTenantSA()
	var vaultRole string
	err = setConfigString(&vaultRole, config, "vaultRole")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	} else if err == nil {
		kms.vaultConfig[vault.AuthKubernetesRole] = vaultRole
	}

	return nil
}

// isTenantSAConfigOption is used by vaultTenantConnection.parseTenantConfig()
// to filter options that should not be set by the configuration in the tenants
// ConfigMap. Options that are allowed to be set, will return true, options
// that are filtered return false.
func isTenantSAConfigOption(opt string) bool {
	// standard vaultTenantConnection options are accepted
	if isTenantConfigOption(opt) {
		return true
	}

	// additional options for vaultTenantSA
	switch opt {
	case "tenantSAName":
	case "vaultAuthPath":
	case "vaultRole":
	default:
		return false
	}

	return true
}

// setServiceAccountName stores the name of the ServiceAccount in the
// configuration if it has been set in the options.
func (kms *vaultTenantSA) setServiceAccountName(config map[string]interface{}) error {
	err := setConfigString(&kms.tenantSAName, config, "tenantSAName")
	if errors.Is(err, errConfigOptionInvalid) {
		return err
	}

	return nil
}

// getServiceAccount returns the Tenants ServiceAccount with the name
// configured in the vaultTenantSA.
func (kms *vaultTenantSA) getServiceAccount() (*corev1.ServiceAccount, error) {
	c, err := kms.getK8sClient()
	if err != nil {
		return nil, fmt.Errorf("can not get ServiceAccount %s/%s, "+
			"failed to connect to Kubernetes: %w", kms.Tenant, kms.tenantSAName, err)
	}

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
func (kms *vaultTenantSA) getToken() (string, error) {
	sa, err := kms.getServiceAccount()
	if err != nil {
		return "", err
	}

	c, err := kms.getK8sClient()
	if err != nil {
		return "", fmt.Errorf("can not get ServiceAccount %s/%s, failed "+
			"to connect to Kubernetes: %w", kms.Tenant,
			kms.tenantSAName, err)
	}

	// From Kubernetes v1.24+, secret for service account tokens are not
	// automatically created. Trying to fetch tokens from service account secret references will fail
	// refer: https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.24.md \
	// #no-really-you-must-read-this-before-you-upgrade-1.
	token, err := kms.createToken(sa, c)
	if err == nil {
		return token, nil
	}

	for _, secretRef := range sa.Secrets {
		secret, sErr := c.CoreV1().Secrets(kms.Tenant).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
		if sErr != nil {
			return "", fmt.Errorf("failed to get Secret %s/%s: %w", kms.Tenant, secretRef.Name, sErr)
		}

		token, ok := secret.Data["token"]
		if ok {
			return string(token), nil
		}
	}

	return "", fmt.Errorf("failed to find/create ServiceAccount token %s/%s: %w", kms.Tenant, kms.tenantSAName, err)
}

// getTokenPath creates a temporary directory structure that contains the token
// linked from the ServiceAccount. This path can then be used in place of the
// standard `/var/run/secrets/kubernetes.io/serviceaccount/token` location.
func (kms *vaultTenantSA) getTokenPath() (string, error) {
	dir, err := os.MkdirTemp("", kms.tenantSAName)
	if err != nil {
		return "", fmt.Errorf("failed to create directory for ServiceAccount %s/%s: %w", kms.tenantSAName, kms.Tenant, err)
	}

	token, err := kms.getToken()
	if err != nil {
		return "", err
	}

	err = os.WriteFile(dir+"/token", []byte(token), 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to write token for ServiceAccount %s/%s: %w", kms.tenantSAName, kms.Tenant, err)
	}

	return dir + "/token", nil
}

// createToken creates required service account token using the TokenRequest API.
func (kms *vaultTenantSA) createToken(sa *corev1.ServiceAccount, client *kubernetes.Clientset) (string, error) {
	tokenRequest := &authenticationv1.TokenRequest{}
	token, err := client.CoreV1().ServiceAccounts(kms.Tenant).CreateToken(
		context.TODO(),
		sa.Name,
		tokenRequest,
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create token for service account %s/%s: %w", kms.Tenant, sa.Name, err)
	}

	return token.Status.Token, nil
}
