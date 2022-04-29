/*
Copyright 2022 SUSE LLC.
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

package cephfs

import (
	"context"
	"errors"
	"testing"

	kmsapi "github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/stretchr/testify/assert"
)

func TestGetPassphraseFromKMS(t *testing.T) {
	t.Parallel()

	for _, provider := range kmsapi.GetKMSProviderList() {
		if provider.CreateTestDummy == nil {
			continue
		}
		kms := kmsapi.GetKMSTestDummy(provider.UniqueID)
		assert.NotNil(t, kms)

		volEnc, err := util.NewVolumeEncryption(provider.UniqueID, kms)
		if errors.Is(err, util.ErrDEKStoreNeeded) {
			_, err = volEnc.KMS.GetSecret("")
			if errors.Is(err, kmsapi.ErrGetSecretUnsupported) {
				continue // currently unsupported by fscrypt integration
			}
		}
		assert.NotNil(t, volEnc)

		pass, err := getPassphrase(context.TODO(), *volEnc, "")
		assert.NoError(t, err)
		assert.NotEmpty(t, pass)
	}
}
