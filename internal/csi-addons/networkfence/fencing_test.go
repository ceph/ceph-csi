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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
			clientInfo:  "client.4305 2001:0db8:85a3:0000:0000:8a2e:0370:7334:0/422650892",
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

func TestParseBlocklistEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected IPWithNonce
	}{
		{
			name:     "Valid IP and nonce",
			input:    "192.168.1.1:6789/abcdef123456",
			expected: IPWithNonce{IP: "192.168.1.1", Nonce: "abcdef123456"},
		},
		{
			name:     "IPv6 address with full notation",
			input:    "2001:0db8:0000:0000:0000:8a2e:0370:7334:6789/abc123",
			expected: IPWithNonce{IP: "2001:0db8:0000:0000:0000:8a2e:0370:7334", Nonce: "abc123"},
		},
		{
			name:     "IPv6 address with compressed zeros",
			input:    "2001:db8::1428:57ab:6789/def456",
			expected: IPWithNonce{IP: "2001:db8::1428:57ab", Nonce: "def456"},
		},
		{
			name:     "IPv6 loopback address",
			input:    "::1:6789/ghi789",
			expected: IPWithNonce{IP: "::1", Nonce: "ghi789"},
		},
		{
			name:     "IPv6 address with IPv4 mapping",
			input:    "::ffff:192.0.2.128:6789/jkl012",
			expected: IPWithNonce{IP: "::ffff:192.0.2.128", Nonce: "jkl012"},
		},
		{
			name:     "IP without port",
			input:    "10.0.0.1/nonce123",
			expected: IPWithNonce{},
		},
		{
			name:     "Extra whitespace",
			input:    "  172.16.0.1:1234/abc123  extra info  ",
			expected: IPWithNonce{IP: "172.16.0.1", Nonce: "abc123"},
		},
	}

	nf := &NetworkFence{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := nf.parseBlocklistEntry(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseBlocklistForCIDR(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		blocklist string
		cidr      string
		expected  []IPWithNonce
	}{
		{
			name: "Single IPv4 in CIDR",
			blocklist: `192.168.1.1:0/1234567 expires 2023-07-01 10:00:00.000000
listed 1 entries`,
			cidr:     "192.168.1.0/24",
			expected: []IPWithNonce{{IP: "192.168.1.1", Nonce: "1234567"}},
		},
		{
			name: "Multiple IPv4 in CIDR",
			blocklist: `192.168.1.1:0/1234567 expires 2023-07-01 10:00:00.000000
192.168.1.2:0/7654321 expires 2023-07-01 11:00:00.000000
192.168.2.1:0/abcdefg expires 2023-07-01 12:00:00.000000
listed 3 entries`,
			cidr: "192.168.1.0/24",
			expected: []IPWithNonce{
				{IP: "192.168.1.1", Nonce: "1234567"},
				{IP: "192.168.1.2", Nonce: "7654321"},
			},
		},
		{
			name: "IPv6 in CIDR",
			blocklist: `2001:db8::1:0/fedcba expires 2023-07-01 10:00:00.000000
2001:db8::2:0/abcdef expires 2023-07-01 11:00:00.000000
listed 2 entries`,
			cidr:     "2001:db8::/64",
			expected: []IPWithNonce{{IP: "2001:db8::1", Nonce: "fedcba"}, {IP: "2001:db8::2", Nonce: "abcdef"}},
		},
		{
			name:      "Empty blocklist",
			blocklist: `listed 0 entries`,
			cidr:      "192.168.1.0/24",
			expected:  []IPWithNonce{},
		},
		{
			name: "No matching IPs",
			blocklist: `10.0.0.1:0/1234567 expires 2023-07-01 10:00:00.000000
listed 1 entries`,
			cidr:     "192.168.1.0/24",
			expected: []IPWithNonce{},
		},
	}

	nf := &NetworkFence{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := nf.parseBlocklistForCIDR(context.TODO(), tc.blocklist, tc.cidr)
			require.Equal(t, tc.expected, result)
		})
	}
}
