/*
Copyright 2022 The Ceph-CSI Authors.
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
