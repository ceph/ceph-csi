/*
Copyright 2019 The Ceph-CSI Authors.

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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	// Encryption passphrase location in K8s secrets
	encryptionPassphraseKey = "encryptionPassphrase"

	// Default KMS type
	defaultKMSType = "default"

	// kmsTypeSecretsMetadata is the SecretsKMS with per-volume encryption,
	// where the DEK is stored in the metadata of the volume itself.
	kmsTypeSecretsMetadata = "metadata"
)

// SecretsKMS is default KMS implementation that means no KMS is in use.
type SecretsKMS struct {
	integratedDEK

	passphrase string
}

// initSecretsKMS initializes a SecretsKMS that uses the passphrase from the
// secret that is configured for the StorageClass. This KMS provider uses a
// single (LUKS) passhprase for all volumes.
func initSecretsKMS(secrets map[string]string) (EncryptionKMS, error) {
	passphraseValue, ok := secrets[encryptionPassphraseKey]
	if !ok {
		return nil, errors.New("missing encryption passphrase in secrets")
	}
	return SecretsKMS{passphrase: passphraseValue}, nil
}

// GetID is returning ID representing default KMS `default`.
func (kms SecretsKMS) GetID() string {
	return defaultKMSType
}

// Destroy frees all used resources.
func (kms SecretsKMS) Destroy() {
	// nothing to do
}

// FetchDEK returns passphrase from Kubernetes secrets.
func (kms SecretsKMS) FetchDEK(key string) (string, error) {
	return kms.passphrase, nil
}

// StoreDEK does nothing, as there is no passphrase per key (volume), so
// no need to store is anywhere.
func (kms SecretsKMS) StoreDEK(key, value string) error {
	return nil
}

// RemoveDEK is doing nothing as no new passphrases are saved with
// SecretsKMS.
func (kms SecretsKMS) RemoveDEK(key string) error {
	return nil
}

// SecretsMetadataKMS is a KMS based on the SecretsKMS, but stores the
// Data-Encryption-Key (DEK) in the metadata of the volume.
type SecretsMetadataKMS struct {
	SecretsKMS

	encryptionKMSID string
}

// initSecretsMetadataKMS initializes a SecretsMetadataKMS that wraps a
// SecretsKMS, so that the passphrase from the StorageClass secrets can be used
// for encrypting/decrypting DEKs that are stored in a detached DEKStore.
func initSecretsMetadataKMS(encryptionKMSID string, secrets map[string]string) (EncryptionKMS, error) {
	eKMS, err := initSecretsKMS(secrets)
	if err != nil {
		return nil, err
	}

	sKMS, ok := eKMS.(SecretsKMS)
	if !ok {
		return nil, fmt.Errorf("failed to convert %T to SecretsKMS", eKMS)
	}

	smKMS := SecretsMetadataKMS{}
	smKMS.SecretsKMS = sKMS
	smKMS.encryptionKMSID = encryptionKMSID

	return smKMS, nil
}

// GetID is returning ID representing the SecretsMetadataKMS.
func (kms SecretsMetadataKMS) GetID() string {
	return kms.encryptionKMSID
}

// Destroy frees all used resources.
func (kms SecretsMetadataKMS) Destroy() {
	kms.SecretsKMS.Destroy()
}

func (kms SecretsMetadataKMS) requiresDEKStore() DEKStoreType {
	return DEKStoreMetadata
}

// encryptedMetedataDEK contains the encrypted DEK and the Nonce that was used
// during encryption. This structure is stored (in JSON format) in the DEKStore
// that is linked to this KMS provider.
type encryptedMetedataDEK struct {
	// DEK is the encrypted data-encryption-key for the volume.
	DEK []byte `json:"dek"`
	// Nonce is a random byte slice to guarantee the uniqueness of the
	// encrypted DEK.
	Nonce []byte `json:"nonce"`
}

// EncryptDEK encrypts the plainDEK with a key derived from the passphrase from
// the SecretsKMS and the volumeID.
// The resulting encryptedDEK contains a JSON with the encrypted DEK and the
// nonce that was used for encrypting.
func (kms SecretsMetadataKMS) EncryptDEK(volumeID, plainDEK string) (string, error) {
	// use the passphrase from the SecretsKMS
	passphrase, err := kms.SecretsKMS.FetchDEK(volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to get passphrase: %w", err)
	}

	aead, err := generateCipher(passphrase, volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to generate cipher: %w", err)
	}

	emd := encryptedMetedataDEK{}
	emd.Nonce, err = generateNonce(aead.NonceSize())
	if err != nil {
		return "", fmt.Errorf("failed to generated nonce: %w", err)
	}
	emd.DEK = aead.Seal(nil, emd.Nonce, []byte(plainDEK), nil)

	emdData, err := json.Marshal(&emd)
	if err != nil {
		return "", fmt.Errorf("failed to convert "+
			"encryptedMetedataDEK to JSON: %w", err)
	}

	return string(emdData), nil
}

// DecryptDEK takes the JSON formatted `encryptedMetedataDEK` contents, and it
// fetches SecretsKMS passphase to decrypt the DEK.
func (kms SecretsMetadataKMS) DecryptDEK(volumeID, encryptedDEK string) (string, error) {
	// use the passphrase from the SecretsKMS
	passphrase, err := kms.SecretsKMS.FetchDEK(volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to get passphrase: %w", err)
	}

	aead, err := generateCipher(passphrase, volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to generate cipher: %w", err)
	}

	emd := encryptedMetedataDEK{}
	err = json.Unmarshal([]byte(encryptedDEK), &emd)
	if err != nil {
		return "", fmt.Errorf("failed to convert data to "+
			"encryptedMetedataDEK: %w", err)
	}

	dek, err := aead.Open(nil, emd.Nonce, emd.DEK, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	return string(dek), nil
}

// generateCipher returns a AEAD cipher based on a passphrase and salt
// (volumeID). The cipher can then be used to encrypt/decrypt the DEK.
func generateCipher(passphrase, salt string) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), []byte(salt), 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}
	return aead, nil
}

// generateNonce returns a byte slice with random contents.
func generateNonce(size int) ([]byte, error) {
	nonce := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return nonce, nil
}
