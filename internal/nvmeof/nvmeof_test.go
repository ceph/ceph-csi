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
	"context"
	"encoding/json"
	"math/big"
	"testing"

	pb "github.com/ceph/ceph-nvmeof/lib/go/nvmeof"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestListenerDetailsSetDefaults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   ListenerDetails
		address string
		port    uint32
		family  string
		wantErr bool
	}{
		{
			name: "hostname resolves to ipv4",
			input: ListenerDetails{
				Hostname: "192.0.2.1",
			},
			address: "0.0.0.0",
			port:    4420,
			family:  addressFamilyIPv4,
		},
		{
			name: "hostname resolves to ipv6",
			input: ListenerDetails{
				Hostname: "2001:db8::1",
			},
			address: "::",
			port:    4420,
			family:  addressFamilyIPv6,
		},
		{
			name: "ipv4 wildcard",
			input: ListenerDetails{
				Hostname:      "nvmeof-gw-a",
				AddressFamily: addressFamilyIPv4,
			},
			address: "0.0.0.0",
			port:    4420,
			family:  addressFamilyIPv4,
		},
		{
			name: "ipv6 wildcard",
			input: ListenerDetails{
				Hostname:      "nvmeof-gw-a",
				AddressFamily: addressFamilyIPv6,
			},
			address: "::",
			port:    4420,
			family:  addressFamilyIPv6,
		},
		{
			name: "unresolvable hostname falls back to ipv4",
			input: ListenerDetails{
				Hostname: "nvmeof-gw.invalid",
			},
			address: "0.0.0.0",
			port:    4420,
			family:  addressFamilyIPv4,
		},
		{
			name: "infer ipv6",
			input: ListenerDetails{
				GatewayAddress: GatewayAddress{Address: "2001:db8::1", Port: 5500},
			},
			address: "2001:db8::1",
			port:    5500,
			family:  addressFamilyIPv6,
		},
		{
			name: "infer ipv4",
			input: ListenerDetails{
				GatewayAddress: GatewayAddress{Address: "192.0.2.1"},
			},
			address: "192.0.2.1",
			port:    4420,
			family:  addressFamilyIPv4,
		},
		{
			name: "ipv6 address ipv4 family",
			input: ListenerDetails{
				GatewayAddress: GatewayAddress{Address: "2001:db8::1"},
				AddressFamily:  addressFamilyIPv4,
			},
			wantErr: true,
		},
		{
			name: "ipv4 address ipv6 family",
			input: ListenerDetails{
				GatewayAddress: GatewayAddress{Address: "192.0.2.1"},
				AddressFamily:  addressFamilyIPv6,
			},
			wantErr: true,
		},
		{
			name:    "invalid family",
			input:   ListenerDetails{AddressFamily: "invalid"},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			listener := test.input
			err := listener.setDefaults()
			if test.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, test.address, listener.Address)
			require.Equal(t, test.port, listener.Port)
			require.Equal(t, test.family, listener.AddressFamily)
			require.NoError(t, listener.setDefaults())
		})
	}
}

func TestAddressFamilyFromAddresses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		addresses []string
		family    pb.AddressFamily
		wantErr   bool
	}{
		{
			name: "ipv4",
			addresses: []string{
				"192.0.2.1",
			},
			family: pb.AddressFamily_ipv4,
		},
		{
			name: "ipv6",
			addresses: []string{
				"2001:db8::1",
			},
			family: pb.AddressFamily_ipv6,
		},
		{
			name: "skip invalid address",
			addresses: []string{
				"invalid",
				"2001:db8::1",
			},
			family: pb.AddressFamily_ipv6,
		},
		{
			name: "no valid address",
			addresses: []string{
				"invalid",
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			family, err := addressFamilyFromAddresses("gateway.example", test.addresses)
			if test.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, test.family, family)
		})
	}
}

type listenerGatewayClient struct {
	pb.GatewayClient
	created *pb.CreateListenerReq
	deleted *pb.DeleteListenerReq
}

func (client *listenerGatewayClient) CreateListener(
	_ context.Context, req *pb.CreateListenerReq, _ ...grpc.CallOption,
) (*pb.ReqStatus, error) {
	client.created = req

	return &pb.ReqStatus{}, nil
}

func (client *listenerGatewayClient) DeleteListener(
	_ context.Context, req *pb.DeleteListenerReq, _ ...grpc.CallOption,
) (*pb.ReqStatus, error) {
	client.deleted = req

	return &pb.ReqStatus{}, nil
}

