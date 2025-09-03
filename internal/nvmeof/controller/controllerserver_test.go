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

package controller

import (
	"strconv"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/nvmeof"
)

func TestExtractHostNQNFromNodeID(t *testing.T) {
	t.Parallel()

	//nolint:lll // these tests just contain long strings
	tests := []struct {
		nodeID     string
		hostNQN    string
		shouldFail bool
	}{
		{
			nodeID:  "ip-10-0-79-198.us-east-2.compute.internal::nqn.2025-08.io.ceph:ip-10-0-79-198.us-east-2.compute.internal",
			hostNQN: "nqn.2025-08.io.ceph:ip-10-0-79-198.us-east-2.compute.internal",
		},
		{
			nodeID:  "ip-10-0-79-198.us-east-2.compute.internal::nqn.2014-08.org.nvmexpress:uuid:8d018d47-1a5b-40cc-b26c-4ec9351d6723",
			hostNQN: "nqn.2014-08.org.nvmexpress:uuid:8d018d47-1a5b-40cc-b26c-4ec9351d6723",
		},
		{
			nodeID:     "ip-10-0-79-198.us-east-2.compute.internal",
			shouldFail: true,
		},
	}

	for _, test := range tests {
		hostNQN, err := extractHostNQNFromNodeID(test.nodeID)
		if test.shouldFail {
			require.Error(t, err)

			continue
		}

		require.Equal(t, test.hostNQN, hostNQN)
	}
}

func TestToRBDMetadataKey(t *testing.T) {
	t.Parallel()

	mdKey := toRBDMetadataKey("SubsystemNQN")
	require.Equal(t, ".rbd.nvmeof.SubsystemNQN", mdKey)
}

func TestPolulateVolumeContext(t *testing.T) {
	t.Parallel()

	volume := &csi.Volume{}
	config := &nvmeof.NVMeoFVolumeData{
		SubsystemNQN:  "nqn.2014-08.org.nvmexpress:uuid:e61ecd13-2727-42a3-947e-2127d63abacc",
		NamespaceID:   42,
		NamespaceUUID: "c1a0223f-a9ba-4f10-89c2-7496ad50e026",
		ListenerInfo: nvmeof.ListenerDetails{
			GatewayAddress: nvmeof.GatewayAddress{
				Address: "127.0.0.1",
				Port:    4420,
			},
			Hostname: "localhost",
		},
		GatewayManagementInfo: nvmeof.GatewayConfig{
			Address: "127.0.0.2",
			Port:    5500,
		},
	}

	populateVolumeContext(volume, config)

	require.Equal(t, config.SubsystemNQN, volume.GetVolumeContext()["SubsystemNQN"])
	require.Equal(t, strconv.FormatUint(uint64(config.NamespaceID), 10), volume.GetVolumeContext()["NamespaceID"])
	require.Equal(t, config.NamespaceUUID, volume.GetVolumeContext()["NamespaceUUID"])
	require.Equal(t, config.ListenerInfo.Address, volume.GetVolumeContext()["ListenerAddress"])
	require.Equal(t, strconv.FormatUint(uint64(config.ListenerInfo.Port), 10), volume.GetVolumeContext()["ListenerPort"])
	require.Equal(t, config.ListenerInfo.Hostname, volume.GetVolumeContext()["ListenerHostname"])
	require.Equal(t, config.GatewayManagementInfo.Address, volume.GetVolumeContext()["GatewayAddress"])
	require.Equal(t,
		strconv.FormatUint(uint64(config.GatewayManagementInfo.Port), 10),
		volume.GetVolumeContext()["GatewayPort"])
}

func TestGetGatewayConfigFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params     map[string]string
		shouldFail bool
	}{
		{
			params:     nil,
			shouldFail: true,
		},
		{
			params: map[string]string{
				"nvmeofGatewayAddress": "127.0.0.1",
			},
			shouldFail: true,
		},
		{
			params: map[string]string{
				"nvmeofGatewayPort": "5500",
			},
			shouldFail: true,
		},
		{
			params: map[string]string{
				"nvmeofGatewayAddress": "127.0.0.1",
				"nvmeofGatewayPort":    "5500",
			},
		},
	}

	for _, test := range tests {
		config, err := getGatewayConfigFromRequest(test.params)
		if test.shouldFail {
			require.Error(t, err)

			continue
		}

		require.NoError(t, err)
		require.NotNil(t, config)
	}
}
