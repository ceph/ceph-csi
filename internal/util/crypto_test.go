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

	"github.com/ceph/ceph-csi/internal/kms"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNewEncryptionPassphrase(t *testing.T) {
	t.Parallel()
	b64Passphrase, err := generateNewEncryptionPassphrase(defaultEncryptionPassphraseSize)
	require.NoError(t, err)

	// b64Passphrase is URL-encoded, decode to verify the length of the
	// passphrase
	passphrase, err := base64.URLEncoding.DecodeString(b64Passphrase)
	assert.NoError(t, err)
	assert.Equal(t, defaultEncryptionPassphraseSize, len(passphrase))
}

func TestKMSWorkflow(t *testing.T) {
	t.Parallel()
	secrets := map[string]string{
		// FIXME: use encryptionPassphraseKey from SecretsKMS
		"encryptionPassphrase": "workflow test",
	}

	kmsProvider, err := kms.GetDefaultKMS(secrets)
	assert.NoError(t, err)
	require.NotNil(t, kmsProvider)

	ve, err := NewVolumeEncryption("", kmsProvider)
	assert.NoError(t, err)
	require.NotNil(t, ve)
	assert.Equal(t, kms.DefaultKMSType, ve.GetID())

	volumeID := "volume-id"

	err = ve.StoreNewCryptoPassphrase(volumeID, defaultEncryptionPassphraseSize)
	assert.NoError(t, err)

	passphrase, err := ve.GetCryptoPassphrase(volumeID)
	assert.NoError(t, err)
	assert.Equal(t, secrets["encryptionPassphrase"], passphrase)
}

func TestEncryptionType(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, EncryptionTypeInvalid, ParseEncryptionType("wat?"))
	assert.EqualValues(t, EncryptionTypeInvalid, ParseEncryptionType("both"))
	assert.EqualValues(t, EncryptionTypeInvalid, ParseEncryptionType("file,block"))
	assert.EqualValues(t, EncryptionTypeInvalid, ParseEncryptionType("block,file"))
	assert.EqualValues(t, EncryptionTypeBlock, ParseEncryptionType("block"))
	assert.EqualValues(t, EncryptionTypeFile, ParseEncryptionType("file"))
	assert.EqualValues(t, EncryptionTypeNone, ParseEncryptionType(""))

	for _, s := range []string{"file", "block", ""} {
		assert.EqualValues(t, s, EncryptionTypeString(ParseEncryptionType(s)))
	}
}

func TestFetchEncryptionType(t *testing.T) {
	t.Parallel()
	volOpts := map[string]string{}
	assert.EqualValues(t, EncryptionTypeBlock, FetchEncryptionType(volOpts, EncryptionTypeBlock))
	assert.EqualValues(t, EncryptionTypeFile, FetchEncryptionType(volOpts, EncryptionTypeFile))
	assert.EqualValues(t, EncryptionTypeNone, FetchEncryptionType(volOpts, EncryptionTypeNone))
	volOpts["encryptionType"] = ""
	assert.EqualValues(t, EncryptionTypeInvalid, FetchEncryptionType(volOpts, EncryptionTypeNone))
	volOpts["encryptionType"] = "block"
	assert.EqualValues(t, EncryptionTypeBlock, FetchEncryptionType(volOpts, EncryptionTypeNone))
	volOpts["encryptionType"] = "file"
	assert.EqualValues(t, EncryptionTypeFile, FetchEncryptionType(volOpts, EncryptionTypeNone))
	volOpts["encryptionType"] = "INVALID"
	assert.EqualValues(t, EncryptionTypeInvalid, FetchEncryptionType(volOpts, EncryptionTypeNone))
}
