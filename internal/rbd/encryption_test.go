/*
Copyright 2023 The Ceph-CSI Authors.

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

package rbd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/util/cryptsetup"
	"github.com/ceph/ceph-csi/pkg/util/crypto"
)

func TestParseEncryptionOpts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		testName     string
		volOptions   map[string]string
		fallbackType crypto.EncryptionType
		expectedKMS  string
		expectedEnc  crypto.EncryptionType
		expectedErr  bool
	}{
		{
			testName: "No Encryption Option",
			volOptions: map[string]string{
				"foo": "bar",
			},
			fallbackType: crypto.EncryptionTypeBlock,
			expectedKMS:  "",
			expectedEnc:  crypto.EncryptionTypeNone,
			expectedErr:  false,
		},
		{
			testName: "Encrypted as false",
			volOptions: map[string]string{
				"encrypted": "false",
			},
			fallbackType: crypto.EncryptionTypeBlock,
			expectedKMS:  "",
			expectedEnc:  crypto.EncryptionTypeNone,
			expectedErr:  false,
		},
		{
			testName: "Encrypted as invalid string",
			volOptions: map[string]string{
				"encrypted": "notbool",
			},
			fallbackType: crypto.EncryptionTypeBlock,
			expectedKMS:  "",
			expectedEnc:  crypto.EncryptionTypeInvalid,
			expectedErr:  true,
		},
		{
			testName: "Valid Encryption Option With KMS ID",
			volOptions: map[string]string{
				"encrypted":       "true",
				"encryptionKMSID": "valid-kms-id",
			},
			fallbackType: crypto.EncryptionTypeBlock,
			expectedKMS:  "valid-kms-id",
			expectedEnc:  crypto.EncryptionTypeBlock,
			expectedErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			actualKMS, actualEnc, actualErr := ParseEncryptionOpts(
				tt.volOptions,
				tt.fallbackType,
			)
			if actualKMS != tt.expectedKMS {
				t.Errorf("Expected KMS ID: %s, but got: %s", tt.expectedKMS, actualKMS)
			}

			if actualEnc != tt.expectedEnc {
				t.Errorf("Expected Encryption Type: %v, but got: %v", tt.expectedEnc, actualEnc)
			}

			if (actualErr != nil) != tt.expectedErr {
				t.Errorf("expected error %v but got %v", tt.expectedErr, actualErr)
			}
		})
	}
}

func TestParseCipherOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		testName           string
		volOptions         map[string]string
		expectedCipher     *string
		expectedIntegrity  *string
		expectedKeySize    *string
		expectedSectorSize *string
		expectedErr        bool
	}{
		{
			testName: "No Encryption Option",
			volOptions: map[string]string{
				"foo": "bar",
			},
			expectedCipher:    nil,
			expectedIntegrity: nil,
			expectedKeySize:   nil,
			expectedErr:       false,
		},
		{
			testName: "Encryption Options with allowed cipher",
			volOptions: map[string]string{
				"encryptionCipher": "aes-xts-plain64",
			},
			expectedCipher:    valueToPointer("aes-xts-plain64"),
			expectedIntegrity: nil,
			expectedKeySize:   nil,
			expectedErr:       false,
		},
		{
			testName: "Encryption Options with allowed config",
			volOptions: map[string]string{
				"encryptionCipher":  "aes-xts-plain64",
				"integrityMode":     "hmac-sha256",
				"encryptionKeySize": "512",
			},
			expectedCipher:    valueToPointer("aes-xts-plain64"),
			expectedIntegrity: valueToPointer("hmac-sha256"),
			expectedKeySize:   valueToPointer("512"),
			expectedErr:       false,
		},
		{
			testName: "Encryption Options with not allowed cipher",
			volOptions: map[string]string{
				// AES-GCM is not secure for disk encryption
				"encryptionCipher":  "aes-gcm",
				"integrityMode":     "aead",
				"encryptionKeySize": "256",
			},
			expectedCipher:    nil,
			expectedIntegrity: nil,
			expectedKeySize:   nil,
			expectedErr:       true,
		},
		{
			testName: "Encryption Options with sector size",
			volOptions: map[string]string{
				// AES-GCM is not secure for disk encryption
				"encryptionCipher":     "aes-xts-plain64",
				"integrityMode":        "hmac-sha256",
				"encryptionKeySize":    "512",
				"encryptionSectorSize": "4096",
			},
			expectedCipher:     valueToPointer("aes-xts-plain64"),
			expectedIntegrity:  valueToPointer("hmac-sha256"),
			expectedKeySize:    valueToPointer("512"),
			expectedSectorSize: valueToPointer("4096"),
			expectedErr:        false,
		},
		// test case key size or integrity mode set
		// will result in no encryption
	}

	for _, tt := range tests {
		t.Run(
			tt.testName,
			func(t *testing.T) {
				t.Parallel()
				assertion := assert.New(t)
				requirement := require.New(t)
				actualEncOptions, actualErr := parseCipherOptions(
					tt.volOptions,
				)
				if tt.expectedErr {
					requirement.Error(actualErr)

					return
				}
				requirement.NoError(actualErr)
				if tt.expectedCipher == nil {
					assertion.Nil(actualEncOptions)

					return
				}
				expectedEncOption := cryptsetup.EncryptionOptions{}
				err := expectedEncOption.SetCipher(*tt.expectedCipher)
				requirement.NoError(err)
				if tt.expectedKeySize != nil {
					err := expectedEncOption.SetKeySize(*tt.expectedKeySize)
					requirement.NoError(err)
				}
				if tt.expectedIntegrity != nil {
					err := expectedEncOption.SetIntegrityMode(*tt.expectedIntegrity)
					requirement.NoError(err)
				}
				if tt.expectedSectorSize != nil {
					err := expectedEncOption.SetSectorSize(*tt.expectedSectorSize)
					requirement.NoError(err)
				}
				assertion.Equal(expectedEncOption, *actualEncOptions)
			},
		)
	}
}

func valueToPointer[T any](s T) *T {
	return &s
}
