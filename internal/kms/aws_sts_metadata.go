/*
Copyright 2022 The Ceph-CSI Authors.

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
	"os"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	awsSTS "github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go/aws"
	awsCreds "github.com/aws/aws-sdk-go/aws/credentials"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	awsKMS "github.com/aws/aws-sdk-go/service/kms"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeAWSSTSMetadata = "aws-sts-metadata"

	// awsRoleSessionName is the name of the role session to connect with aws STS.
	awsRoleSessionName = "ceph-csi-aws-sts-metadata"

	// awsMetadataDefaultSecretsName is the default name of the Kubernetes Secret
	// that contains the credentials to access the Amazon KMS. The name of
	// the Secret can be configured by setting the `kmsSecretName`
	// option.
	//
	// #nosec:G101, value not credential, just references token.
	awsSTSMetadataDefaultSecretsName = "ceph-csi-aws-credentials"

	// awsSTSSecretNameKey is the key for the secret name in the config map.
	awsSTSSecretNameKey = "secretName"

	// The following options are part of the Kubernetes Secrets.
	//
	// #nosec:G101, value not credential, just configuration keys.
	awsSTSRoleARNKey = "awsRoleARN"
	awsSTSCMKARNKey  = "awsCMKARN"
	awsSTSRegionKey  = "awsRegion"

	// tokenFilePath is the path to the file containing the OIDC token.
	//
	// #nosec:G101, value not credential, just path to the token.
	tokenFilePath = "/run/secrets/tokens/oidc-token"
)

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeAWSSTSMetadata,
	Initializer: initAWSSTSMetadataKMS,
})

type awsSTSMetadataKMS struct {
	awsMetadataKMS

	// AWS STS configuration options
	role string
}

func initAWSSTSMetadataKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	kms := &awsSTSMetadataKMS{
		awsMetadataKMS: awsMetadataKMS{
			namespace: args.Tenant,
		},
	}

	// get secret name if set, else use default.
	err := setConfigString(&kms.secretName, args.Config, awsSTSSecretNameKey)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.secretName = awsSTSMetadataDefaultSecretsName
	}

	// read the Kubernetes Secret with aws region, role & cmk ARN.
	secrets, err := kms.getSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets: %w", err)
	}

	var found bool
	kms.role, found = secrets[awsSTSRoleARNKey]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, awsSTSRoleARNKey)
	}

	kms.cmk, found = secrets[awsSTSCMKARNKey]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, awsSTSCMKARNKey)
	}

	kms.region, found = secrets[awsSTSRegionKey]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, awsSTSRegionKey)
	}

	return kms, nil
}

// getSecrets returns required STS configuration options from the Kubernetes Secret.
func (as *awsSTSMetadataKMS) getSecrets() (map[string]string, error) {
	c, err := k8s.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes to "+
			"get Secret %s/%s: %w", as.namespace, as.secretName, err)
	}

	secret, err := c.CoreV1().Secrets(as.namespace).Get(context.TODO(),
		as.secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w",
			as.namespace, as.secretName, err)
	}

	config := make(map[string]string)
	for k, v := range secret.Data {
		switch k {
		case awsSTSRoleARNKey, awsSTSRegionKey, awsSTSCMKARNKey:
			config[k] = string(v)
		default:
			return nil, fmt.Errorf("unsupported option for KMS "+
				"provider %q: %s", kmsTypeAWSMetadata, k)
		}
	}

	return config, nil
}

// getWebIdentityToken returns the web identity token from the file.
func (as *awsSTSMetadataKMS) getWebIdentityToken() (string, error) {
	buf, err := os.ReadFile(tokenFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read oidc token file %q: %w",
			tokenFilePath, err)
	}

	return string(buf), nil
}

// getServiceWithSTS returns a new awsSession established with the STS.
func (as *awsSTSMetadataKMS) getServiceWithSTS() (*awsKMS.KMS, error) {
	webIdentityToken, err := as.getWebIdentityToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get web identity token: %w", err)
	}

	client := awsSTS.New(awsSTS.Options{
		Region: as.region,
	})
	output, err := client.AssumeRoleWithWebIdentity(context.TODO(),
		&awsSTS.AssumeRoleWithWebIdentityInput{
			RoleArn:          aws.String(as.role),
			RoleSessionName:  aws.String(awsRoleSessionName),
			WebIdentityToken: aws.String(webIdentityToken),
		})
	if err != nil {
		return nil, fmt.Errorf("failed to assume role with web identity token: %w", err)
	}

	creds := awsCreds.NewStaticCredentials(*output.Credentials.AccessKeyId,
		*output.Credentials.SecretAccessKey, *output.Credentials.SessionToken)

	sess, err := awsSession.NewSessionWithOptions(awsSession.Options{
		SharedConfigState: awsSession.SharedConfigDisable,
		Config: aws.Config{
			Credentials: creds,
			Region:      &as.region,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return awsKMS.New(sess), nil
}

// EncryptDEK uses the Amazon KMS and the configured CMK to encrypt the DEK.
func (as *awsSTSMetadataKMS) EncryptDEK(_, plainDEK string) (string, error) {
	svc, err := as.getServiceWithSTS()
	if err != nil {
		return "", fmt.Errorf("failed to get KMS service: %w", err)
	}

	result, err := svc.Encrypt(&awsKMS.EncryptInput{
		KeyId:     aws.String(as.cmk),
		Plaintext: []byte(plainDEK),
	})
	if err != nil {
		return "", fmt.Errorf("failed to encrypt DEK: %w", err)
	}

	// base64 encode the encrypted DEK, so that storing it should not have
	// issues
	return base64.StdEncoding.EncodeToString(result.CiphertextBlob), nil
}

// DecryptDEK uses the Amazon KMS and the configured CMK to decrypt the DEK.
func (as *awsSTSMetadataKMS) DecryptDEK(_, encryptedDEK string) (string, error) {
	svc, err := as.getServiceWithSTS()
	if err != nil {
		return "", fmt.Errorf("failed to get KMS service: %w", err)
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
