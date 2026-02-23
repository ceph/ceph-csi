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

func TestGenerateKeyFromPassphrase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		passphrase string
		salt       string
		wantErr    bool
	}{
		{
			name:       "basic valid inputs",
			passphrase: "my-super-secret-passphrase",
			salt:       "some-random-generated-salt",
			wantErr:    false,
		},
		{
			name:       "realistic volume ID as salt",
			passphrase: "kubernetes-secret-passphrase",
			salt:       "csi-vol-1b00f5f8-b1c1-11e9-8421-9243c1f659f0",
			wantErr:    false,
		},
		{
			name:       "empty passphrase",
			passphrase: "",
			salt:       "some-salt",
			wantErr:    false,
		},
		{
			name:       "empty salt",
			passphrase: "some-passphrase",
			salt:       "",
			wantErr:    false,
		},
		{
			name:       "both empty",
			passphrase: "",
			salt:       "",
			wantErr:    false,
		},
		{
			name:       "special characters in passphrase",
			passphrase: "p@ssw0rd!#$%^&*()",
			salt:       "salt",
			wantErr:    false,
		},
		{
			name:       "very long passphrase",
			passphrase: "this-is-a-very-long-passphrase-that-goes-on-and-on-and-on-with-many-characters",
			salt:       "short",
			wantErr:    false,
		},
		{
			name:       "very long salt",
			passphrase: "short",
			salt:       "this-is-a-very-long-salt-that-goes-on-and-on-and-on-with-many-characters",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, err := generateKeyFromPassphrase(tt.passphrase, tt.salt)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, key)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, key)
			require.Len(t, key, 32, "key should be of 256 bits")

			// We should get the same output for same input.
			key2, err2 := generateKeyFromPassphrase(tt.passphrase, tt.salt)
			require.NoError(t, err2)
			require.Equal(t, key, key2, "same inputs should produce identical keys")
		})
	}
}

func TestInitSecretsMetadataKMS(t *testing.T) {
	t.Parallel()
	args := ProviderInitArgs{
		Tenant:  "tenant",
		Config:  nil,
		Secrets: map[string]string{},
	}

	kms, err := initSecretsMetadataKMS(args)
	require.NoError(t, err)
	require.NotNil(t, kms)
	require.Equal(t, DEKStoreMetadata, kms.RequiresDEKStore())
}

func TestWorkflowSecretsMetadataKMS(t *testing.T) {
	t.Parallel()
	args := ProviderInitArgs{
		Tenant:  "tenant",
		Config:  nil,
		Secrets: map[string]string{},
	}
	volumeID := "csi-vol-1b00f5f8-b1c1-11e9-8421-9243c1f659f0"

	kms, err := initSecretsMetadataKMS(args)
	require.NoError(t, err)
	require.NotNil(t, kms)
	require.Equal(t, DEKStoreMetadata, kms.RequiresDEKStore())

	// plainDEK is the (LUKS) passphrase for the volume
	plainDEK := "usually created with generateNewEncryptionPassphrase()"

	ctx := t.Context()

	// with missing encryptionPassphraseKey, encrypting should fail
	_, err = kms.EncryptDEK(ctx, volumeID, plainDEK)
	require.Error(t, err)

	secrets := map[string]string{
		encryptionPassphraseKey: "my-passphrase-from-kubernetes",
	}
	args = ProviderInitArgs{
		Tenant:  "tenant",
		Config:  nil,
		Secrets: secrets,
	}

	kms, err = initSecretsMetadataKMS(args)
	require.NoError(t, err)
	require.NotNil(t, kms)

	encryptedDEK, err := kms.EncryptDEK(ctx, volumeID, plainDEK)
	require.NoError(t, err)
	require.NotEmpty(t, encryptedDEK)
	require.NotEqual(t, plainDEK, encryptedDEK)

	// with an incorrect volumeID, decrypting should fail
	decryptedDEK, err := kms.DecryptDEK(ctx, "incorrect-volumeID", encryptedDEK)
	require.Error(t, err)
	require.Empty(t, decryptedDEK)
	require.NotEqual(t, plainDEK, decryptedDEK)

	// with the right volumeID, decrypting should return the plainDEK
	decryptedDEK, err = kms.DecryptDEK(ctx, volumeID, encryptedDEK)
	require.NoError(t, err)
	require.NotEmpty(t, decryptedDEK)
	require.Equal(t, plainDEK, decryptedDEK)
}

func TestSecretsMetadataKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeSecretsMetadata]
	require.True(t, ok)
}
