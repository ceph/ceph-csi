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
	"errors"
	"testing"

	pb "github.com/ceph/ceph-nvmeof/lib/go/nvmeof"
	"github.com/stretchr/testify/require"
)

func TestResolveIPAddressByFamily(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		family    pb.AddressFamily
		addresses []string
		lookupErr error
		want      string
		wantErr   string
	}{
		{
			name:      "ipv6 after ipv4",
			family:    pb.AddressFamily_ipv6,
			addresses: []string{"192.0.2.1", "2001:db8::1"},
			want:      "2001:db8::1",
		},
		{
			name:      "ipv4 after ipv6",
			family:    pb.AddressFamily_ipv4,
			addresses: []string{"2001:db8::1", "192.0.2.1"},
			want:      "192.0.2.1",
		},
		{
			name:      "ipv4 only for ipv6",
			family:    pb.AddressFamily_ipv6,
			addresses: []string{"192.0.2.1"},
			wantErr:   "no ipv6 addresses",
		},
		{
			name:      "ipv6 only for ipv4",
			family:    pb.AddressFamily_ipv4,
			addresses: []string{"2001:db8::1"},
			wantErr:   "no ipv4 addresses",
		},
		{
			name:      "mapped ipv4 is not ipv6",
			family:    pb.AddressFamily_ipv6,
			addresses: []string{"::ffff:192.0.2.1", "2001:db8::1"},
			want:      "2001:db8::1",
		},
		{
			name:      "skip invalid and unspecified",
			family:    pb.AddressFamily_ipv6,
			addresses: []string{"invalid", "::", "0.0.0.0", "2001:db8::1"},
			want:      "2001:db8::1",
		},
		{
			name:    "no addresses",
			family:  pb.AddressFamily_ipv6,
			wantErr: "no IP addresses",
		},
		{
			name:      "lookup failure",
			family:    pb.AddressFamily_ipv6,
			lookupErr: errors.New("lookup failed"),
			wantErr:   "lookup failed",
		},
		{
			name:    "unsupported family",
			family:  pb.AddressFamily(99),
			wantErr: "unsupported address family",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			address, err := resolveIPAddress("gateway.example", test.family, func(host string) ([]string, error) {
				require.Equal(t, "gateway.example", host)

				return test.addresses, test.lookupErr
			})
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Empty(t, address)

				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, address)
		})
	}
}

func TestFormatUUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in  string
		out string
	}{
		{"438cb4a8ae90477485677ea1414cd3ac", "438cb4a8-ae90-4774-8567-7ea1414cd3ac"},
		{"4-3-8-c--b4a8ae90477485677ea1414cd3ac", "438cb4a8-ae90-4774-8567-7ea1414cd3ac"},
		{"---438cb4a8ae90477485677ea1414cd3ac---", "438cb4a8-ae90-4774-8567-7ea1414cd3ac"},
		{"invalid", "invalid"},
		{"", ""},
	}

	for _, test := range tests {
		out := formatUUID(test.in)
		require.Equal(t, test.out, out)
	}
}
