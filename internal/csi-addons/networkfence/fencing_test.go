/*
Copyright 2023 The Ceph-CSI Authors.

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

package networkfence

import (
	"testing"
	"time"

	osdAdmin "github.com/ceph/go-ceph/common/admin/osd"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/util"
)

func TestGetIPRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cidr        string
		expectedIPs []string
	}{
		{
			cidr:        "192.168.1.0/31",
			expectedIPs: []string{"192.168.1.0", "192.168.1.1"},
		},
		{
			cidr:        "10.0.0.0/30",
			expectedIPs: []string{"10.0.0.0", "10.0.0.1", "10.0.0.2", "10.0.0.3"},
		},
		{
			cidr:        "fd4a:ecbc:cafd:4e49::/127",
			expectedIPs: []string{"fd4a:ecbc:cafd:4e49::", "fd4a:ecbc:cafd:4e49::1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			t.Parallel()
			got, err := getIPRange(tt.cidr)
			require.NoError(t, err)

			// validate if number of IPs in the range is same as expected, if not, fail.
			require.ElementsMatch(t, tt.expectedIPs, got)
		})
	}
}

func TestFetchIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		clientInfo  string
		expectedIP  string
		expectedErr bool
	}{
		{
			clientInfo:  "client.4305 172.21.9.34:0/422650892",
			expectedIP:  "172.21.9.34",
			expectedErr: false,
		},
		{
			clientInfo:  "client.4305 [2001:0db8:85a3:0000:0000:8a2e:0370:7334]:0/422650892",
			expectedIP:  "2001:db8:85a3::8a2e:370:7334",
			expectedErr: false,
		},
		{
			clientInfo:  "client.24152 v1:100.64.0.7:0/3658550259",
			expectedIP:  "100.64.0.7",
			expectedErr: false,
		},
		{
			clientInfo:  "",
			expectedIP:  "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.clientInfo, func(t *testing.T) {
			t.Parallel()

			client := activeClient{Inst: tt.clientInfo}
			ip, actualErr := client.fetchIP()

			if (actualErr != nil) != tt.expectedErr {
				t.Errorf("expected error %v but got %v", tt.expectedErr, actualErr)
			}

			if ip != tt.expectedIP {
				t.Errorf("expected IP %s but got %s", tt.expectedIP, ip)
			}
		})
	}
}

func TestFetchID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		clientInfo  string
		expectedID  int
		expectedErr bool
	}{
		{
			clientInfo:  "client.4305 172.21.9.34:0/422650892",
			expectedID:  4305,
			expectedErr: false,
		},
		{
			clientInfo:  "",
			expectedID:  0,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.clientInfo, func(t *testing.T) {
			t.Parallel()
			ac := &activeClient{Inst: tt.clientInfo}
			actualID, actualErr := ac.fetchID()

			if (actualErr != nil) != tt.expectedErr {
				t.Errorf("expected error %v but got %v", tt.expectedErr, actualErr)
			}

			if actualID != tt.expectedID {
				t.Errorf("expected ID %d but got %d", tt.expectedID, actualID)
			}
		})
	}
}

func Test_containsMatchingBlockListEntry(t *testing.T) {
	t.Parallel()
	type args struct {
		blocklist *[]osdAdmin.Blocklist
		addr      string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "matching entry found blocked outside cool down period 1",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "192.0.1.0:0/32",
						Until: time.Now().Add(1 * time.Hour),
					},
					{
						Addr:  "192.0.2.0:0/32",
						Until: time.Now().Add(1 * time.Hour),
					},
				},
				addr: "192.0.1.0",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "matching entry found blocked outside cool down period 2",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "192.0.1.0:0/32",
						Until: time.Now().Add(util.AutoBlocklistTime - blockListCoolDownPeriod),
					},
					{
						Addr:  "192.0.2.0:0/32",
						Until: time.Now().Add(util.AutoBlocklistTime - blockListCoolDownPeriod),
					},
				},
				addr: "192.0.1.0",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "matching entry found blocked in cool down period 1",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "192.0.1.0:0/32",
						Until: time.Now().Add(util.AutoBlocklistTime),
					},
					{
						Addr:  "192.0.2.0:0/32",
						Until: time.Now().Add(util.AutoBlocklistTime),
					},
				},
				addr: "192.0.1.0",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "matching entry found blocked in cool down period 2",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "192.0.1.0:0/32",
						Until: time.Now().Add(util.AutoBlocklistTime - 2*time.Minute),
					},
					{
						Addr:  "192.0.2.0:0/32",
						Until: time.Now().Add(util.AutoBlocklistTime - 2*time.Minute),
					},
				},
				addr: "192.0.1.0",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "matching IPv6 entry found blocked outside cool down period 1",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::1:0/128",
						Until: time.Now().Add(1 * time.Hour),
					},
					{
						Addr:  "2001:db8::2:0/128",
						Until: time.Now().Add(1 * time.Hour),
					},
				},
				addr: "2001:db8::1",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "matching IPv6 entry found blocked outside cool down period 2",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::1:0/128",
						Until: time.Now().Add(util.AutoBlocklistTime - blockListCoolDownPeriod),
					},
					{
						Addr:  "2001:db8::2:0/128",
						Until: time.Now().Add(util.AutoBlocklistTime - blockListCoolDownPeriod),
					},
				},
				addr: "2001:db8::1",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "matching IPv6 entry found blocked in cool down period 1",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::1:0/128",
						Until: time.Now().Add(util.AutoBlocklistTime),
					},
					{
						Addr:  "2001:db8::2:0/128",
						Until: time.Now().Add(util.AutoBlocklistTime),
					},
				},
				addr: "2001:db8::1",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "matching IPv6 entry found blocked in cool down period 2",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::1:0/128",
						Until: time.Now().Add(util.AutoBlocklistTime - 2*time.Minute),
					},
					{
						Addr:  "2001:db8::2:0/128",
						Until: time.Now().Add(util.AutoBlocklistTime - 2*time.Minute),
					},
				},
				addr: "2001:db8::1",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "address does not match",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "193.0.1.0:0/32",
						Until: time.Now(),
					},
					{
						Addr:  "192.0.2.0:0/32",
						Until: time.Now(),
					},
				},
				addr: "192.0.1.0",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "addr matches but fenced for max duration",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "192.0.1.0:0/32",
						Until: time.Now().Add(util.MaxBlocklistTime),
					},
				},
				addr: "192.0.1.0",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "matching IPv6 entry found",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::1:0/128",
						Until: time.Now(),
					},
					{
						Addr:  "2001:db8::2:0/128",
						Until: time.Now(),
					},
				},
				addr: "2001:db8::1",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "IPv6 address does not match",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::3:0/128",
						Until: time.Now(),
					},
					{
						Addr:  "2001:db8::2:0/128",
						Until: time.Now(),
					},
				},
				addr: "2001:db8::1",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "IPv6 addr matches but fenced for max duration",
			args: args{
				blocklist: &[]osdAdmin.Blocklist{
					{
						Addr:  "2001:db8::1:0/128",
						Until: time.Now().Add(util.MaxBlocklistTime),
					},
				},
				addr: "2001:db8::1",
			},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := containsMatchingBlockListEntry(tt.args.blocklist, tt.args.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("containsMatchingBlockListEntry() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if got != tt.want {
				t.Errorf("containsMatchingBlockListEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_matchEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		actual   string
		expected string
		want     bool
	}{
		{
			name:     "IPv4 match with /32 suffix",
			actual:   "192.168.1.1:0/32",
			expected: "192.168.1.1",
			want:     true,
		},
		{
			name:     "IPv4 match without /32 suffix",
			actual:   "192.168.1.1:0/878978",
			expected: "192.168.1.1",
			want:     false,
		},
		{
			name:     "IPv4 no match different IPs",
			actual:   "192.168.1.2:0/32",
			expected: "192.168.1.1",
			want:     false,
		},
		{
			name:     "IPv6 match with /128 suffix",
			actual:   "2001:db8::1:0/128",
			expected: "2001:db8::1",
			want:     true,
		},
		{
			name:     "IPv6 match without /128 suffix",
			actual:   "2001:db8::1:0/123213",
			expected: "2001:db8::1",
			want:     false,
		},
		{
			name:     "IPv6 no match different IPs",
			actual:   "2001:db8::2:0/128",
			expected: "2001:db8::1",
			want:     false,
		},
		{
			name:     "IPv6 compressed notation match",
			actual:   "fd4a:ecbc:cafd:4e49::1:0/128",
			expected: "fd4a:ecbc:cafd:4e49::1",
			want:     true,
		},
		{
			name:     "Invalid expected IP",
			actual:   "192.168.1.1:0/32",
			expected: "invalid-ip",
			want:     false,
		},
		{
			name:     "Invalid actual IP",
			actual:   "invalid:0/32",
			expected: "192.168.1.1",
			want:     false,
		},
		{
			name:     "Empty strings",
			actual:   "",
			expected: "",
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchEntry(tt.actual, tt.expected)
			if got != tt.want {
				t.Errorf("matchEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}
