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

package util

import (
	"errors"
	"testing"

	kmsapi "github.com/ceph/ceph-csi/internal/kms"

	"github.com/stretchr/testify/assert"
)

func TestGetPassphraseFromKMS(t *testing.T) {
	t.Parallel()

	for _, provider := range kmsapi.GetKMSTestProvider() {
		if provider.CreateTestDummy == nil {
			continue
		}
		kms := kmsapi.GetKMSTestDummy(provider.UniqueID)
		assert.NotNil(t, kms)

		volEnc, err := NewVolumeEncryption(provider.UniqueID, kms)
		if errors.Is(err, ErrDEKStoreNeeded) {
			_, err = volEnc.KMS.GetSecret("")
			if errors.Is(err, kmsapi.ErrGetSecretUnsupported) {
				continue // currently unsupported by fscrypt integration
			}
		}
		assert.NotNil(t, volEnc)

		if kms.RequiresDEKStore() == kmsapi.DEKStoreIntegrated {
			continue
		}

		secret, err := kms.GetSecret("")
		assert.NoError(t, err, provider.UniqueID)
		assert.NotEmpty(t, secret, provider.UniqueID)
	}
}
