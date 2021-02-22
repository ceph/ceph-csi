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
	"errors"
)

const (
	// Encryption passphrase location in K8s secrets
	encryptionPassphraseKey = "encryptionPassphrase"

	// Default KMS type
	defaultKMSType = "default"
)

// SecretsKMS is default KMS implementation that means no KMS is in use.
type SecretsKMS struct {
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

// GetPassphrase returns passphrase from Kubernetes secrets.
func (kms SecretsKMS) GetPassphrase(key string) (string, error) {
	return kms.passphrase, nil
}

// SavePassphrase does nothing, as there is no passphrase per key (volume), so
// no need to store is anywhere.
func (kms SecretsKMS) SavePassphrase(key, value string) error {
	return nil
}

// DeletePassphrase is doing nothing as no new passphrases are saved with
// SecretsKMS.
func (kms SecretsKMS) DeletePassphrase(key string) error {
	return nil
}
