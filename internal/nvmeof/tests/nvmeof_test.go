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

package integration

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/nvmeof"
)

func TestRealGateway(t *testing.T) {
	t.Parallel()
	// Skip if no real gateway available
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Get configuration from environment variables (REQUIRED)
	address := os.Getenv("NVMEOF_GATEWAY_ADDRESS")
	portstr := os.Getenv("NVMEOF_GATEWAY_PORT")
	hostname := os.Getenv("NVMEOF_GATEWAY_HOSTNAME")
	nvmeofListenerPortStr := os.Getenv("NVMEOF_LISTENER_PORT")

	// Fail if any required environment variable is missing
	if address == "" {
		t.Skip("NVMEOF_GATEWAY_ADDRESS environment variable is required")
	}
	if portstr == "" {
		t.Skip("NVMEOF_GATEWAY_PORT environment variable is required")
	}
	if hostname == "" {
		t.Skip("NVMEOF_GATEWAY_HOSTNAME environment variable is required")
	}
	if nvmeofListenerPortStr == "" {
		t.Skip("NVMEOF_LISTENER_PORT environment variable is required")
	}
	nvmeofListenerPort, err := strconv.ParseUint(nvmeofListenerPortStr, 10, 32)
	if err != nil {
		t.Fatalf("Invalid NVMEOF_LISTENER_PORT '%s': %v", nvmeofListenerPortStr, err)
	}

	portUint32, err := strconv.ParseUint(portstr, 10, 32)
	if err != nil {
		t.Fatalf("Invalid NVMEOF_GATEWAY_PORT '%s': %v", portstr, err)
	}
	config := &nvmeof.GatewayConfig{
		Address: address,
		Port:    uint32(portUint32),
	}

	client, err := nvmeof.NewGatewayRpcClient(config)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}

	ctx := t.Context()
	testNQN := "nqn.2016-06.io.ceph:subsystem.test-integration"
	hostNQN := "nqn.2016-06.io.ceph:host.test"

	nvmeofData := &nvmeof.NVMeoFVolumeData{
		SubsystemNQN: testNQN,
		NamespaceID:  0, // will be set after namespace creation
		ListenerInfo: []nvmeof.ListenerDetails{
			{
				GatewayAddress: nvmeof.GatewayAddress{
					Address: config.Address,
					Port:    uint32(nvmeofListenerPort),
				},
				Hostname: hostname,
			},
		},
		GatewayManagementInfo: *config,
	}

	t.Cleanup(func() {
		// Cleanup resources in reverse order of creation

		// 1. Remove host (if it exists)
		if err := client.RemoveHost(ctx, nvmeofData.SubsystemNQN, hostNQN); err != nil {
			t.Logf("Cleanup warning: failed to remove host: %v", err)
		}

		// 2. Delete listener (if it exists)
		if err := client.DeleteListener(ctx, nvmeofData.SubsystemNQN, nvmeofData.ListenerInfo[0]); err != nil {
			t.Logf("Cleanup warning: failed to delete listener: %v", err)
		}

		// 3. Delete subsystem (if it exists)
		if err := client.DeleteSubsystem(ctx, nvmeofData.SubsystemNQN); err != nil {
			t.Logf("Cleanup warning: failed to delete subsystem: %v", err)
		}

		// 4. Close connection last
		if closeErr := client.Destroy(); closeErr != nil {
			t.Logf("Warning: failed to close client: %v", closeErr)
		}
	})

	// Test create subsystem
	err = client.CreateSubsystem(ctx, nvmeofData.SubsystemNQN)
	require.NoError(t, err)
	t.Logf("✓ Subsystem created: %s", nvmeofData.SubsystemNQN)

	// Test add host
	err = client.AddHost(ctx, nvmeofData.SubsystemNQN, hostNQN)
	require.NoError(t, err)
	t.Logf("✓ Host added: %s to subsystem %s", hostNQN, nvmeofData.SubsystemNQN)

	// Test check subsystem exists
	exists, err := client.SubsystemExists(ctx, nvmeofData.SubsystemNQN)
	require.NoError(t, err)
	require.True(t, exists, "Subsystem should exist")
	t.Logf("✓ Subsystem exists: %s", nvmeofData.SubsystemNQN)

	// Test create listener
	err = client.CreateListener(ctx, nvmeofData.SubsystemNQN, nvmeofData.ListenerInfo[0])
	require.NoError(t, err)
	t.Logf("✓ Listener created for subsystem %s at %s", testNQN, config)

	// Test delete listener
	err = client.DeleteListener(ctx, testNQN, nvmeofData.ListenerInfo[0])
	require.NoError(t, err)
	t.Logf("✓ Listener deleted for subsystem %s at %s", testNQN, config)

	// Test remove host
	err = client.RemoveHost(ctx, testNQN, hostNQN)
	require.NoError(t, err)
	t.Logf("✓ Host removed: %s from subsystem %s", hostNQN, testNQN)

	// Test create namespace
	// poolName := "mypool"
	// imageName := "test-image"
	// nsID, err := client.CreateNamespace(ctx, testNQN, poolName, radosNS, imageName)
	// require.NoError(t, err)
	// require.Greater(t, nsID, uint32(0), "Namespace ID should be greater than 0")

	// Test delete namespace
	// err = client.DeleteNamespace(ctx, testNQN, nsID)
	// require.NoError(t, err)

	// Test delete subsystem
	err = client.DeleteSubsystem(ctx, nvmeofData.SubsystemNQN)
	require.NoError(t, err)
	t.Logf("✓ Subsystem deleted: %s", nvmeofData.SubsystemNQN)

	// Test check subsystem does not exist after deletion
	exists, err = client.SubsystemExists(ctx, nvmeofData.SubsystemNQN)
	require.NoError(t, err)
	require.False(t, exists, "Subsystem should not exist after deletion")
	t.Logf("✓ Subsystem does not exist after deletion: %s", nvmeofData.SubsystemNQN)
}
