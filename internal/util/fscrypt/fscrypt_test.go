/*
Copyright 2025 The Ceph-CSI Authors.

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
package fscrypt

import (
	"errors"
	"testing"
)

func TestResizePassphrase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		passphrase string
		size       int
		ret        string
		err        error
	}{
		{
			"matching passphrase size",
			"secret",
			6,
			"secret",
			nil,
		},
		{
			"short passphrase",
			"secret",
			64,
			"secretsecretsecretsecretsecretsecretsecretsecretsecretsecretsecr",
			nil,
		},
		{
			"long passphrase",
			"secret",
			2,
			"se",
			nil,
		},
		{
			"half a passphrase",
			"secret",
			3,
			"sec",
			nil,
		},
		{
			"a little too shot passphrase",
			"secret",
			7,
			"secrets",
			nil,
		},
		{
			"empty passphrase",
			"",
			16,
			"",
			ErrEmptyPassphrase,
		},
		{
			"zero length requested",
			"secret",
			0,
			"",
			ErrEmptyPassphrase,
		},
		{
			"negative length requested",
			"secret",
			-32,
			"",
			ErrEmptyPassphrase,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ret, err := resizePassphrase(tt.passphrase, tt.size)
			if ret != tt.ret {
				t.Errorf("resizePassphrase() returned %q of %d bytes, expected %q of %d bytes", tt.ret, len(tt.ret), ret, len(ret))
			}
			if !errors.Is(err, tt.err) {
				t.Errorf("resizePassphrase() returned %v as error, expected %v", err, tt.err)
			}
		})
	}
}