func TestListenerRPCAddressFamily(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"192.0.2.1", "2001:db8::1", "::"} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			client := &listenerGatewayClient{}
			gateway := &GatewayRpcClient{client: client}
			listener := ListenerDetails{GatewayAddress: GatewayAddress{Address: address, Port: 4420}, Hostname: "gw"}
			family := pb.AddressFamily_ipv6
			if address == "192.0.2.1" {
				family = pb.AddressFamily_ipv4
			}
			require.NoError(t, gateway.CreateListener(context.Background(), "nqn.test", listener))
			require.Equal(t, family, client.created.GetAdrfam())
			require.NoError(t, gateway.DeleteListener(context.Background(), "nqn.test", listener))
			require.Equal(t, family, client.deleted.GetAdrfam())
		})
	}
}

func TestConvertListenersFromProtoAddressFamily(t *testing.T) {
	t.Parallel()
	listeners := ConvertListenersFromProto([]*pb.ListenerInfo{
		{HostName: "gw", Traddr: "::", Trsvcid: 4420, Adrfam: pb.AddressFamily_ipv6},
	})
	require.Len(t, listeners, 1)
	require.Equal(t, "ipv6", listeners[0].AddressFamily)
}

func TestResolveListeners(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		address  string
		hostname string
		family   string
		want     string
		wantErr  bool
	}{
		{
			name:     "legacy ipv4 wildcard",
			address:  "0.0.0.0",
			hostname: "192.0.2.1",
			want:     "192.0.2.1",
		},
		{
			name:     "ipv6 wildcard",
			address:  "::",
			hostname: "2001:db8::1",
			family:   addressFamilyIPv6,
			want:     "2001:db8::1",
		},
		{
			name:     "infer ipv6 wildcard",
			address:  "::",
			hostname: "2001:db8::1",
			want:     "2001:db8::1",
		},
		{
			name:     "expanded ipv6 wildcard",
			address:  "0:0:0:0:0:0:0:0",
			hostname: "2001:db8::1",
			want:     "2001:db8::1",
		},
		{
			name:     "empty ipv6 address",
			hostname: "2001:db8::1",
			family:   addressFamilyIPv6,
			want:     "2001:db8::1",
		},
		{
			name:    "explicit ipv6 address",
			address: "2001:db8::1",
			family:  addressFamilyIPv6,
			want:    "2001:db8::1",
		},
		{
			name:    "explicit ipv4 address",
			address: "192.0.2.1",
			want:    "192.0.2.1",
		},
		{
			name:     "ipv4 target for ipv6",
			address:  "::",
			hostname: "192.0.2.1",
			wantErr:  true,
		},
		{
			name:     "ipv6 target for ipv4",
			address:  "0.0.0.0",
			hostname: "2001:db8::1",
			wantErr:  true,
		},
		{
			name:    "address family mismatch",
			address: "2001:db8::1",
			family:  addressFamilyIPv4,
			wantErr: true,
		},
		{
			name:    "invalid family",
			address: "::",
			family:  "invalid",
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			listener := ListenerDetails{
				GatewayAddress: GatewayAddress{
					Address: test.address,
					Port:    4420,
				},
				Hostname:      test.hostname,
				AddressFamily: test.family,
			}
			input := []ListenerDetails{listener}
			listeners, err := ResolveListeners(context.Background(), input)
			require.Equal(t, listener, input[0])
			if test.wantErr {
				require.Error(t, err)
				require.Empty(t, listeners)

				return
			}
			require.NoError(t, err)
			require.Len(t, listeners, 1)
			require.Equal(t, test.want, listeners[0].Address)
			require.Equal(t, uint32(4420), listeners[0].Port)
			require.Equal(t, test.hostname, listeners[0].Hostname)
		})
	}
}

func TestResolveListenersPartialFailure(t *testing.T) {
	t.Parallel()
	listeners, err := ResolveListeners(context.Background(), []ListenerDetails{
		{
			GatewayAddress: GatewayAddress{
				Address: "::",
				Port:    4420,
			},
			Hostname:      "192.0.2.1",
			AddressFamily: addressFamilyIPv6,
		},
		{
			GatewayAddress: GatewayAddress{
				Address: "::",
				Port:    4420,
			},
			Hostname:      "2001:db8::1",
			AddressFamily: addressFamilyIPv6,
		},
	})
	require.NoError(t, err)
	require.Len(t, listeners, 1)
	require.Equal(t, "2001:db8::1", listeners[0].Address)
}

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
					Name    string          `json:"Name"`
					Address nvmePathAddress `json:"Address"`
					State   string          `json:"State"`
				} `json:"Paths"`
			}{
				{
					NQN: "nqn.2016-06.io.ceph:subsystem.test",
					Paths: []struct {
						Name    string          `json:"Name"`
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
