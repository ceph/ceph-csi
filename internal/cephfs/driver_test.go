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

package cephfs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/util"
)

func TestSetupCSIAddonsServer(t *testing.T) {
	t.Parallel()

	// endpoint in a temporary directory
	tmpDir := t.TempDir()
	endpoint := "unix://" + tmpDir + "/csi-addons.sock"

	config := &util.Config{
		CSIAddonsEndpoint: endpoint,
	}

	drv := &Driver{}
	err := drv.setupCSIAddonsServer(config)
	require.NoError(t, err)
	require.NotNil(t, drv.cas)

	// verify the socket file has been created
	_, err = os.Stat(tmpDir + "/csi-addons.sock")
	require.NoError(t, err)

	// stop the gRPC server
	drv.cas.Stop()
}
