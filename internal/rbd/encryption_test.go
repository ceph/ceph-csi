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
		testName          string
		volOptions        map[string]string
		expectedCipher    *string
		expectedIntegrity *string
		expectedKeySize   *uint
		expectedErr       bool
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
			expectedCipher:    stringp("aes-xts-plain64"),
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
			expectedCipher:    stringp("aes-xts-plain64"),
			expectedIntegrity: stringp("hmac-sha256"),
			expectedKeySize:   uintp(512),
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
	}

	for _, tt := range tests {
		t.Run(
			tt.testName,
			func(t *testing.T) {
				t.Parallel()
				actualEncOptions, actualErr := parseCipherOptions(
					tt.volOptions,
				)
				if tt.expectedErr {
					if actualErr == nil {
						t.Errorf("expected an error but got nil")

						return
					}

					return
				}
				if actualErr != nil {
					t.Errorf("expected no error but got: %v", actualErr)

					return
				}
				expectOptions := tt.expectedCipher != nil
				actualHasOptions := actualEncOptions != nil

				if expectOptions != actualHasOptions {
					if expectOptions {
						t.Errorf("expected options (for cipher %q) but got nil", *tt.expectedCipher)
					} else {
						t.Errorf("expected nil options (no cipher) but got non-nil options")
					}

					return
				}
				if !actualHasOptions {
					return
				}

				actual := actualEncOptions.Cipher()
				comparePtr(t, "Cipher", tt.expectedCipher, &actual)
				comparePtr(t, "IntegrityMode", tt.expectedIntegrity, actualEncOptions.IntegrityMode())
				comparePtr(t, "Key Size", tt.expectedKeySize, actualEncOptions.KeySize())
			},
		)
	}
}

func comparePtr[T comparable](t *testing.T, fieldName string, expected, actual *T) {
	t.Helper()
	switch {
	case expected == nil && actual != nil:
		t.Errorf("%s: expected nil but got %v", fieldName, *actual)
	case expected != nil && actual == nil:
		t.Errorf("%s: expected %v but got nil", fieldName, *expected)
	case expected != nil && actual != nil && *expected != *actual:
		t.Errorf("%s: mismatch, expected %v but got %v", fieldName, *expected, *actual)
	}
}

func stringp(s string) *string {
	return &s
}

func uintp(i uint) *uint {
	return &i
}
