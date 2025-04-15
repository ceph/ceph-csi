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
	"strings"
	"testing"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/rbd/types"
)

func TestValidateLastSyncInfo(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()
	duration := int64(56743)
	zero := int64(0)

	tests := []struct {
		name        string
		description string
		info        types.SyncInfo
		expectedErr string
	}{
		{
			name: "valid description",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &syncInfo{
				LocalSnapshotTime:    1684675261,
				LastSnapshotDuration: &duration,
				LastSnapshotBytes:    81920,
			},
			expectedErr: "",
		},
		{
			name:        "empty description",
			description: "",
			info: &syncInfo{
				LastSnapshotDuration: nil,
				LastSnapshotBytes:    0,
			},
			expectedErr: rbderrors.ErrLastSyncTimeNotFound.Error(),
		},
		{
			name: "description without last_snapshot_bytes",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &syncInfo{
				LastSnapshotDuration: &duration,
				LocalSnapshotTime:    1684675261,
				LastSnapshotBytes:    0,
			},
			expectedErr: "",
		},
		{
			name: "description without local_snapshot_time",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &syncInfo{
				LastSnapshotDuration: nil,
				LastSnapshotBytes:    0,
			},
			expectedErr: rbderrors.ErrLastSyncTimeNotFound.Error(),
		},
		{
			name: "description without last_snapshot_sync_seconds",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &syncInfo{
				LastSnapshotDuration: nil,
				LocalSnapshotTime:    1684675261,
				LastSnapshotBytes:    81920,
			},
			expectedErr: "",
		},
		{
			name: "description with last_snapshot_sync_seconds = 0",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_sync_seconds":0,
			"last_snapshot_bytes":81920,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &syncInfo{
				LastSnapshotDuration: &zero,
				LocalSnapshotTime:    1684675261,
				LastSnapshotBytes:    81920,
			},
			expectedErr: "",
		},
		{
			name: "description with invalid JSON",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying,{"bytes_per_second":0.0,"last_snapshot_bytes":81920","bytes_per_snapshot":149504.0","remote_snapshot_timestamp":1662655501`,
			info: &syncInfo{
				LastSnapshotDuration: nil,
				LastSnapshotBytes:    0,
			},
			expectedErr: "failed to unmarshal",
		},
		{
			name:        "description with no JSON",
			description: `replaying`,
			info: &syncInfo{
				LastSnapshotDuration: nil,
				LastSnapshotBytes:    0,
			},
			expectedErr: rbderrors.ErrLastSyncTimeNotFound.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			teststruct, err := newSyncInfo(ctx, tt.description)
			if err != nil && !strings.Contains(err.Error(), tt.expectedErr) {
				// returned error
				t.Errorf("newSyncInfo() returned error, expected: %v, got: %v",
					tt.expectedErr, err)
			}
			if teststruct != nil {
				if teststruct.GetLastSyncTime().Unix() != tt.info.GetLastSyncTime().Unix() {
					t.Errorf("name: %v, got %v, expected %v",
						tt.name,
						teststruct.GetLastSyncTime().Unix(),
						tt.info.GetLastSyncTime().Unix())
				}

				ttLastSyncDuration := tt.info.GetLastSyncDuration()
				tsLastSyncDuration := teststruct.GetLastSyncDuration()
				if ttLastSyncDuration == nil && tsLastSyncDuration != nil {
					t.Errorf("name: %v, got %v, expected %v",
						tt.name,
						ttLastSyncDuration,
						tsLastSyncDuration)
				}
				if ttLastSyncDuration != nil && tsLastSyncDuration != nil {
					ttLastSyncDurationSecs := ttLastSyncDuration.Seconds()
					tsLastSyncDurationSecs := tsLastSyncDuration.Seconds()
					if ttLastSyncDurationSecs != tsLastSyncDurationSecs {
						t.Errorf("name: %v, got %v, expected %v",
							tt.name,
							ttLastSyncDuration,
							tsLastSyncDuration)
					}
				}

				if teststruct.GetLastSyncBytes() != tt.info.GetLastSyncBytes() {
					t.Errorf("name: %v, got %v, expected %v",
						tt.name,
						teststruct.GetLastSyncBytes(),
						tt.info.GetLastSyncBytes())
				}
			}
		})
	}
}
