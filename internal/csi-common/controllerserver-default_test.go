/*
Copyright 2026 ceph-csi authors.

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

package csicommon

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
)

// mockControllerServerWithGroup implements both ControllerServer and GroupControllerServer.
type mockControllerServerWithGroup struct {
	csi.UnimplementedControllerServer
	csi.UnimplementedGroupControllerServer
}

// GroupControllerGetCapabilities implements a basic GroupControllerServer
// method. This differs from the implementation in
// csi.UnimplementedGroupControllerServer, which returns an UnimplementedError.
func (m *mockControllerServerWithGroup) GroupControllerGetCapabilities(
	ctx context.Context,
	req *csi.GroupControllerGetCapabilitiesRequest,
) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	return &csi.GroupControllerGetCapabilitiesResponse{
		Capabilities: []*csi.GroupControllerServiceCapability{},
	}, nil
}

func TestToGroupControllerServer(t *testing.T) {
	t.Parallel()

	t.Run("success - ControllerServer implements GroupControllerServer", func(t *testing.T) {
		t.Parallel()

		// Create a mock that implements both interfaces
		cs := &mockControllerServerWithGroup{}

		// This should succeed and return a GroupControllerServer
		gcs := ToGroupControllerServer(cs)
		require.NotNil(t, gcs)

		// Verify it's actually a GroupControllerServer by calling a method
		resp, err := gcs.GroupControllerGetCapabilities(
			context.Background(),
			&csi.GroupControllerGetCapabilitiesRequest{},
		)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("success - DefaultControllerServer implements GroupControllerServer", func(t *testing.T) {
		t.Parallel()

		// DefaultControllerServer should implement both interfaces
		driver := NewCSIDriver("test-driver", "v1.0.0", "test-node", "test-instance", false)
		require.NotNil(t, driver)

		cs := NewDefaultControllerServer(driver)
		require.NotNil(t, cs)

		// This should succeed
		gcs := ToGroupControllerServer(cs)
		require.NotNil(t, gcs)

		// Verify it works
		resp, err := gcs.GroupControllerGetCapabilities(
			context.Background(),
			&csi.GroupControllerGetCapabilitiesRequest{},
		)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	// Note: Testing the failure case (when ControllerServer does not implement
	// GroupControllerServer) is not practical because ToGroupControllerServer
	// calls log.FatalLogMsg which calls os.Exit() to terminate the process. In
	// production code, this is intentional as it indicates a programming error
	// that should be caught during development/testing, not at runtime.
}
