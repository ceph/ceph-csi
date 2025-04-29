/*
Copyright 2025 Ceph-CSI authors.

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

package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptionType(t *testing.T) {
	t.Parallel()
	require.Equal(t, EncryptionTypeInvalid, ParseEncryptionType("wat?"))
	require.Equal(t, EncryptionTypeInvalid, ParseEncryptionType("both"))
	require.Equal(t, EncryptionTypeInvalid, ParseEncryptionType("file,block"))
	require.Equal(t, EncryptionTypeInvalid, ParseEncryptionType("block,file"))
	require.Equal(t, EncryptionTypeBlock, ParseEncryptionType("block"))
	require.Equal(t, EncryptionTypeFile, ParseEncryptionType("file"))
	require.Equal(t, EncryptionTypeNone, ParseEncryptionType(""))

	for _, s := range []string{"file", "block", ""} {
		require.Equal(t, s, ParseEncryptionType(s).String())
	}
}
