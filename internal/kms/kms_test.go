/*
Copyright 2021 The Ceph-CSI Authors.

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func noinitKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	return nil, nil
}

func TestRegisterProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		provider Provider
		panics   bool
	}{{
		Provider{
			UniqueID: "incomplete-provider",
		},
		true,
	}, {
		Provider{
			UniqueID:    "initializer-only",
			Initializer: noinitKMS,
		},
		false,
	}}

	for _, test := range tests {
		provider := test.provider
		if test.panics {
			assert.Panics(t, func() { RegisterProvider(provider) })
		} else {
			assert.True(t, RegisterProvider(provider))
		}
	}
}
