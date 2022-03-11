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

	"github.com/aws/aws-sdk-go/aws"
	awsCreds "github.com/aws/aws-sdk-go/aws/credentials"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	awsKMS "github.com/aws/aws-sdk-go/service/kms"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeAWSMetadata = "aws-metadata"

	// awsMetadataDefaultSecretsName is the default name of the Kubernetes Secret
	// that contains the credentials to access the Amazon KMS. The name of
	// the Secret can be configured by setting the `KMS_SECRET_NAME`
	// option.
	//
	// #nosec:G101, value not credential, just references token.
	awsMetadataDefaultSecretsName = "ceph-csi-aws-credentials"

	// awsSecretNameKey contains the name of the Kubernetes Secret that has
	// the credentials to access the Amazon KMS.
	//
	// #nosec:G101, no hardcoded secret, this is a configuration key.
	awsSecretNameKey = "KMS_SECRET_NAME"
	awsRegionKey     = "AWS_REGION"

	// The following options are part of the Kubernetes Secrets.
	//
	// #nosec:G101, no hardcoded secrets, only configuration keys.
	awsAccessKey = "AWS_ACCESS_KEY_ID"
	// #nosec:G101.
	awsSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	// #nosec:G101.
	awsSessionToken = "AWS_SESSION_TOKEN"
	awsCMK          = "AWS_CMK_ARN"
)

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeAWSMetadata,
	Initializer: initAWSMetadataKMS,
})

type awsMetadataKMS struct {
	// basic options to get the secret
	namespace  string
	secretName string

	// standard AWS configuration options
	region          string
	secretAccessKey string
	accessKey       string
	sessionToken    string
	cmk             string
}

func initAWSMetadataKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	kms := &awsMetadataKMS{
		namespace: args.Namespace,
	}

	// required options for further configuration (getting secrets)
	err := setConfigString(&kms.secretName, args.Config, awsSecretNameKey)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.secretName = awsMetadataDefaultSecretsName
	}
	err = setConfigString(&kms.region, args.Config, awsRegionKey)
	if err != nil {
		return nil, err
	}

	// read the Kubernetes Secret with credentials
	secrets, err := kms.getSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for %T: %w", kms,
			err)
	}

	err = setConfigString(&kms.accessKey, secrets, awsAccessKey)
	if err != nil {
		return nil, err
	}
	err = setConfigString(&kms.secretAccessKey, secrets,
		awsSecretAccessKey)
	if err != nil {
		return nil, err
	}
	// awsSessionToken is optional
	err = setConfigString(&kms.sessionToken, secrets, awsSessionToken)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}
	err = setConfigString(&kms.cmk, secrets, awsCMK)
	if err != nil {
		return nil, err
	}

	return kms, nil
}

func (kms *awsMetadataKMS) getSecrets() (map[string]interface{}, error) {
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
		case awsSecretAccessKey, awsAccessKey, awsSessionToken, awsCMK:
			config[k] = string(v)
		default:
			return nil, fmt.Errorf("unsupported option for KMS "+
				"provider %q: %s", kmsTypeAWSMetadata, k)
		}
	}

	return config, nil
}

func (kms *awsMetadataKMS) Destroy() {
	// Nothing to do.
}

// RequiresDEKStore indicates that the DEKs should get stored in the metadata
// of the volumes. This Amazon KMS provider does not support storing DEKs in
// AWS as that adds additional costs.
func (kms *awsMetadataKMS) RequiresDEKStore() DEKStoreType {
	return DEKStoreMetadata
}

func (kms *awsMetadataKMS) getService() (*awsKMS.KMS, error) {
	creds := awsCreds.NewStaticCredentials(kms.accessKey,
		kms.secretAccessKey, kms.sessionToken)

	sess, err := awsSession.NewSessionWithOptions(awsSession.Options{
		SharedConfigState: awsSession.SharedConfigDisable,
		Config: aws.Config{
			Credentials: creds,
			Region:      aws.String(kms.region),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return awsKMS.New(sess), nil
}

// EncryptDEK uses the Amazon KMS and the configured CMK to encrypt the DEK.
func (kms *awsMetadataKMS) EncryptDEK(volumeID, plainDEK string) (string, error) {
	svc, err := kms.getService()
	if err != nil {
		return "", fmt.Errorf("could not get KMS service: %w", err)
	}

	result, err := svc.Encrypt(&awsKMS.EncryptInput{
		KeyId:     aws.String(kms.cmk),
		Plaintext: []byte(plainDEK),
	})
	if err != nil {
		return "", fmt.Errorf("failed to encrypt DEK: %w", err)
	}

	// base64 encode the encrypted DEK, so that storing it should not have
	// issues
	encryptedDEK := base64.StdEncoding.EncodeToString(result.CiphertextBlob)

	return encryptedDEK, nil
}

// DecryptDEK uses the Amazon KMS and the configured CMK to decrypt the DEK.
func (kms *awsMetadataKMS) DecryptDEK(volumeID, encryptedDEK string) (string, error) {
	svc, err := kms.getService()
	if err != nil {
		return "", fmt.Errorf("could not get KMS service: %w", err)
	}

	ciphertextBlob, err := base64.StdEncoding.DecodeString(encryptedDEK)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 cipher: %w",
			err)
	}

	result, err := svc.Decrypt(&awsKMS.DecryptInput{
		CiphertextBlob: ciphertextBlob,
	})
	if err != nil {
		return "", fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	return string(result.Plaintext), nil
}

func (kms *awsMetadataKMS) GetSecret(volumeID string) (string, error) {
	return "", ErrGetSecretUnsupported
}
