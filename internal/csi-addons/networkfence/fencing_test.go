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
