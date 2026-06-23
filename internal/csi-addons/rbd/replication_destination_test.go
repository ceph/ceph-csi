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

package rbd

import (
	"context"
	"testing"

	"github.com/csi-addons/spec/lib/go/replication"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ceph/ceph-csi/api/deploy/kubernetes"
)

// TestGetDestinationIDFromCSIID tests the CSI ID mapping logic for replication destinations.
func TestGetDestinationIDFromCSIID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name        string
		srcID       string
		clusterID   string
		poolName    string
		destInfo    *kubernetes.ReplicationDestinationInfo
		wantDestID  string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid mapping with pool remapping",
			srcID:     "0001-000f-primary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			clusterID: "primary-cluster",
			poolName:  "rbd",
			destInfo: &kubernetes.ReplicationDestinationInfo{
				RemoteClusterID: "secondary-cluster",
				RBD: &kubernetes.RemoteRBDDetails{
					RemotePoolMapping: map[string]kubernetes.RemotePoolDetails{
						"rbd": {PoolID: "5"},
					},
				},
			},
			wantDestID: "0001-0011-secondary-cluster-0000000000000005-00000000-1111-2222-3333-444444444444",
			wantErr:    false,
		},
		{
			name:      "valid mapping without pool remapping",
			srcID:     "0001-000f-primary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			clusterID: "primary-cluster",
			poolName:  "replicapool",
			destInfo: &kubernetes.ReplicationDestinationInfo{
				RemoteClusterID: "secondary-cluster",
				RBD: &kubernetes.RemoteRBDDetails{
					RemotePoolMapping: map[string]kubernetes.RemotePoolDetails{
						"rbd": {PoolID: "5"},
					},
				},
			},
			wantDestID: "0001-0011-secondary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			wantErr:    false,
		},
		{
			name:       "no destination configured returns same ID",
			srcID:      "0001-000f-primary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			clusterID:  "primary-cluster",
			poolName:   "rbd",
			destInfo:   nil,
			wantDestID: "0001-000f-primary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			wantErr:    false,
		},
		{
			name:      "invalid source ID",
			srcID:     "invalid-csi-id",
			clusterID: "primary-cluster",
			poolName:  "rbd",
			destInfo: &kubernetes.ReplicationDestinationInfo{
				RemoteClusterID: "secondary-cluster",
				RBD: &kubernetes.RemoteRBDDetails{
					RemotePoolMapping: map[string]kubernetes.RemotePoolDetails{
						"rbd": {PoolID: "5"},
					},
				},
			},
			wantErr:     true,
			errContains: "failed to decompose source CSI ID",
		},
		{
			name:      "empty remote cluster ID",
			srcID:     "0001-000f-primary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			clusterID: "primary-cluster",
			poolName:  "rbd",
			destInfo: &kubernetes.ReplicationDestinationInfo{
				RemoteClusterID: "",
				RBD: &kubernetes.RemoteRBDDetails{
					RemotePoolMapping: map[string]kubernetes.RemotePoolDetails{},
				},
			},
			wantErr:     true,
			errContains: "remoteClusterID is empty",
		},
		{
			name:      "invalid remote pool ID format",
			srcID:     "0001-000f-primary-cluster-0000000000000001-00000000-1111-2222-3333-444444444444",
			clusterID: "primary-cluster",
			poolName:  "rbd",
			destInfo: &kubernetes.ReplicationDestinationInfo{
				RemoteClusterID: "secondary-cluster",
				RBD: &kubernetes.RemoteRBDDetails{
					RemotePoolMapping: map[string]kubernetes.RemotePoolDetails{
						"rbd": {PoolID: "invalid"},
					},
				},
			},
			wantErr:     true,
			errContains: "invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			destID, err := getDestinationIDFromCSIID(ctx, tt.srcID, tt.clusterID, tt.poolName, tt.destInfo)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantDestID, destID)
			}
		})
	}
}

// TestGetReplicationDestinationInfo_RequestValidation tests request validation for GetReplicationDestinationInfo RPC.
func TestGetReplicationDestinationInfo_RequestValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rs := &ReplicationServer{
		driverInstance: "test-driver",
	}

	tests := []struct {
		name        string
		req         *replication.GetReplicationDestinationInfoRequest
		wantCode    codes.Code
		errContains string
	}{
		{
			name:        "nil replication source",
			req:         &replication.GetReplicationDestinationInfoRequest{},
			wantCode:    codes.InvalidArgument,
			errContains: "replication source is required",
		},
		{
			name: "empty volume ID",
			req: &replication.GetReplicationDestinationInfoRequest{
				ReplicationSource: &replication.ReplicationSource{
					Type: &replication.ReplicationSource_Volume{
						Volume: &replication.ReplicationSource_VolumeSource{
							VolumeId: "",
						},
					},
				},
			},
			wantCode:    codes.InvalidArgument,
			errContains: "empty volume ID",
		},
		{
			name: "empty volume group ID",
			req: &replication.GetReplicationDestinationInfoRequest{
				ReplicationSource: &replication.ReplicationSource{
					Type: &replication.ReplicationSource_Volumegroup{
						Volumegroup: &replication.ReplicationSource_VolumeGroupSource{
							VolumeGroupId: "",
						},
					},
				},
			},
			wantCode:    codes.InvalidArgument,
			errContains: "empty volume group ID",
		},
		{
			name: "neither volume nor volume group specified",
			req: &replication.GetReplicationDestinationInfoRequest{
				ReplicationSource: &replication.ReplicationSource{
					Type: nil,
				},
			},
			wantCode:    codes.InvalidArgument,
			errContains: "either volume or volumegroup source must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := rs.GetReplicationDestinationInfo(ctx, tt.req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "error should be a gRPC status error")
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Contains(t, st.Message(), tt.errContains)
		})
	}
}
