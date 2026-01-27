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

package nvmeof

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayAddress_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		address string
		port    uint32
		result  string
	}{
		{"127.0.0.1", 5500, "127.0.0.1:5500"},
		{"localhost", 0o055 /* octal: 45 */, "localhost:45"},
		{"localhost.localdomain", 0, "localhost.localdomain:0"},
	}

	for _, test := range tests {
		gw := GatewayAddress{
			Address: test.address,
			Port:    test.port,
		}

		require.Equal(t, test.result, gw.String())
	}
}

func TestGatewayRpcClient_generateSerialNumber(t *testing.T) {
	t.Parallel()

	client := &GatewayRpcClient{
		// ransom serial should always result in 0
		maxSerial: big.NewInt(1),
	}

	serial, err := client.generateSerialNumber()
	require.NoError(t, err)
	require.Equal(t, "Ceph2", serial)
}

func TestListSubsystems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jsonInput string
		wantErr   bool
		wantHosts int
		validate  func(t *testing.T, hosts nvmeHostConnections)
	}{
		{
			name: "valid single host with single subsystem and single path",
			jsonInput: `[
				{
					"HostNQN": "nqn.2014-08.org.nvmexpress:uuid:12345678",
					"Subsystems": [
						{
							"NQN": "nqn.2016-06.io.ceph:subsystem.test",
							"Paths": [
								{
									"Address": "traddr=10.242.64.32,trsvcid=4420,src_addr=10.242.64.33",
									"State": "live"
								}
							]
						}
					]
				}
			]`,
			wantErr:   false,
			wantHosts: 1,
			validate: func(t *testing.T, hosts nvmeHostConnections) {
				t.Helper()
				require.Len(t, hosts, 1)
				require.Equal(t, "nqn.2014-08.org.nvmexpress:uuid:12345678", hosts[0].HostNQN)
				require.Len(t, hosts[0].Subsystems, 1)
				require.Equal(t, "nqn.2016-06.io.ceph:subsystem.test", hosts[0].Subsystems[0].NQN)
				require.Len(t, hosts[0].Subsystems[0].Paths, 1)

				// Validate parsed address fields
				path := hosts[0].Subsystems[0].Paths[0]
				require.Equal(t, "10.242.64.32", path.Address.Traddr)
				require.Equal(t, "4420", path.Address.Trsvcid)
				require.Equal(t, "10.242.64.33", path.Address.SrcAddr)
				require.Equal(t, "live", path.State)
			},
		},
		{
			name: "valid host with multipath (multiple paths)",
			jsonInput: `[
				{
					"HostNQN": "nqn.2014-08.org.nvmexpress:uuid:abcdef",
					"Subsystems": [
						{
							"NQN": "nqn.2016-06.io.ceph:subsystem.multipath",
							"Paths": [
								{
									"Address": "traddr=10.128.2.70,trsvcid=4420,src_addr=10.242.64.33",
									"State": "live"
								},
								{
									"Address": "traddr=10.129.2.45,trsvcid=4420,src_addr=10.242.64.33",
									"State": "live"
								}
							]
						}
					]
				}
			]`,
			wantErr:   false,
			wantHosts: 1,
			validate: func(t *testing.T, hosts nvmeHostConnections) {
				t.Helper()
				require.Len(t, hosts[0].Subsystems[0].Paths, 2)

				// Check first path
				path1 := hosts[0].Subsystems[0].Paths[0]
				require.Equal(t, "10.128.2.70", path1.Address.Traddr)
				require.Equal(t, "4420", path1.Address.Trsvcid)
				require.Equal(t, "live", path1.State)

				// Check second path
				path2 := hosts[0].Subsystems[0].Paths[1]
				require.Equal(t, "10.129.2.45", path2.Address.Traddr)
				require.Equal(t, "4420", path2.Address.Trsvcid)
				require.Equal(t, "live", path2.State)
			},
		},
		{
			name: "path with non-live state",
			jsonInput: `[
				{
					"HostNQN": "nqn.2014-08.org.nvmexpress:uuid:test",
					"Subsystems": [
						{
							"NQN": "nqn.2016-06.io.ceph:subsystem.test",
							"Paths": [
								{
									"Address": "traddr=192.168.1.10,trsvcid=5500,",
									"State": "connecting"
								}
							]
						}
					]
				}
			]`,
			wantErr:   false,
			wantHosts: 1,
			validate: func(t *testing.T, hosts nvmeHostConnections) {
				t.Helper()
				path := hosts[0].Subsystems[0].Paths[0]
				require.Equal(t, "connecting", path.State)
			},
		},
		{
			name: "multiple hosts",
			jsonInput: `[
				{
					"HostNQN": "nqn.2014-08.org.nvmexpress:uuid:host1",
					"Subsystems": [
						{
							"NQN": "nqn.2016-06.io.ceph:subsystem.one",
							"Paths": [
								{
									"Address": "traddr=10.0.0.1,trsvcid=4420,src_addr=10.0.0.2",
									"State": "live"
								}
							]
						}
					]
				},
				{
					"HostNQN": "nqn.2014-08.org.nvmexpress:uuid:host2",
					"Subsystems": [
						{
							"NQN": "nqn.2016-06.io.ceph:subsystem.two",
							"Paths": [
								{
									"Address": "traddr=10.0.1.1,trsvcid=4420,src_addr=10.0.1.2",
									"State": "live"
								}
							]
						}
					]
				}
			]`,
			wantErr:   false,
			wantHosts: 2,
			validate: func(t *testing.T, hosts nvmeHostConnections) {
				t.Helper()
				require.Len(t, hosts, 2)
				require.Equal(t, "nqn.2014-08.org.nvmexpress:uuid:host1", hosts[0].HostNQN)
				require.Equal(t, "nqn.2014-08.org.nvmexpress:uuid:host2", hosts[1].HostNQN)
			},
		},
		{
			name:      "invalid json",
			jsonInput: `{invalid json}`,
			wantErr:   true,
		},
		{
			name:      "empty array",
			jsonInput: `[]`,
			wantErr:   false,
			wantHosts: 0,
			validate: func(t *testing.T, hosts nvmeHostConnections) {
				t.Helper()
				require.Empty(t, hosts)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var hosts nvmeHostConnections
			err := json.Unmarshal([]byte(tt.jsonInput), &hosts)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Len(t, hosts, tt.wantHosts)

			if tt.validate != nil {
				tt.validate(t, hosts)
			}
		})
	}
}

