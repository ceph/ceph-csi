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
