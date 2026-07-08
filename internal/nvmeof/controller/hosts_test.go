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

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/nvmeof"
)

func TestParseHostParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		params      map[string]string
		expected    *nvmeof.NVMeoFHostList
		expectError bool
	}{
		{
			name:     "no allowHostNQNs parameter",
			params:   map[string]string{},
			expected: nil,
		},
		{
			name: "empty allowHostNQNs (remove all hosts)",
			params: map[string]string{
				nvmeof.AllowHostNQNs: "",
			},
			expected: &nvmeof.NVMeoFHostList{HostNQNs: []string{}}, // Empty slice, not nil - signals to remove all hosts
		},
		{
			name: "single host NQN",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `- nqn.2014-08.org.nvmexpress:host1`,
			},
			expected: &nvmeof.NVMeoFHostList{
				HostNQNs: []string{"nqn.2014-08.org.nvmexpress:host1"},
			},
		},
		{
			name: "multiple host NQNs",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `- nqn.2014-08.org.nvmexpress:host1
- nqn.2014-08.org.nvmexpress:host2
- nqn.2014-08.org.nvmexpress:host3`,
			},
			expected: &nvmeof.NVMeoFHostList{
				HostNQNs: []string{
					"nqn.2014-08.org.nvmexpress:host1",
					"nqn.2014-08.org.nvmexpress:host2",
					"nqn.2014-08.org.nvmexpress:host3",
				},
			},
		},
		{
			name: "wildcard to allow any host",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `- "*"`,
			},
			expected: &nvmeof.NVMeoFHostList{
				HostNQNs: []string{"*"},
			},
		},
		{
			name: "YAML list in flow style",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `["nqn.2014-08.org.nvmexpress:host1", "nqn.2014-08.org.nvmexpress:host2"]`,
			},
			expected: &nvmeof.NVMeoFHostList{
				HostNQNs: []string{
					"nqn.2014-08.org.nvmexpress:host1",
					"nqn.2014-08.org.nvmexpress:host2",
				},
			},
		},
		{
			name: "invalid YAML - not a list (plain string)",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `nqn.2014-08.org.nvmexpress:host1`,
			},
			expectError: true,
		},
		{
			name: "invalid YAML - map instead of list",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `host1: nqn.2014-08.org.nvmexpress:host1`,
			},
			expectError: true,
		},
		{
			name: "invalid YAML - unclosed quote",
			params: map[string]string{
				nvmeof.AllowHostNQNs: `- "nqn.2014-08.org.nvmexpress:host1`,
			},
			expectError: true,
		},
		{
			name: "other parameters present but no allowHostNQNs",
			params: map[string]string{
				"someOtherParam":       "value",
				"nvmeofRWIOsPerSecond": "10000",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := nvmeof.NewNVMeoFHostListFromParams(tt.params)

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
