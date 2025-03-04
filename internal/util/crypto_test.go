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
	"context"
	"encoding/base64"
	"testing"

	"github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/pkg/util/crypto"

	"github.com/stretchr/testify/require"
)

func TestGenerateNewEncryptionPassphrase(t *testing.T) {
	t.Parallel()
	b64Passphrase, err := generateNewEncryptionPassphrase(defaultEncryptionPassphraseSize)
	require.NoError(t, err)

	// b64Passphrase is URL-encoded, decode to verify the length of the
	// passphrase
	passphrase, err := base64.URLEncoding.DecodeString(b64Passphrase)
	require.NoError(t, err)
	require.Len(t, passphrase, defaultEncryptionPassphraseSize)
}

func TestKMSWorkflow(t *testing.T) {
	t.Parallel()
	secrets := map[string]string{
		// FIXME: use encryptionPassphraseKey from SecretsKMS
		"encryptionPassphrase": "workflow test",
	}

	kmsProvider, err := kms.GetDefaultKMS(secrets)
	require.NoError(t, err)
	require.NotNil(t, kmsProvider)

	ve, err := NewVolumeEncryption("", kmsProvider)
	require.NoError(t, err)
	require.NotNil(t, ve)
	require.Equal(t, kms.DefaultKMSType, ve.GetID())

	volumeID := "volume-id"
	ctx := context.TODO()

	err = ve.StoreNewCryptoPassphrase(ctx, volumeID, defaultEncryptionPassphraseSize)
	require.NoError(t, err)

	passphrase, err := ve.GetCryptoPassphrase(ctx, volumeID)
	require.NoError(t, err)
	require.Equal(t, secrets["encryptionPassphrase"], passphrase)
}

func TestFetchEncryptionType(t *testing.T) {
	t.Parallel()
	volOpts := map[string]string{}
	require.EqualValues(t, crypto.EncryptionTypeBlock, FetchEncryptionType(volOpts, crypto.EncryptionTypeBlock))
	require.EqualValues(t, crypto.EncryptionTypeFile, FetchEncryptionType(volOpts, crypto.EncryptionTypeFile))
	require.EqualValues(t, crypto.EncryptionTypeNone, FetchEncryptionType(volOpts, crypto.EncryptionTypeNone))
	volOpts["encryptionType"] = ""
	require.EqualValues(t, crypto.EncryptionTypeInvalid, FetchEncryptionType(volOpts, crypto.EncryptionTypeNone))
	volOpts["encryptionType"] = "block"
	require.EqualValues(t, crypto.EncryptionTypeBlock, FetchEncryptionType(volOpts, crypto.EncryptionTypeNone))
	volOpts["encryptionType"] = "file"
	require.EqualValues(t, crypto.EncryptionTypeFile, FetchEncryptionType(volOpts, crypto.EncryptionTypeNone))
	volOpts["encryptionType"] = "INVALID"
	require.EqualValues(t, crypto.EncryptionTypeInvalid, FetchEncryptionType(volOpts, crypto.EncryptionTypeNone))
}
