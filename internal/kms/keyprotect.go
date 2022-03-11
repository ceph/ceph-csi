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
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	kp "github.com/IBM/keyprotect-go-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeKeyProtectMetadata = "ibmkeyprotect"
	// keyProtectMetadataDefaultSecretsName is the default name of the Kubernetes Secret
	// that contains the credentials to access the Key Protect KMS. The name of
	// the Secret can be configured by setting the `IBM_KP_SECRET_NAME`
	// option.
	//
	// #nosec:G101, value not credential, just references token.
	keyProtectMetadataDefaultSecretsName = "ceph-csi-kp-credentials"

	// keyProtectSecretNameKey contains the name of the Kubernetes Secret that has
	// the credentials to access the Key ProtectKMS.
	//
	// #nosec:G101, no hardcoded secret, this is a configuration key.
	keyProtectSecretNameKey = "IBM_KP_SECRET_NAME"
	keyProtectRegionKey     = "IBM_KP_REGION"

	keyProtectServiceInstanceID = "IBM_KP_SERVICE_INSTANCE_ID"
	keyProtectServiceBaseURL    = "IBM_KP_BASE_URL"
	keyProtectServiceTokenURL   = "IBM_KP_TOKEN_URL" //nolint:gosec // only configuration key
	// The following options are part of the Kubernetes Secrets.
	// #nosec:G101, no hardcoded secrets, only configuration keys.
	keyProtectServiceAPIKey   = "IBM_KP_SERVICE_API_KEY"
	KeyProtectCustomerRootKey = "IBM_KP_CUSTOMER_ROOT_KEY"
	keyProtectSessionToken    = "IBM_KP_SESSION_TOKEN" //nolint:gosec // only configuration key
	keyProtectCRK             = "IBM_KP_CRK_ARN"
)

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeKeyProtectMetadata,
	Initializer: initKeyProtectKMS,
})

// KeyProtectKMS store the KMS connection information retrieved from the kms configmap.
type keyProtectKMS struct {
	// basic options to get the secret
	namespace  string
	secretName string

	// standard KeyProtect configuration options
	client            *kp.Client
	serviceAPIKey     string
	customerRootKey   string
	serviceInstanceID string
	baseURL           string
	tokenURL          string
	region            string
	sessionToken      string
	crk               string
}

func initKeyProtectKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	kms := &keyProtectKMS{
		namespace: args.Namespace,
	}
	// required options for further configuration (getting secrets)
	err := setConfigString(&kms.secretName, args.Config, keyProtectSecretNameKey)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.secretName = keyProtectMetadataDefaultSecretsName
	}

	err = setConfigString(&kms.serviceInstanceID, args.Config, keyProtectServiceInstanceID)
	if err != nil {
		return nil, err
	}

	err = setConfigString(&kms.baseURL, args.Config, keyProtectServiceBaseURL)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.baseURL = kp.DefaultBaseURL
	}

	err = setConfigString(&kms.tokenURL, args.Config, keyProtectServiceTokenURL)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.tokenURL = kp.DefaultTokenURL
	}

	// read the Kubernetes Secret with credentials
	secrets, err := kms.getSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for %T: %w", kms,
			err)
	}

	err = setConfigString(&kms.serviceAPIKey, secrets, keyProtectServiceAPIKey)
	if err != nil {
		return nil, err
	}
	err = setConfigString(&kms.customerRootKey, secrets, KeyProtectCustomerRootKey)
	if err != nil {
		return nil, err
	}

	// keyProtectSessionToken is optional
	err = setConfigString(&kms.sessionToken, secrets, keyProtectSessionToken)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}

	// KeyProtect Region is optional
	err = setConfigString(&kms.region, args.Config, keyProtectRegionKey)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}

	// crk arn is optional
	err = setConfigString(&kms.crk, secrets, keyProtectCRK)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}

	return kms, nil
}

func (kms *keyProtectKMS) getSecrets() (map[string]interface{}, error) {
	c, err := k8s.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes to "+
			"get Secret %s/%s: %w", kms.namespace, kms.secretName, err)
	}

	secret, err := c.CoreV1().Secrets(kms.namespace).Get(context.TODO(),
		kms.secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w",
			kms.namespace, kms.secretName, err)
	}

	config := make(map[string]interface{})

	for k, v := range secret.Data {
		switch k {
		case keyProtectServiceAPIKey, KeyProtectCustomerRootKey, keyProtectSessionToken, keyProtectCRK:
			config[k] = string(v)
		default:
			return nil, fmt.Errorf("unsupported option for KMS "+
				"provider %q: %s", kmsTypeKeyProtectMetadata, k)
		}
	}

	return config, nil
}

func (kms *keyProtectKMS) Destroy() {
	// Nothing to do.
}

func (kms *keyProtectKMS) RequiresDEKStore() DEKStoreType {
	return DEKStoreMetadata
}

func (kms *keyProtectKMS) getService() error {
	// Use your Service API Key and your KeyProtect Service Instance ID to create a ClientConfig
	cc := kp.ClientConfig{
		BaseURL:    kms.baseURL,
		TokenURL:   kms.tokenURL,
		APIKey:     kms.serviceAPIKey,
		InstanceID: kms.serviceInstanceID,
	}

	// Build a new client from the config
	client, err := kp.New(cc, kp.DefaultTransport())
	if err != nil {
		return fmt.Errorf("failed to create keyprotect client: %w", err)
	}
	kms.client = client

	return nil
}

// EncryptDEK uses the KeyProtect KMS and the configured CRK to encrypt the DEK.
func (kms *keyProtectKMS) EncryptDEK(volumeID, plainDEK string) (string, error) {
	if err := kms.getService(); err != nil {
		return "", fmt.Errorf("could not get KMS service: %w", err)
	}

	dekByteSlice := []byte(plainDEK)
	aadVolID := []string{volumeID}
	result, err := kms.client.Wrap(context.TODO(), kms.customerRootKey, dekByteSlice, &aadVolID)
	if err != nil {
		return "", fmt.Errorf("failed to wrap the DEK: %w", err)
	}

	// base64 encode the encrypted DEK, so that storing it should not have
	// issues

	return base64.StdEncoding.EncodeToString(result), nil
}

// DecryptDEK uses the Key protect KMS and the configured CRK to decrypt the DEK.
func (kms *keyProtectKMS) DecryptDEK(volumeID, encryptedDEK string) (string, error) {
	if err := kms.getService(); err != nil {
		return "", fmt.Errorf("could not get KMS service: %w", err)
	}

	ciphertextBlob, err := base64.StdEncoding.DecodeString(encryptedDEK)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 cipher: %w",
			err)
	}

	aadVolID := []string{volumeID}
	result, err := kms.client.Unwrap(context.TODO(), kms.customerRootKey, ciphertextBlob, &aadVolID)
	if err != nil {
		return "", fmt.Errorf("failed to unwrap the DEK: %w", err)
	}

	return string(result), nil
}

func (kms *keyProtectKMS) GetSecret(volumeID string) (string, error) {
	return "", ErrGetSecretUnsupported
}
