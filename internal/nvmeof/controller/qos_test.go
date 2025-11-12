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

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/nvmeof"
)

func TestParseQoSParameters(t *testing.T) {
	t.Parallel()

	uint64Ptr := func(v uint64) *uint64 { return &v }

	tests := []struct {
		name        string
		params      map[string]string
		expected    *nvmeof.NVMeoFQosVolume
		expectError bool
	}{
		{
			name:     "empty parameters",
			params:   map[string]string{},
			expected: nil,
		},
		{
			name: "all QoS parameters",
			params: map[string]string{
				nvmeof.RwIosPerSecond:    "10000",
				nvmeof.RwMbytesPerSecond: "100",
				nvmeof.RMbytesPerSecond:  "50",
				nvmeof.WMbytesPerSecond:  "50",
			},
			expected: &nvmeof.NVMeoFQosVolume{
				RwIosPerSecond:    uint64Ptr(10000),
				RwMbytesPerSecond: uint64Ptr(100),
				RMbytesPerSecond:  uint64Ptr(50),
				WMbytesPerSecond:  uint64Ptr(50),
			},
		},
		{
			name: "single QoS parameter",
			params: map[string]string{
				nvmeof.RwIosPerSecond: "5000",
			},
			expected: &nvmeof.NVMeoFQosVolume{
				RwIosPerSecond: uint64Ptr(5000),
			},
		},
		{
			name: "zero value (unlimited)",
			params: map[string]string{
				nvmeof.RwIosPerSecond:    "0",
				nvmeof.RwMbytesPerSecond: "100",
			},
			expected: &nvmeof.NVMeoFQosVolume{
				RwIosPerSecond:    uint64Ptr(0),
				RwMbytesPerSecond: uint64Ptr(100),
			},
		},
		{
			name: "partial QoS parameters",
			params: map[string]string{
				nvmeof.RwIosPerSecond:   "10000",
				nvmeof.RMbytesPerSecond: "50",
			},
			expected: &nvmeof.NVMeoFQosVolume{
				RwIosPerSecond:   uint64Ptr(10000),
				RMbytesPerSecond: uint64Ptr(50),
			},
		},
		{
			name: "empty string values ignored",
			params: map[string]string{
				nvmeof.RwIosPerSecond:    "",
				nvmeof.RwMbytesPerSecond: "100",
			},
			expected: &nvmeof.NVMeoFQosVolume{
				RwMbytesPerSecond: uint64Ptr(100),
			},
		},
		{
			name: "invalid number format",
			params: map[string]string{
				nvmeof.RwIosPerSecond: "invalid",
			},
			expectError: true,
		},
		{
			name: "negative number",
			params: map[string]string{
				nvmeof.RwIosPerSecond: "-100",
			},
			expectError: true,
		},
		{
			name: "number too large",
			params: map[string]string{
				nvmeof.RwIosPerSecond: "18446744073709551616", // uint64 max + 1
			},
			expectError: true,
		},
		{
			name: "floating point number",
			params: map[string]string{
				nvmeof.RwMbytesPerSecond: "100.5",
			},
			expectError: true,
		},
		{
			name: "mixed valid and invalid",
			params: map[string]string{
				nvmeof.RwIosPerSecond:    "10000",
				nvmeof.RwMbytesPerSecond: "invalid",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := parseQoSParameters(tt.params)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
