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

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestV1RefCountBytes(t *testing.T) {
	t.Parallel()

	var (
		refCountBytes          = []byte{0x0, 0x0, 0x0, 0x7B}
		refCountValue          = refCount(123)
		wrongSizeRefCountBytes = []byte{0, 0, 1}
	)

	t.Run("ToBytes", func(ts *testing.T) {
		ts.Parallel()

		bs := refCountValue.toBytes()
		assert.Equal(ts, refCountBytes, bs)
	})

	t.Run("FromBytes", func(ts *testing.T) {
		ts.Parallel()

		rc, err := refCountFromBytes(refCountBytes)
		assert.NoError(ts, err)
		assert.Equal(ts, refCountValue, rc)

		_, err = refCountFromBytes(wrongSizeRefCountBytes)
		assert.Error(ts, err)
	})
}
