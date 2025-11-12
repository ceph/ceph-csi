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

package cryptsetup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRecommendation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		testName string

		cipher                 string
		keySize                uint
		integrityMode          string
		expectedRecommendation RecommendationLevel
	}{
		// Update the tests as needed when the recommendationConfig is changed.
		{
			testName:               "Invalid Recommendation no Cipher",
			cipher:                 "aes-128",
			expectedRecommendation: InvalidRecommended,
		},
		{
			testName:               "Not recommended",
			cipher:                 "aes-xts-random",
			keySize:                512,
			integrityMode:          "hxmax-512",
			expectedRecommendation: NotRecommended,
		},
		{
			testName:               "Unknown Recommendation with uncommon key size",
			cipher:                 "aes-xts-plain64",
			keySize:                321,
			expectedRecommendation: NotRecommended,
		},
		{
			testName:               "Recommended Configuration",
			cipher:                 "aes-xts-random",
			keySize:                512,
			integrityMode:          "hmac-sha256",
			expectedRecommendation: Recommended,
		},
		{
			testName:               "Unknown Configuration unknown integrity mode",
			cipher:                 "aes-xts-random",
			keySize:                512,
			integrityMode:          "hac-256",
			expectedRecommendation: NotRecommended,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			assertion := assert.New(t)
			options := EncryptionOptions{cipher: tt.cipher, keysize: &tt.keySize, integrityMode: &tt.integrityMode}
			recommendation := GetRecommendation(options)
			assertion.Equal(tt.expectedRecommendation, recommendation)
		})
	}
}

func TestAllowedEncryptionOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		testName               string
		cipher                 string
		expectedErrorCipher    bool
		keySize                string
		expectedErrorKeySize   bool
		integritymode          string
		expectedErrorIntegrity bool
	}{
		{
			testName:               "Invalid Options",
			cipher:                 "cipherName",
			expectedErrorCipher:    true,
			keySize:                "number",
			expectedErrorKeySize:   true,
			integritymode:          "modes",
			expectedErrorIntegrity: true,
		},
		{
			testName:               "Valid Cipher, key size and invalid integrity mode",
			cipher:                 "aes-xts-random",
			expectedErrorCipher:    false,
			keySize:                "512",
			expectedErrorKeySize:   false,
			integritymode:          "hmax-4523",
			expectedErrorIntegrity: true,
		},
		{
			testName:               "Valid Cipher, key size and integrity mode",
			cipher:                 "aes-xts-random",
			expectedErrorCipher:    false,
			keySize:                "512",
			expectedErrorKeySize:   false,
			integritymode:          "hmac-sha256",
			expectedErrorIntegrity: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			requirement := require.New(t)
			options := EncryptionOptions{}
			setError(requirement, options.SetCipher, tt.cipher, tt.expectedErrorCipher)
			setError(requirement, options.SetKeySize, tt.keySize, tt.expectedErrorKeySize)
			setError(requirement, options.SetIntegrityMode, tt.integritymode, tt.expectedErrorIntegrity)
		})
	}
}

func TestLuksStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		testName                      string
		cipher                        string
		expectedErrorCipher           bool
		inputKeySize                  string
		expectedErrorKeySize          bool
		integritymode                 string
		expectedErrorIntegrity        bool
		expectedIntegrityMode         string
		integrityKeySize              string
		expectedIntegrityKeySizeError bool
		expectedCipherKeySize         uint
		expectedIntegrityKeySize      uint
	}{
		{
			testName:                      "Invalid Options",
			cipher:                        "cipherName",
			expectedErrorCipher:           false,
			inputKeySize:                  "number",
			expectedErrorKeySize:          true,
			integritymode:                 "modes",
			expectedErrorIntegrity:        true,
			integrityKeySize:              "number",
			expectedIntegrityKeySizeError: true,
		},
		{
			testName:                      "Valid Status",
			cipher:                        "aes-xts-random",
			expectedErrorCipher:           false,
			inputKeySize:                  "1024",
			expectedErrorKeySize:          false,
			integritymode:                 "hmac(sha512)", // different integrity mode structure for a luks status
			expectedErrorIntegrity:        false,
			expectedIntegrityMode:         "hmac-sha512",
			integrityKeySize:              "512",
			expectedIntegrityKeySizeError: false,
			expectedCipherKeySize:         512, // in a valid status the input key size is the sum of
			// cipher key size of the integrity key size
			expectedIntegrityKeySize: 512,
		},
		{
			testName:                      "Invalid Status with wrong Integrity Mode",
			cipher:                        "aes-xts-random",
			expectedErrorCipher:           false,
			inputKeySize:                  "1024",
			expectedErrorKeySize:          false,
			integritymode:                 "hmac()", // different integrity mode structure for a luks status
			expectedErrorIntegrity:        true,
			expectedIntegrityMode:         "hmac-sha512",
			integrityKeySize:              "512",
			expectedIntegrityKeySizeError: false,
			expectedCipherKeySize:         512,
			expectedIntegrityKeySize:      512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			assertion := assert.New(t)
			requirement := require.New(t)
			status := LuksStatus{}
			// No error thrown for SetCipher()
			setError(requirement, status.SetCipher, tt.cipher, tt.expectedErrorCipher)
			setError(requirement, status.SetKeySize, tt.inputKeySize, tt.expectedErrorKeySize)
			setError(requirement, status.SetIntegrityModeFromLuks, tt.integritymode, tt.expectedErrorIntegrity)
			setError(requirement, status.SetIntegrityKeySize, tt.integrityKeySize, tt.expectedIntegrityKeySizeError)

			getEqual(assertion, status.Cipher, tt.expectedErrorCipher, tt.cipher)
			getEqual(assertion, status.IntegrityKeySize, tt.expectedIntegrityKeySizeError, &tt.expectedIntegrityKeySize)
			getEqual(assertion, status.IntegrityMode, tt.expectedErrorIntegrity, &tt.expectedIntegrityMode)
			result, err := status.CipherKeySize()
			if tt.expectedErrorCipher {
				requirement.Error(err)
			} else {
				requirement.NoError(err)
				assertion.Equal(tt.expectedCipherKeySize, result)
			}

			options := EncryptionOptions{
				cipher:        tt.cipher,
				keysize:       &tt.expectedCipherKeySize,
				integrityMode: &tt.expectedIntegrityMode,
			}
			isEqual, err := options.Equal(status)
			if !tt.expectedErrorCipher &&
				!tt.expectedErrorIntegrity &&
				!tt.expectedErrorKeySize &&
				!tt.expectedIntegrityKeySizeError {
				requirement.NoError(err)
				assertion.True(isEqual)
			}

			unEqualOption := EncryptionOptions{cipher: tt.integritymode}
			isEqual, err = unEqualOption.Equal(status)
			if !tt.expectedErrorCipher &&
				!tt.expectedErrorIntegrity &&
				!tt.expectedErrorKeySize &&
				!tt.expectedIntegrityKeySizeError {
				requirement.NoError(err)
				assertion.False(isEqual)
			}
		})
	}
}

func setError(requirement *require.Assertions, set func(string) error, argument string, hasError bool) {
	err := set(argument)
	if hasError {
		requirement.Error(err)
	} else {
		requirement.NoError(err)
	}
}

func getEqual[T any](assertion *assert.Assertions, get func() T, hasError bool, expected T) {
	if hasError {
		return
	}
	result := get()
	assertion.Equal(expected, result)
}
