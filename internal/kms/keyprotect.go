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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/IBM/go-sdk-core/v5/core"
	kp "github.com/IBM/keyprotect-go-client/ibmkeyprotectapiv2"

	"github.com/ceph/ceph-csi/internal/util/k8s"
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

	// keyProtectDefaultTokenURL is the IAM token endpoint.
	keyProtectDefaultTokenURL = "https://iam.cloud.ibm.com/oidc/token" //nolint:gosec // only the IAM token endpoint
	keyProtectIAMTokenPath    = "/oidc/token"

	// retry settings matching the old client.
	keyProtectRetryMax     = 4
	keyProtectRetryWaitMax = 30 * time.Second
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
	client            *kp.IbmKeyProtectApiV2
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
		kms.baseURL = kp.DefaultServiceURL
	}

	err = setConfigString(&kms.tokenURL, args.Config, keyProtectServiceTokenURL)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.tokenURL = keyProtectDefaultTokenURL
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

func (kms *keyProtectKMS) Destroy() {
	// Nothing to do.
}

func (kms *keyProtectKMS) RequiresDEKStore() DEKStoreType {
	return DEKStoreMetadata
}

// keyActionRequest is the body of the KeyProtect `wrap` and `unwrap` key
// actions. The v2 SDK passes the body through as an opaque
// io.ReadCloser, so it has to be marshaled here.
type keyActionRequest struct {
	Plaintext  string   `json:"plaintext,omitempty"`
	Ciphertext string   `json:"ciphertext,omitempty"`
	AAD        []string `json:"aad,omitempty"`
}

func (r keyActionRequest) body() (io.ReadCloser, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the key action request: %w", err)
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// EncryptDEK uses the KeyProtect KMS and the configured CRK to encrypt the DEK.
func (kms *keyProtectKMS) EncryptDEK(ctx context.Context, volumeID, plainDEK string) (string, error) {
	if err := kms.getService(); err != nil {
		return "", fmt.Errorf("could not get KMS service: %w", err)
	}

	body, err := keyActionRequest{Plaintext: plainDEK, AAD: []string{volumeID}}.body()
	if err != nil {
		return "", err
	}

	opts := kms.client.NewWrapKeyOptions(kms.customerRootKey, kms.serviceInstanceID)
	opts.SetKeyActionWrapBody(body)

	result, _, err := kms.client.WrapKeyWithContext(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to wrap the DEK: %w", err)
	}
	if result == nil || result.Ciphertext == nil {
		return "", errors.New("failed to wrap the DEK: no ciphertext in the response")
	}

	// KeyProtect returns the ciphertext base64 encoded already, base64 encode
	// it once more to stay compatible with the older client.
	return base64.StdEncoding.EncodeToString([]byte(*result.Ciphertext)), nil
}

// DecryptDEK uses the Key protect KMS and the configured CRK to decrypt the DEK.
func (kms *keyProtectKMS) DecryptDEK(ctx context.Context, volumeID, encryptedDEK string) (string, error) {
	if err := kms.getService(); err != nil {
		return "", fmt.Errorf("could not get KMS service: %w", err)
	}

	ciphertextBlob, err := base64.StdEncoding.DecodeString(encryptedDEK)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 cipher: %w",
			err)
	}

	body, err := keyActionRequest{Ciphertext: string(ciphertextBlob), AAD: []string{volumeID}}.body()
	if err != nil {
		return "", err
	}

	opts := kms.client.NewUnwrapKeyOptions(kms.customerRootKey, kms.serviceInstanceID, body)

	result, _, err := kms.client.UnwrapKeyWithContext(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to unwrap the DEK: %w", err)
	}
	if result == nil || result.Plaintext == nil {
		return "", errors.New("failed to unwrap the DEK: no plaintext in the response")
	}

	return *result.Plaintext, nil
}

func (kms *keyProtectKMS) GetSecret(ctx context.Context, volumeID string) (string, error) {
	return "", ErrGetSecretUnsupported
}

func (kms *keyProtectKMS) getSecrets() (map[string]any, error) {
	secretData, err := k8s.GetSecret(kms.secretName, kms.namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w", kms.namespace, kms.secretName, err)
	}

	config := make(map[string]any)

	for k, v := range secretData {
		switch k {
		case keyProtectServiceAPIKey, KeyProtectCustomerRootKey, keyProtectSessionToken, keyProtectCRK:
			config[k] = v
		default:
			return nil, fmt.Errorf("unsupported option for KMS "+
				"provider %q: %s", kmsTypeKeyProtectMetadata, k)
		}
	}

	return config, nil
}

func (kms *keyProtectKMS) getService() error {
	authenticator := &core.IamAuthenticator{
		ApiKey: kms.serviceAPIKey,
		// Requires the base IAM endpoint, trim the oidc token suffix.
		URL: strings.TrimSuffix(kms.tokenURL, keyProtectIAMTokenPath),
	}

	// Build a new client, the Instance ID is passed with
	// every request in the headers.
	client, err := kp.NewIbmKeyProtectApiV2(&kp.IbmKeyProtectApiV2Options{
		URL:           kms.baseURL,
		Authenticator: authenticator,
	})
	if err != nil {
		return fmt.Errorf("failed to create keyprotect client: %w", err)
	}

	client.Service.EnableRetries(keyProtectRetryMax, keyProtectRetryWaitMax)
	kms.client = client

	return nil
}
