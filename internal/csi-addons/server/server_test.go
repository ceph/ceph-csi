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

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCSIAddonsServer(t *testing.T) {
	t.Parallel()

	t.Run("valid endpoint", func(t *testing.T) {
		t.Parallel()

		cas, err := NewCSIAddonsServer("unix:///tmp/csi-addons.sock")
		require.NoError(t, err)
		require.NotNil(t, cas)
	})

	t.Run("empty endpoint", func(t *testing.T) {
		t.Parallel()

		cas, err := NewCSIAddonsServer("")
		require.Error(t, err)
		assert.Nil(t, cas)
	})

	t.Run("no UDS endpoint", func(t *testing.T) {
		t.Parallel()

		cas, err := NewCSIAddonsServer("endpoint at /tmp/...")
		require.Error(t, err)
		assert.Nil(t, cas)
	})
}
