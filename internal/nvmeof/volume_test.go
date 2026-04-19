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

package nvmeof

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetListenersWithDefaults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in  []ListenerDetails
		out []ListenerDetails
	}{
		{
			in: []ListenerDetails{
				{Hostname: "nvmeof-gw-a", GatewayAddress: GatewayAddress{Port: 0, Address: ""}},
				{Hostname: "nvmeof-gw-b", GatewayAddress: GatewayAddress{Port: 1234, Address: ""}},
				{Hostname: "nvmeof-gw-c", GatewayAddress: GatewayAddress{Port: 0, Address: "10.92.3.12"}},
				{Hostname: "nvmeof-gw-d", GatewayAddress: GatewayAddress{Port: 1234, Address: "10.92.3.13"}},
			},
			out: []ListenerDetails{
				{Hostname: "nvmeof-gw-a", GatewayAddress: GatewayAddress{Port: 4420, Address: "0.0.0.0"}},
				{Hostname: "nvmeof-gw-b", GatewayAddress: GatewayAddress{Port: 1234, Address: "0.0.0.0"}},
				{Hostname: "nvmeof-gw-c", GatewayAddress: GatewayAddress{Port: 4420, Address: "10.92.3.12"}},
				{Hostname: "nvmeof-gw-d", GatewayAddress: GatewayAddress{Port: 1234, Address: "10.92.3.13"}},
			},
		},
	}

	for _, test := range tests {
		vol := &NVMeoFVolumeData{ListenerInfo: test.in}
		vol.SetListenersWithDefaults()
		require.Equal(t, test.out, vol.ListenerInfo)
	}
}

func TestSetupListeners(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		jsonInput   string
		expected    []ListenerDetails
		expectError bool
	}{
		{
			name:      "valid listeners JSON",
			jsonInput: `[{"hostname": "nvmeof-gw-a", "port": 4420, "address": "10.92.3.12"}]`,
			expected: []ListenerDetails{
				{Hostname: "nvmeof-gw-a", GatewayAddress: GatewayAddress{Port: 4420, Address: "10.92.3.12"}},
			},
			expectError: false,
		},
		{
			name:        "invalid listeners JSON",
			jsonInput:   `invalid json`,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "missing required fields",
			jsonInput:   `[{"port": 4420, "address": "10.92.3.12"}]`,
			expected:    nil,
			expectError: true,
		},
		// An empty listeners array is invalid because at least one listener is required.
		// the listener label can be omitted, then network mask label must be provided.
		{
			name:        "empty listeners array",
			jsonInput:   `[]`,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "no listeners entry",
			jsonInput:   ``,
			expected:    []ListenerDetails{},
			expectError: false,
		},
	}

	for _, test := range tests {
		listeners, err := SetupListeners(test.jsonInput)
		if test.expectError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, test.expected, listeners)
		}
	}
}

func TestSetFromParameters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		params      map[string]string
		expected    NVMeoFVolumeData
		expectError bool
	}{
		{
			name: "valid parameters with listeners and gateway info",
			params: map[string]string{
				"subsystemNQN":         "nqn.2016-06.io.ceph:subsystem.test-integration",
				"nvmeofGatewayAddress": "10.241.1.5",
				"nvmeofGatewayPort":    "5500",
				"listeners":            `[{"hostname": "nvmeof-gw-a", "port": 4420, "address": "10.92.3.12"}]`,
			},
			expected: NVMeoFVolumeData{
				SubsystemNQN: "nqn.2016-06.io.ceph:subsystem.test-integration",
				GatewayManagementInfo: GatewayConfig{
					Address: "10.241.1.5",
					Port:    5500,
				},
				ListenerInfo: []ListenerDetails{
					{Hostname: "nvmeof-gw-a", GatewayAddress: GatewayAddress{Port: 4420, Address: "10.92.3.12"}},
				},
			},
			expectError: false,
		},
		{
			name: "valid parameters with DH-CHAP authentication",
			params: map[string]string{
				"subsystemNQN":         "nqn.2016-06.io.ceph:subsystem.test-dhchap",
				"nvmeofGatewayAddress": "10.241.1.6",
				"nvmeofGatewayPort":    "5500",
				"dhchapMode":           "bidirectional",
				"authenticationKMSID":  "vault-kms",
				"listeners":            `[{"hostname": "nvmeof-gw-b", "port": 4420, "address": "10.92.3.13"}]`,
			},
			expected: NVMeoFVolumeData{
				SubsystemNQN: "nqn.2016-06.io.ceph:subsystem.test-dhchap",
				GatewayManagementInfo: GatewayConfig{
					Address: "10.241.1.6",
					Port:    5500,
				},
				Security: NVMeoFSecurityConfig{
					DhchapMode:          "bidirectional",
					AuthenticationKMSID: "vault-kms",
				},
				ListenerInfo: []ListenerDetails{
					{Hostname: "nvmeof-gw-b", GatewayAddress: GatewayAddress{Port: 4420, Address: "10.92.3.13"}},
				},
			},
			expectError: false,
		},
		{
			name: "valid parameters with default KMS type",
			params: map[string]string{
				"subsystemNQN":         "nqn.2016-06.io.ceph:subsystem.test-dhchap-default-kms",
				"nvmeofGatewayAddress": "10.241.1.6",
				"nvmeofGatewayPort":    "5500",
				"dhchapMode":           "bidirectional",
				"listeners":            `[{"hostname": "nvmeof-gw-b", "port": 4420, "address": "10.92.3.13"}]`,
			},
			expected: NVMeoFVolumeData{
				SubsystemNQN: "nqn.2016-06.io.ceph:subsystem.test-dhchap-default-kms",
				GatewayManagementInfo: GatewayConfig{
					Address: "10.241.1.6",
					Port:    5500,
				},
				Security: NVMeoFSecurityConfig{
					DhchapMode:          "bidirectional",
					AuthenticationKMSID: "metadata",
				},
				ListenerInfo: []ListenerDetails{
					{Hostname: "nvmeof-gw-b", GatewayAddress: GatewayAddress{Port: 4420, Address: "10.92.3.13"}},
				},
			},
			expectError: false,
		},
		{
			name: "invalid gateway port",
			params: map[string]string{
				"subsystemNQN":         "nqn.2016-06.io.ceph:subsystem.test-invalid-port",
				"nvmeofGatewayAddress": "10.241.1.7",
				"nvmeofGatewayPort":    "invalid-port",
			},
			expected:    NVMeoFVolumeData{},
			expectError: true,
		},
		{
			name: "invalid listeners JSON",
			params: map[string]string{
				"subsystemNQN":         "nqn.2016-06.io.ceph:subsystem.test-invalid-listeners",
				"nvmeofGatewayAddress": "10.241.1.8",
				"nvmeofGatewayPort":    "5500",
				"listeners":            `invalid-json`,
			},
			expected:    NVMeoFVolumeData{},
			expectError: true,
		},
	}
	for _, test := range tests {
		vol := &NVMeoFVolumeData{}
		err := vol.SetFromParameters(test.params)
		if test.expectError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, test.expected, *vol)
		}
	}
}
