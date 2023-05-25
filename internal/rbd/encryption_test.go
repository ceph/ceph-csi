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

	"github.com/ceph/ceph-csi/internal/util"
)

func TestParseEncryptionOpts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		testName     string
		volOptions   map[string]string
		fallbackType util.EncryptionType
		expectedKMS  string
		expectedEnc  util.EncryptionType
		expectedErr  bool
	}{
		{
			testName: "No Encryption Option",
			volOptions: map[string]string{
				"foo": "bar",
			},
			fallbackType: util.EncryptionTypeBlock,
			expectedKMS:  "",
			expectedEnc:  util.EncryptionTypeNone,
			expectedErr:  false,
		},
		{
			testName: "Encrypted as false",
			volOptions: map[string]string{
				"encrypted": "false",
			},
			fallbackType: util.EncryptionTypeBlock,
			expectedKMS:  "",
			expectedEnc:  util.EncryptionTypeNone,
			expectedErr:  false,
		},
		{
			testName: "Encrypted as invalid string",
			volOptions: map[string]string{
				"encrypted": "notbool",
			},
			fallbackType: util.EncryptionTypeBlock,
			expectedKMS:  "",
			expectedEnc:  util.EncryptionTypeInvalid,
			expectedErr:  true,
		},
		{
			testName: "Valid Encryption Option With KMS ID",
			volOptions: map[string]string{
				"encrypted":       "true",
				"encryptionKMSID": "valid-kms-id",
			},
			fallbackType: util.EncryptionTypeBlock,
			expectedKMS:  "valid-kms-id",
			expectedEnc:  util.EncryptionTypeBlock,
			expectedErr:  false,
		},
	}

	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.testName, func(t *testing.T) {
			t.Parallel()
			actualKMS, actualEnc, actualErr := ParseEncryptionOpts(
				newtt.volOptions,
				newtt.fallbackType,
			)
			if actualKMS != newtt.expectedKMS {
				t.Errorf("Expected KMS ID: %s, but got: %s", newtt.expectedKMS, actualKMS)
			}

			if actualEnc != newtt.expectedEnc {
				t.Errorf("Expected Encryption Type: %v, but got: %v", newtt.expectedEnc, actualEnc)
			}

			if (actualErr != nil) != newtt.expectedErr {
				t.Errorf("expected error %v but got %v", newtt.expectedErr, actualErr)
			}
		})
	}
}
