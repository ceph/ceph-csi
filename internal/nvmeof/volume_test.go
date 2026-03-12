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
