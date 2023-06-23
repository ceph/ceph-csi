/*
Copyright 2021 The Ceph-CSI Authors.

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
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	corerbd "github.com/ceph/ceph-csi/internal/rbd"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
	"github.com/csi-addons/spec/lib/go/replication"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestValidateSchedulingInterval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		interval string
		wantErr  bool
	}{
		{
			"valid interval in minutes",
			"3m",
			false,
		},
		{
			"valid interval in hour",
			"22h",
			false,
		},
		{
			"valid interval in days",
			"13d",
			false,
		},
		{
			"invalid interval without number",
			"d",
			true,
		},
		{
			"invalid interval without (m|h|d) suffix",
			"12",
			true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSchedulingInterval(tt.interval)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSchedulingInterval() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
		})
	}
}

func TestValidateSchedulingDetails(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()
	tests := []struct {
		name       string
		parameters map[string]string
		wantErr    bool
	}{
		{
			"valid parameters",
			map[string]string{
				imageMirroringKey:      string(imageMirrorModeSnapshot),
				schedulingIntervalKey:  "1h",
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			false,
		},
		{
			"valid parameters when optional startTime is missing",
			map[string]string{
				imageMirroringKey:     string(imageMirrorModeSnapshot),
				schedulingIntervalKey: "1h",
			},
			false,
		},
		{
			"when mirroring mode is journal",
			map[string]string{
				imageMirroringKey:     string(imageMirrorModeJournal),
				schedulingIntervalKey: "1h",
			},
			false,
		},
		{
			"when startTime is specified without interval",
			map[string]string{
				imageMirroringKey:      string(imageMirrorModeSnapshot),
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			true,
		},
		{
			"when no scheduling is specified",
			map[string]string{
				imageMirroringKey: string(imageMirrorModeSnapshot),
			},
			false,
		},
		{
			"when no parameters and scheduling details are specified",
			map[string]string{},
			false,
		},
		{
			"when no mirroring mode is specified",
			map[string]string{
				schedulingIntervalKey:  "1h",
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSchedulingDetails(ctx, tt.parameters)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSchedulingDetails() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
		})
	}
}

func TestGetSchedulingDetails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		parameters    map[string]string
		wantInterval  admin.Interval
		wantStartTime admin.StartTime
	}{
		{
			"valid parameters",
			map[string]string{
				schedulingIntervalKey:  "1h",
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			admin.Interval("1h"),
			admin.StartTime("14:00:00-05:00"),
		},
		{
			"valid parameters when optional startTime is missing",
			map[string]string{
				imageMirroringKey:     string(imageMirrorModeSnapshot),
				schedulingIntervalKey: "1h",
			},
			admin.Interval("1h"),
			admin.NoStartTime,
		},
		{
			"when startTime is specified without interval",
			map[string]string{
				imageMirroringKey:      string(imageMirrorModeSnapshot),
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			admin.NoInterval,
			admin.StartTime("14:00:00-05:00"),
		},
		{
			"when no parameters and scheduling details are specified",
			map[string]string{},
			admin.NoInterval,
			admin.NoStartTime,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			interval, startTime := getSchedulingDetails(tt.parameters)
			if !reflect.DeepEqual(interval, tt.wantInterval) {
				t.Errorf("getSchedulingDetails() interval = %v, want %v", interval, tt.wantInterval)
			}
			if !reflect.DeepEqual(startTime, tt.wantStartTime) {
				t.Errorf("getSchedulingDetails() startTime = %v, want %v", startTime, tt.wantStartTime)
			}
		})
	}
}

func TestCheckVolumeResyncStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    librbd.SiteMirrorImageStatus
		wantErr bool
	}{
		{
			name: "test when rbd mirror daemon is not running",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateUnknown,
				Up:    false,
			},
			wantErr: true,
		},
		{
			name: "test for unknown state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateUnknown,
				Up:    true,
			},
			wantErr: false,
		},
		{
			name: "test for error state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateError,
				Up:    true,
			},
			wantErr: true,
		},
		{
			name: "test for syncing state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateSyncing,
				Up:    true,
			},
			wantErr: true,
		},
		{
			name: "test for starting_replay state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateStartingReplay,
				Up:    true,
			},
			wantErr: true,
		},
		{
			name: "test for replaying state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateReplaying,
				Up:    true,
			},
			wantErr: false,
		},
		{
			name: "test for stopping_replay state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateStoppingReplay,
				Up:    true,
			},
			wantErr: true,
		},
		{
			name: "test for stopped state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateStopped,
				Up:    true,
			},
			wantErr: true,
		},
		{
			name: "test for invalid state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusState(100),
				Up:    true,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		ts := tt
		t.Run(ts.name, func(t *testing.T) {
			t.Parallel()
			if err := checkVolumeResyncStatus(ts.args); (err != nil) != ts.wantErr {
				t.Errorf("checkVolumeResyncStatus() error = %v, expect error = %v", err, ts.wantErr)
			}
		})
	}
}

func TestCheckRemoteSiteStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		args      librbd.GlobalMirrorImageStatus
		wantReady bool
	}{
		{
			name: "Test a single peer in sync",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Test a single peer in sync, including a local instance",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
					{
						MirrorUUID: "",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Test a multiple peers in sync",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote1",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
					{
						MirrorUUID: "remote2",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Test no remote peers",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{},
			},
			wantReady: false,
		},
		{
			name: "Test single peer not in sync",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote",
						State:      librbd.MirrorImageStatusStateReplaying,
						Up:         true,
					},
				},
			},
			wantReady: false,
		},
		{
			name: "Test single peer not up",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         false,
					},
				},
			},
			wantReady: false,
		},
		{
			name: "Test multiple peers, when first peer is not in sync",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote1",
						State:      librbd.MirrorImageStatusStateStoppingReplay,
						Up:         true,
					},
					{
						MirrorUUID: "remote2",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
				},
			},
			wantReady: false,
		},
		{
			name: "Test multiple peers, when second peer is not up",
			args: librbd.GlobalMirrorImageStatus{
				SiteStatuses: []librbd.SiteMirrorImageStatus{
					{
						MirrorUUID: "remote1",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         true,
					},
					{
						MirrorUUID: "remote2",
						State:      librbd.MirrorImageStatusStateUnknown,
						Up:         false,
					},
				},
			},
			wantReady: false,
		},
	}
	for _, tt := range tests {
		ts := tt
		t.Run(ts.name, func(t *testing.T) {
			t.Parallel()
			if ready := checkRemoteSiteStatus(context.TODO(), &ts.args); ready != ts.wantReady {
				t.Errorf("checkRemoteSiteStatus() ready = %v, expect ready = %v", ready, ts.wantReady)
			}
		})
	}
}

func TestValidateLastSyncInfo(t *testing.T) {
	t.Parallel()
	duration, err := time.ParseDuration(strconv.Itoa(int(56743)) + "s")
	if err != nil {
		t.Errorf("failed to parse duration)")
	}

	tests := []struct {
		name        string
		description string
		info        *replication.GetVolumeReplicationInfoResponse
		expectedErr string
	}{
		{
			name: "valid description",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncTime:     timestamppb.New(time.Unix(1684675261, 0)),
				LastSyncDuration: durationpb.New(duration),
				LastSyncBytes:    81920,
			},
			expectedErr: "",
		},
		{
			name:        "empty description",
			description: "",
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncTime:     nil,
				LastSyncDuration: nil,
				LastSyncBytes:    0,
			},
			expectedErr: corerbd.ErrLastSyncTimeNotFound.Error(),
		},
		{
			name: "description without last_snapshot_bytes",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncDuration: durationpb.New(duration),
				LastSyncTime:     timestamppb.New(time.Unix(1684675261, 0)),
				LastSyncBytes:    0,
			},
			expectedErr: "",
		},
		{
			name: "description without local_snapshot_time",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncDuration: nil,
				LastSyncTime:     nil,
				LastSyncBytes:    0,
			},
			expectedErr: corerbd.ErrLastSyncTimeNotFound.Error(),
		},
		{
			name: "description without last_snapshot_sync_seconds",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncDuration: nil,
				LastSyncTime:     timestamppb.New(time.Unix(1684675261, 0)),
				LastSyncBytes:    81920,
			},
			expectedErr: "",
		},
		{
			name: "description with last_snapshot_sync_seconds = 0",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_sync_seconds":0,
			"last_snapshot_bytes":81920,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncDuration: durationpb.New(time.Duration(0)),
				LastSyncTime:     timestamppb.New(time.Unix(1684675261, 0)),
				LastSyncBytes:    81920,
			},
			expectedErr: "",
		},
		{
			name: "description with invalid JSON",
			//nolint:lll // sample output cannot be split into multiple lines.
			description: `replaying,{"bytes_per_second":0.0,"last_snapshot_bytes":81920","bytes_per_snapshot":149504.0","remote_snapshot_timestamp":1662655501`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncDuration: nil,
				LastSyncTime:     nil,
				LastSyncBytes:    0,
			},
			expectedErr: "failed to unmarshal",
		},
		{
			name:        "description with no JSON",
			description: `replaying`,
			info: &replication.GetVolumeReplicationInfoResponse{
				LastSyncDuration: nil,
				LastSyncTime:     nil,
				LastSyncBytes:    0,
			},
			expectedErr: corerbd.ErrLastSyncTimeNotFound.Error(),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			teststruct, err := getLastSyncInfo(tt.description)
			if err != nil && !strings.Contains(err.Error(), tt.expectedErr) {
				// returned error
				t.Errorf("getLastSyncInfo() returned error, expected: %v, got: %v",
					tt.expectedErr, err)
			}
			if teststruct != nil {
				if teststruct.LastSyncTime.GetSeconds() != tt.info.LastSyncTime.GetSeconds() {
					t.Errorf("name: %v, getLastSyncInfo() %v, expected %v", tt.name, teststruct.LastSyncTime, tt.info.LastSyncTime)
				}
				if tt.info.LastSyncDuration == nil && teststruct.LastSyncDuration != nil {
					t.Errorf("name: %v, getLastSyncInfo() %v, expected %v", tt.name, teststruct.LastSyncDuration,
						tt.info.LastSyncDuration)
				}
				if teststruct.LastSyncDuration.GetSeconds() != tt.info.LastSyncDuration.GetSeconds() {
					t.Errorf("name: %v, getLastSyncInfo() %v, expected %v", tt.name, teststruct.LastSyncDuration,
						tt.info.LastSyncDuration)
				}
				if teststruct.LastSyncBytes != tt.info.LastSyncBytes {
					t.Errorf("name: %v, getLastSyncInfo() %v, expected %v", tt.name, teststruct.LastSyncBytes, tt.info.LastSyncBytes)
				}
			}
		})
	}
}

func TestGetGRPCError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		err         error
		expectedErr error
	}{
		{
			name:        "FetchingLocalStateFailed",
			err:         corerbd.ErrFetchingLocalState,
			expectedErr: status.Error(codes.Internal, corerbd.ErrFetchingLocalState.Error()),
		},
		{
			name:        "ResyncImageFailed",
			err:         corerbd.ErrResyncImageFailed,
			expectedErr: status.Error(codes.Internal, corerbd.ErrResyncImageFailed.Error()),
		},
		{
			name:        "DisableImageMirroringFailed",
			err:         corerbd.ErrDisableImageMirroringFailed,
			expectedErr: status.Error(codes.Internal, corerbd.ErrDisableImageMirroringFailed.Error()),
		},
		{
			name:        "FetchingMirroringInfoFailed",
			err:         corerbd.ErrFetchingMirroringInfo,
			expectedErr: status.Error(codes.Internal, corerbd.ErrFetchingMirroringInfo.Error()),
		},
		{
			name:        "InvalidArgument",
			err:         corerbd.ErrInvalidArgument,
			expectedErr: status.Error(codes.InvalidArgument, corerbd.ErrInvalidArgument.Error()),
		},
		{
			name:        "Aborted",
			err:         corerbd.ErrAborted,
			expectedErr: status.Error(codes.Aborted, corerbd.ErrAborted.Error()),
		},
		{
			name:        "FailedPrecondition",
			err:         corerbd.ErrFailedPrecondition,
			expectedErr: status.Error(codes.FailedPrecondition, corerbd.ErrFailedPrecondition.Error()),
		},
		{
			name:        "Unavailable",
			err:         corerbd.ErrUnavailable,
			expectedErr: status.Error(codes.Unavailable, corerbd.ErrUnavailable.Error()),
		},
		{
			name:        "InvalidError",
			err:         errors.New("some error"),
			expectedErr: status.Error(codes.Unknown, "some error"),
		},
		{
			name:        "NilError",
			err:         nil,
			expectedErr: status.Error(codes.OK, "ok string"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getGRPCError(tt.err)
			assert.Equal(t, tt.expectedErr, result)
		})
	}
}
