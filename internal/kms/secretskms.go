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

package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	"golang.org/x/crypto/scrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Encryption passphrase location in K8s secrets.
	encryptionPassphraseKey = "encryptionPassphrase"

	// kmsTypeSecretsMetadata is the secretKMS with per-volume encryption,
	// where the DEK is stored in the metadata of the volume itself.
	kmsTypeSecretsMetadata = "metadata"

	// metadataSecretNameKey contains the key which corresponds to the
	// kubernetes secret name from where encryptionPassphrase is feteched.
	metadataSecretNameKey = "secretName"
	// metadataSecretNamespaceKey contains the key which corresponds to the
	// kubernetes secret namespace from where encryptionPassphrase is feteched.
	metadataSecretNamespaceKey = "secretNamespace"
)

// secretsKMS is default KMS implementation that means no KMS is in use.
type secretsKMS struct {
	integratedDEK

	passphrase string
}

var _ = RegisterProvider(Provider{
	UniqueID:    DefaultKMSType,
	Initializer: newSecretsKMS,
})

// newSecretsKMS initializes a secretsKMS that uses the passphrase from the
// secret that is configured for the StorageClass. This KMS provider uses a
// single (LUKS) passhprase for all volumes.
func newSecretsKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	passphraseValue, ok := args.Secrets[encryptionPassphraseKey]
	if !ok {
		return nil, errors.New("missing encryption passphrase in secrets")
	}

	return secretsKMS{passphrase: passphraseValue}, nil
}

// Destroy frees all used resources.
func (kms secretsKMS) Destroy() {
	// nothing to do
}

// FetchDEK returns passphrase from Kubernetes secrets.
func (kms secretsKMS) FetchDEK(key string) (string, error) {
	return kms.passphrase, nil
}

// StoreDEK does nothing, as there is no passphrase per key (volume), so
// no need to store is anywhere.
func (kms secretsKMS) StoreDEK(key, value string) error {
	return nil
}

// RemoveDEK is doing nothing as no new passphrases are saved with
// secretsKMS.
func (kms secretsKMS) RemoveDEK(key string) error {
	return nil
}

// secretsMetadataKMS is a KMS based on the secretKMS, but stores the
// Data-Encryption-Key (DEK) in the metadata of the volume.
type secretsMetadataKMS struct {
	secretsKMS
}

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeSecretsMetadata,
	Initializer: initSecretsMetadataKMS,
})

// initSecretsMetadataKMS initializes a secretsMetadataKMS that wraps a secretKMS,
// so that the passphrase from the user provided or StorageClass secrets can be used
// for encrypting/decrypting DEKs that are stored in a detached DEKStore.
func initSecretsMetadataKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	var (
		smKMS                secretsMetadataKMS
		encryptionPassphrase string
		ok                   bool
		err                  error
	)

	encryptionPassphrase, err = smKMS.fetchEncryptionPassphrase(
		args.Config, args.Tenant)
	if err != nil {
		if !errors.Is(err, errConfigOptionMissing) {
			return nil, err
		}
		// if 'userSecret' option is not specified, fetch encryptionPassphrase
		// from storageclass secrets.
		encryptionPassphrase, ok = args.Secrets[encryptionPassphraseKey]
		if !ok {
			return nil, fmt.Errorf(
				"missing %q in storageclass secret", encryptionPassphraseKey)
		}
	}
	smKMS.secretsKMS = secretsKMS{passphrase: encryptionPassphrase}

	return smKMS, nil
}

// fetchEncryptionPassphrase fetches encryptionPassphrase from user provided secret.
func (kms secretsMetadataKMS) fetchEncryptionPassphrase(
	config map[string]interface{},
	defaultNamespace string,
) (string, error) {
	var (
		secretName      string
		secretNamespace string
	)

	err := setConfigString(&secretName, config, metadataSecretNameKey)
	if err != nil {
		return "", err
	}

	err = setConfigString(&secretNamespace, config, metadataSecretNamespaceKey)
	if err != nil {
		if !errors.Is(err, errConfigOptionMissing) {
			return "", err
		}
		// if 'secretNamespace' option is not specified, defaults to namespace in
		// which PVC was created
		secretNamespace = defaultNamespace
	}

	c, err := k8s.NewK8sClient()
	if err != nil {
		return "", fmt.Errorf("can not get Secret %s/%s, failed to "+
			"connect to Kubernetes: %w", secretNamespace, secretName, err)
	}

	secret, err := c.CoreV1().Secrets(secretNamespace).Get(context.TODO(),
		secretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get Secret %s/%s: %w",
			secretNamespace, secretName, err)
	}

	passphraseValue, ok := secret.Data[encryptionPassphraseKey]
	if !ok {
		return "", fmt.Errorf("missing %q in Secret %s/%s",
			encryptionPassphraseKey, secretNamespace, secretName)
	}

	return string(passphraseValue), nil
}

// Destroy frees all used resources.
func (kms secretsMetadataKMS) Destroy() {
	kms.secretsKMS.Destroy()
}

func (kms secretsMetadataKMS) RequiresDEKStore() DEKStoreType {
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
// the secretsKMS and the volumeID.
// The resulting encryptedDEK contains a JSON with the encrypted DEK and the
// nonce that was used for encrypting.
func (kms secretsMetadataKMS) EncryptDEK(volumeID, plainDEK string) (string, error) {
	// use the passphrase from the secretKMS
	passphrase, err := kms.secretsKMS.FetchDEK(volumeID)
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

// DecryptDEK takes the JSON formatted `encryptedMetadataDEK` contents, and it
// fetches secretKMS passphrase to decrypt the DEK.
func (kms secretsMetadataKMS) DecryptDEK(volumeID, encryptedDEK string) (string, error) {
	// use the passphrase from the secretKMS
	passphrase, err := kms.secretsKMS.FetchDEK(volumeID)
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

func (kms secretsMetadataKMS) GetSecret(volumeID string) (string, error) {
	// use the passphrase from the secretKMS
	return kms.secretsKMS.FetchDEK(volumeID)
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
