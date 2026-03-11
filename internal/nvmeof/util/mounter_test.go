/*
Copyright 2026 The Ceph-CSI Authors.

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

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNVMEDeviceFromRawSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"/dev/nvme0n2", "/dev/nvme0n2"},
		{"devtmpfs[/nvme1n1]", "/dev/nvme1n1"},
		{"devtmpfs[/nvme0n1]", "/dev/nvme0n1"},
		{"devtmpfs[/sda]", ""},
		{"blabla", ""},
	}

	for _, test := range tests {
		result := parseNVMEDeviceFromRawSource(test.input)
		require.Equal(t, test.expected, result)
	}
}
