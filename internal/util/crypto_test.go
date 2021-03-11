/*
Copyright 2021 Ceph-CSI authors.

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
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitSecretsKMS(t *testing.T) {
	secrets := map[string]string{}

	// no passphrase in the secrets, should fail
	kms, err := initSecretsKMS(secrets)
	assert.Error(t, err)
	assert.Nil(t, kms)

	// set a passphrase and it should pass
	secrets[encryptionPassphraseKey] = "plaintext encryption key"
	kms, err = initSecretsKMS(secrets)
	assert.NotNil(t, kms)
	assert.NoError(t, err)
}

func TestGenerateNewEncryptionPassphrase(t *testing.T) {
	b64Passphrase, err := generateNewEncryptionPassphrase()
	require.NoError(t, err)

	// b64Passphrase is URL-encoded, decode to verify the length of the
	// passphrase
	passphrase, err := base64.URLEncoding.DecodeString(b64Passphrase)
	assert.NoError(t, err)
	assert.Equal(t, encryptionPassphraseSize, len(passphrase))
}

func TestKMSWorkflow(t *testing.T) {
	secrets := map[string]string{
		encryptionPassphraseKey: "workflow test",
	}

	kms, err := GetKMS("tenant", defaultKMSType, secrets)
	assert.NoError(t, err)
	require.NotNil(t, kms)
	assert.Equal(t, defaultKMSType, kms.GetID())

	volumeID := "volume-id"

	err = StoreNewCryptoPassphrase(volumeID, kms)
	assert.NoError(t, err)

	passphrase, err := GetCryptoPassphrase(volumeID, kms)
	assert.NoError(t, err)
	assert.Equal(t, secrets[encryptionPassphraseKey], passphrase)
}
