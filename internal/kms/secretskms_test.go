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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSecretsKMS(t *testing.T) {
	t.Parallel()
	secrets := map[string]string{}

	// no passphrase in the secrets, should fail
	kms, err := newSecretsKMS(ProviderInitArgs{
		Secrets: secrets,
	})
	require.Error(t, err)
	require.Nil(t, kms)

	// set a passphrase and it should pass
	secrets[encryptionPassphraseKey] = "plaintext encryption key"
	kms, err = newSecretsKMS(ProviderInitArgs{
		Secrets: secrets,
	})
	require.NotNil(t, kms)
	require.NoError(t, err)
}

func TestGenerateNonce(t *testing.T) {
	t.Parallel()
	size := 64
	nonce, err := generateNonce(size)
	require.Len(t, nonce, size)
	require.NoError(t, err)
}

func TestGenerateCipher(t *testing.T) {
	t.Parallel()
	//nolint:gosec // this passphrase is intentionally hardcoded
	passphrase := "my-cool-luks-passphrase"
	salt := "unique-id-for-the-volume"

	aead, err := generateCipher(passphrase, salt)
	require.NoError(t, err)
	require.NotNil(t, aead)
}

func TestInitSecretsMetadataKMS(t *testing.T) {
	t.Parallel()
	args := ProviderInitArgs{
		Tenant:  "tenant",
		Config:  nil,
		Secrets: map[string]string{},
	}

	// passphrase it not set, init should fail
	kms, err := initSecretsMetadataKMS(args)
	require.Error(t, err)
	require.Nil(t, kms)

	// set a passphrase to get a working KMS
	args.Secrets[encryptionPassphraseKey] = "my-passphrase-from-kubernetes"

	kms, err = initSecretsMetadataKMS(args)
	require.NoError(t, err)
	require.NotNil(t, kms)
	require.Equal(t, DEKStoreMetadata, kms.RequiresDEKStore())
}

func TestWorkflowSecretsMetadataKMS(t *testing.T) {
	t.Parallel()
	secrets := map[string]string{
		encryptionPassphraseKey: "my-passphrase-from-kubernetes",
	}
	args := ProviderInitArgs{
		Tenant:  "tenant",
		Config:  nil,
		Secrets: secrets,
	}
	volumeID := "csi-vol-1b00f5f8-b1c1-11e9-8421-9243c1f659f0"

	kms, err := initSecretsMetadataKMS(args)
	require.NoError(t, err)
	require.NotNil(t, kms)

	// plainDEK is the (LUKS) passphrase for the volume
	plainDEK := "usually created with generateNewEncryptionPassphrase()"

	ctx := context.TODO()

	encryptedDEK, err := kms.EncryptDEK(ctx, volumeID, plainDEK)
	require.NoError(t, err)
	require.NotEqual(t, "", encryptedDEK)
	require.NotEqual(t, plainDEK, encryptedDEK)

	// with an incorrect volumeID, decrypting should fail
	decryptedDEK, err := kms.DecryptDEK(ctx, "incorrect-volumeID", encryptedDEK)
	require.Error(t, err)
	require.Equal(t, "", decryptedDEK)
	require.NotEqual(t, plainDEK, decryptedDEK)

	// with the right volumeID, decrypting should return the plainDEK
	decryptedDEK, err = kms.DecryptDEK(ctx, volumeID, encryptedDEK)
	require.NoError(t, err)
	require.NotEqual(t, "", decryptedDEK)
	require.Equal(t, plainDEK, decryptedDEK)
}

func TestSecretsMetadataKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeSecretsMetadata]
	require.True(t, ok)
}
