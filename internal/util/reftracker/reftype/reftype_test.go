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

package reftype

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefTypeBytes(t *testing.T) {
	t.Parallel()

	var (
		refTypeNormalBytes = []byte{1}
		refTypeMaskBytes   = []byte{2}

		expectedBytes = [][]byte{refTypeNormalBytes, refTypeMaskBytes}
		refTypes      = []RefType{Normal, Mask}

		refTypeInvalidBytes   = []byte{0xFF}
		refTypeWrongSizeBytes = []byte{0, 0, 0, 0, 1}
	)

	t.Run("ToBytes", func(ts *testing.T) {
		ts.Parallel()

		for i := range expectedBytes {
			bs := ToBytes(refTypes[i])
			assert.Equal(ts, expectedBytes[i], bs)
		}
	})

	t.Run("FromBytes", func(ts *testing.T) {
		ts.Parallel()

		for i := range refTypes {
			refType, err := FromBytes(expectedBytes[i])
			assert.NoError(ts, err)
			assert.Equal(ts, refTypes[i], refType)
		}

		_, err := FromBytes(refTypeInvalidBytes)
		assert.Error(ts, err)

		_, err = FromBytes(refTypeWrongSizeBytes)
		assert.Error(ts, err)
	})
}
