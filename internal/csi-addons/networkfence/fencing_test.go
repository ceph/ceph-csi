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

	"github.com/stretchr/testify/assert"
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
		ts := tt
		t.Run(ts.cidr, func(t *testing.T) {
			t.Parallel()
			got, err := getIPRange(ts.cidr)
			assert.NoError(t, err)

			// validate if number of IPs in the range is same as expected, if not, fail.
			assert.ElementsMatch(t, ts.expectedIPs, got)
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
			clientInfo:  "",
			expectedIP:  "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		ts := tt

		t.Run(ts.clientInfo, func(t *testing.T) {
			t.Parallel()

			client := activeClient{Inst: ts.clientInfo}
			ip, actualErr := client.fetchIP()

			if (actualErr != nil) != ts.expectedErr {
				t.Errorf("expected error %v but got %v", ts.expectedErr, actualErr)
			}

			if ip != ts.expectedIP {
				t.Errorf("expected IP %s but got %s", ts.expectedIP, ip)
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
		ts := tt
		t.Run(ts.clientInfo, func(t *testing.T) {
			t.Parallel()
			ac := &activeClient{Inst: ts.clientInfo}
			actualID, actualErr := ac.fetchID()

			if (actualErr != nil) != ts.expectedErr {
				t.Errorf("expected error %v but got %v", ts.expectedErr, actualErr)
			}

			if actualID != ts.expectedID {
				t.Errorf("expected ID %d but got %d", ts.expectedID, actualID)
			}
		})
	}
}
