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

package kms

import "encoding/base64"

type TestDummyFunc func() EncryptionKMS

type ProviderTest struct {
	UniqueID        string
	CreateTestDummy TestDummyFunc
}

type kmsTestProviderList struct {
	providers map[string]ProviderTest
}

var kmsTestManager = kmsTestProviderList{providers: map[string]ProviderTest{}}

func RegisterTestProvider(provider ProviderTest) bool {
	kmsTestManager.providers[provider.UniqueID] = provider

	return true
}

func GetKMSTestDummy(kmsID string) EncryptionKMS {
	provider, ok := kmsTestManager.providers[kmsID]
	if !ok {
		return nil
	}

	return provider.CreateTestDummy()
}

func GetKMSTestProvider() map[string]ProviderTest {
	return kmsTestManager.providers
}

func newDefaultTestDummy() EncryptionKMS {
	return secretsKMS{passphrase: base64.URLEncoding.EncodeToString(
		[]byte("test dummy passphrase"))}
}

func newSecretsMetadataTestDummy() EncryptionKMS {
	smKMS := secretsMetadataKMS{}
	smKMS.secretsKMS = secretsKMS{passphrase: base64.URLEncoding.EncodeToString(
		[]byte("test dummy passphrase"))}

	return smKMS
}

var _ = RegisterTestProvider(ProviderTest{
	UniqueID:        kmsTypeSecretsMetadata,
	CreateTestDummy: newSecretsMetadataTestDummy,
})

var _ = RegisterTestProvider(ProviderTest{
	UniqueID:        DefaultKMSType,
	CreateTestDummy: newDefaultTestDummy,
})
