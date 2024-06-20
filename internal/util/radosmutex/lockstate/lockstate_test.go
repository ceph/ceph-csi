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

package lockstate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLockStateBytes(t *testing.T) {
	t.Parallel()

	var (
		LockStateUnlockedBytes = []byte{0}
		LockStateLockedBytes   = []byte{1}

		expectedBytes = [][]byte{LockStateUnlockedBytes, LockStateLockedBytes}
		LockStates    = []LockState{Unlocked, Locked}

		LockStateInvalidBytes   = []byte{0xFF}
		LockStateWrongSizeBytes = []byte{0, 0, 0, 0, 1}
	)

	t.Run("ToBytes", func(ts *testing.T) {
		ts.Parallel()

		for i := range expectedBytes {
			bs := ToBytes(LockStates[i])
			require.Equal(ts, expectedBytes[i], bs)
		}
	})

	t.Run("FromBytes", func(ts *testing.T) {
		ts.Parallel()

		for i := range LockStates {
			LockState, err := FromBytes(expectedBytes[i])
			require.NoError(ts, err)
			require.Equal(ts, LockStates[i], LockState)
		}

		_, err := FromBytes(LockStateInvalidBytes)
		require.Error(ts, err)

		_, err = FromBytes(LockStateWrongSizeBytes)
		require.Error(ts, err)
	})
}
