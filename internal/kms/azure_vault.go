/*
Copyright 2024 The Ceph-CSI Authors.

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
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeAzure = "azure-kv"

	// azureDefaultSecretsName is the default name of the Kubernetes secret
	// that contains the credentials to access the Azure key vault. The name
	// the secret can be configured by setting the `AZURE_CERT_SECRET_NAME` option.
	//
	// #nosec:G101, value not credentials, just references token.
	azureDefaultSecretsName = "ceph-csi-azure-credentials"

	// azureSecretNameKey contains the name of the Kubernetes secret that has
	// the credentials to access the Azure key vault.
	//
	// #nosec:G101, no hardcoded secret, this is a configuration key.
	azureSecretNameKey = "AZURE_CERT_SECRET_NAME"

	azureVaultURL = "AZURE_VAULT_URL"
	azureClientID = "AZURE_CLIENT_ID"
	azureTenantID = "AZURE_TENANT_ID"

	// The following options are part of the Kubernetes secrets.
	//
	// #nosec:G101, no hardcoded secrets, only configuration keys.
	azureClientCertificate = "CLIENT_CERT"
)

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeAzure,
	Initializer: initAzureKeyVaultKMS,
})

type azureKMS struct {
	// basic
	namespace  string
	secretName string

	integratedDEK

	// standard
	vaultURL          string
	clientID          string
	tenantID          string
	clientCertificate string
}

func initAzureKeyVaultKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	kms := &azureKMS{
		namespace: args.Namespace,
	}

	// required options for further configuration (getting secrets)
	err := setConfigString(&kms.secretName, args.Config, azureSecretNameKey)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.secretName = azureDefaultSecretsName
	}

	err = setConfigString(&kms.vaultURL, args.Config, azureVaultURL)
	if err != nil {
		return nil, err
	}
	err = setConfigString(&kms.clientID, args.Config, azureClientID)
	if err != nil {
		return nil, err
	}
	err = setConfigString(&kms.tenantID, args.Config, azureTenantID)
	if err != nil {
		return nil, err
	}

	// read the kubernetes secret with credentials
	secrets, err := kms.getSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for %T, %w", kms, err)
	}

	var encodedClientCertificate string
	err = setConfigString(&encodedClientCertificate, secrets, azureClientCertificate)
	if err != nil {
		return nil, err
	}

	clientCertificate, err := base64.StdEncoding.DecodeString(encodedClientCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client certificate: %w", err)
	}

	kms.clientCertificate = string(clientCertificate)

	return kms, nil
}

func (kms *azureKMS) Destroy() {
	// Nothing to do.
}

func (kms *azureKMS) getService() (*azsecrets.Client, error) {
	certs, key, err := azidentity.ParseCertificates([]byte(kms.clientCertificate), []byte{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure client certificate: %w", err)
	}
	creds, err := azidentity.NewClientCertificateCredential(kms.tenantID, kms.clientID, certs, key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	azClient, err := azsecrets.NewClient(kms.vaultURL, creds, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}

	return azClient, nil
}

func (kms *azureKMS) getSecrets() (map[string]interface{}, error) {
	c, err := k8s.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes to "+
			"get secret %s/%s: %w", kms.namespace, kms.secretName, err)
	}

	secret, err := c.CoreV1().Secrets(kms.namespace).Get(context.TODO(),
		kms.secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", kms.namespace, kms.secretName, err)
	}

	config := make(map[string]interface{})
	for k, v := range secret.Data {
		switch k {
		case azureClientCertificate:
			config[k] = string(v)
		default:
			return nil, fmt.Errorf("unsupported option for KMS provider %q: %s", kmsTypeAzure, k)
		}
	}

	return config, nil
}

// FetchDEK returns passphrase from Azure key vault.
func (kms *azureKMS) FetchDEK(ctx context.Context, key string) (string, error) {
	svc, err := kms.getService()
	if err != nil {
		return "", fmt.Errorf("failed to get KMS service: %w", err)
	}

	getSecretResponse, err := svc.GetSecret(ctx, key, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	return *getSecretResponse.Value, nil
}

// StoreDEK saves new passphrase to Azure key vault.
func (kms *azureKMS) StoreDEK(ctx context.Context, key, value string) error {
	svc, err := kms.getService()
	if err != nil {
		return fmt.Errorf("failed to get KMS service: %w", err)
	}

	setSecretParams := azsecrets.SetSecretParameters{
		Value: &value,
	}
	_, err = svc.SetSecret(ctx, key, setSecretParams, nil)
	if err != nil {
		return fmt.Errorf("failed to set seceret %w", err)
	}

	return nil
}

// RemoveDEK deletes passphrase from Azure key vault.
func (kms *azureKMS) RemoveDEK(ctx context.Context, key string) error {
	svc, err := kms.getService()
	if err != nil {
		return fmt.Errorf("failed to get KMS service: %w", err)
	}

	_, err = svc.DeleteSecret(ctx, key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete seceret %w", err)
	}

	return nil
}
