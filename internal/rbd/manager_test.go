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

package rbd

import (
	"context"
	"testing"
)

func TestMakeVolumeGroupID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// parameters for NewManager()
		parameters map[string]string

		// arguments for MakeVolumeGroupID()
		poolID    int64
		groupName string // should include the "volumeGroupNamePrefix"

		// results
		volumeGroupID string
		expectError   bool
	}{
		{
			name:          "no parameters set",
			parameters:    nil,
			volumeGroupID: "",
			expectError:   true,
		},
		{
			name: "missing 'clusterID' parameter",
			parameters: map[string]string{
				"pool":    "replicapool",
				"missing": "clusterID",
			},
			poolID:        1,
			groupName:     "csi-vol-group-my-volume-group",
			volumeGroupID: "",
			expectError:   true,
		},
		{
			name: "good volume group with default prefix",
			parameters: map[string]string{
				"pool":      "replicapool",
				"clusterID": "k8s-rook-ceph",
			},
			poolID:        1,
			groupName:     "csi-vol-group-1fac1545-2d0f-4b42-8abb-066ebc39cba9",
			volumeGroupID: "0001-000d-k8s-rook-ceph-0000000000000001-1fac1545-2d0f-4b42-8abb-066ebc39cba9",
			expectError:   false,
		},
		{
			name: "volume group with altervative prefix",
			parameters: map[string]string{
				"pool":                  "replicapool",
				"clusterID":             "k8s-rook-ceph",
				"volumeGroupNamePrefix": "its-a-group-",
			},
			poolID:        1,
			groupName:     "its-a-group-df4a7204-6b06-4eb3-9547-036d845f0cd3",
			volumeGroupID: "0001-000d-k8s-rook-ceph-0000000000000001-df4a7204-6b06-4eb3-9547-036d845f0cd3",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.TODO()
			mgr := NewManager("rbd.example.org", tt.parameters, nil)
			defer mgr.Destroy(ctx)

			id, err := mgr.MakeVolumeGroupID(ctx, tt.poolID, tt.groupName)
			if (err != nil) != tt.expectError {
				t.Logf("mgr: %+v", mgr)
				t.Errorf("MakeVolumeGroupID failed unexpectedly: %v", err)
			} else if id != tt.volumeGroupID {
				t.Errorf("MakeVolumeGroupID returned %q, expected %q", id, tt.volumeGroupID)
			}
		})
	}
}