func TestHasLivePathToGateway(t *testing.T) {
	t.Parallel()

	// Setup test data
	connections := nvmeHostConnections{
		{
			HostNQN: "nqn.2014-08.org.nvmexpress:uuid:test-host",
			Subsystems: []struct {
				NQN   string `json:"NQN"`
				Paths []struct {
					Address nvmePathAddress `json:"Address"`
					State   string          `json:"State"`
				} `json:"Paths"`
			}{
				{
					NQN: "nqn.2016-06.io.ceph:subsystem.test",
					Paths: []struct {
						Address nvmePathAddress `json:"Address"`
						State   string          `json:"State"`
					}{
						{
							Address: nvmePathAddress{
								Traddr:  "10.128.2.70",
								Trsvcid: "4420",
								SrcAddr: "10.242.64.33",
							},
							State: "live",
						},
						{
							Address: nvmePathAddress{
								Traddr:  "10.129.2.45",
								Trsvcid: "4420",
								SrcAddr: "10.242.64.33",
							},
							State: "connecting",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name         string
		subsystemNQN string
		hostNQN      string
		gatewayIP    string
		gatewayPort  string
		want         bool
	}{
		{
			name:         "matching live path",
			subsystemNQN: "nqn.2016-06.io.ceph:subsystem.test",
			hostNQN:      "nqn.2014-08.org.nvmexpress:uuid:test-host",
			gatewayIP:    "10.128.2.70",
			gatewayPort:  "4420",
			want:         true,
		},
		{
			name:         "path exists but not live",
			subsystemNQN: "nqn.2016-06.io.ceph:subsystem.test",
			hostNQN:      "nqn.2014-08.org.nvmexpress:uuid:test-host",
			gatewayIP:    "10.129.2.45",
			gatewayPort:  "4420",
			want:         true,
		},
		{
			name:         "wrong gateway IP",
			subsystemNQN: "nqn.2016-06.io.ceph:subsystem.test",
			hostNQN:      "nqn.2014-08.org.nvmexpress:uuid:test-host",
			gatewayIP:    "10.130.0.1",
			gatewayPort:  "4420",
			want:         false,
		},
		{
			name:         "wrong port",
			subsystemNQN: "nqn.2016-06.io.ceph:subsystem.test",
			hostNQN:      "nqn.2014-08.org.nvmexpress:uuid:test-host",
			gatewayIP:    "10.128.2.70",
			gatewayPort:  "5500",
			want:         false,
		},
		{
			name:         "wrong subsystem NQN",
			subsystemNQN: "nqn.2016-06.io.ceph:subsystem.wrong",
			hostNQN:      "nqn.2014-08.org.nvmexpress:uuid:test-host",
			gatewayIP:    "10.128.2.70",
			gatewayPort:  "4420",
			want:         false,
		},
		{
			name:         "wrong host NQN",
			subsystemNQN: "nqn.2016-06.io.ceph:subsystem.test",
			hostNQN:      "nqn.2014-08.org.nvmexpress:uuid:wrong-host",
			gatewayIP:    "10.128.2.70",
			gatewayPort:  "4420",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := connections.hasPathToGateway(
				tt.subsystemNQN,
				tt.hostNQN,
				tt.gatewayIP,
				tt.gatewayPort,
			)

			require.Equal(t, tt.want, got)
		})
	}
}
