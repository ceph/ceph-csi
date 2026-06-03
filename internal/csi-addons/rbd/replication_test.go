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
	"testing"
	"time"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
	"github.com/csi-addons/spec/lib/go/replication"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corerbd "github.com/ceph/ceph-csi/internal/rbd"
	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
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
	ctx := t.Context()
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
	ctx := t.Context()
	t.Parallel()
	tests := []struct {
		name    string
		args    corerbd.SiteMirrorImageStatus
		wantErr bool
	}{
		{
			name: "test when local_snapshot_timestamp is non zero",
			args: corerbd.SiteMirrorImageStatus{
				SiteMirrorImageStatus: librbd.SiteMirrorImageStatus{
					//nolint:lll // sample output cannot be split into multiple lines.
					Description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
				},
			},
			wantErr: false,
		},
		{
			name: "test when local_snapshot_timestamp is zero",
			//nolint:lll // sample output cannot be split into multiple lines.
			args: corerbd.SiteMirrorImageStatus{
				SiteMirrorImageStatus: librbd.SiteMirrorImageStatus{
					Description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":0,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
				},
			},
			wantErr: true,
		},
		{
			name: "test when local_snapshot_timestamp is not present",
			//nolint:lll // sample output cannot be split into multiple lines.
			args: corerbd.SiteMirrorImageStatus{
				SiteMirrorImageStatus: librbd.SiteMirrorImageStatus{
					Description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := checkVolumeResyncStatus(ctx, tt.args); (err != nil) != tt.wantErr {
				t.Errorf("checkVolumeResyncStatus() error = %v, expect error = %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckRemoteSiteStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		args      corerbd.GlobalMirrorStatus
		wantReady bool
	}{
		{
			name: "Test a single peer in sync",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID: "remote",
							State:      librbd.MirrorImageStatusStateUnknown,
							Up:         true,
						},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Test a single peer in sync, including a local instance",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
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
			},
			wantReady: true,
		},
		{
			name: "Test a multiple peers in sync",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
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
			},
			wantReady: true,
		},
		{
			name: "Test no remote peers",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{},
				},
			},
			wantReady: false,
		},
		{
			name: "Test single peer not in sync",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID: "remote",
							State:      librbd.MirrorImageStatusStateReplaying,
							Up:         true,
						},
					},
				},
			},
			wantReady: false,
		},
		{
			name: "Test single peer not up",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID: "remote",
							State:      librbd.MirrorImageStatusStateUnknown,
							Up:         false,
						},
					},
				},
			},
			wantReady: false,
		},
		{
			name: "Test multiple peers, when first peer is not in sync",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
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
			},
			wantReady: false,
		},
		{
			name: "Test multiple peers, when second peer is not up",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
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
			},
			wantReady: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if ready := checkRemoteSiteStatus(t.Context(), tt.args.GetAllSitesStatus()); ready != tt.wantReady {
				t.Errorf("checkRemoteSiteStatus() ready = %v, expect ready = %v", ready, tt.wantReady)
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
			name:        "InvalidArgument",
			err:         rbderrors.ErrInvalidArgument,
			expectedErr: status.Error(codes.InvalidArgument, rbderrors.ErrInvalidArgument.Error()),
		},
		{
			name:        "Aborted",
			err:         rbderrors.ErrAborted,
			expectedErr: status.Error(codes.Aborted, rbderrors.ErrAborted.Error()),
		},
		{
			name:        "FailedPrecondition",
			err:         rbderrors.ErrFailedPrecondition,
			expectedErr: status.Error(codes.FailedPrecondition, rbderrors.ErrFailedPrecondition.Error()),
		},
		{
			name:        "Unavailable",
			err:         rbderrors.ErrUnavailable,
			expectedErr: status.Error(codes.Unavailable, rbderrors.ErrUnavailable.Error()),
		},
		{
			name:        "InvalidError",
			err:         errors.New("some error"),
			expectedErr: status.Error(codes.Internal, "some error"),
		},
		{
			name:        "NilError",
			err:         nil,
			expectedErr: status.Error(codes.OK, "ok string"),
		},
		{
			name:        "ErrImageNotFound",
			err:         rbderrors.ErrImageNotFound,
			expectedErr: status.Error(codes.NotFound, rbderrors.ErrImageNotFound.Error()),
		},
		{
			name:        "ErrPoolNotFound",
			err:         util.ErrPoolNotFound,
			expectedErr: status.Error(codes.NotFound, util.ErrPoolNotFound.Error()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getGRPCError(tt.err)
			require.Equal(t, tt.expectedErr, result)
		})
	}
}

func Test_timestampFromString(t *testing.T) {
	tm := time.Now()
	t.Parallel()
	tests := []struct {
		name      string
		timestamp string
		want      time.Time
		wantErr   bool
	}{
		{
			name:      "valid timestamp",
			timestamp: timestampToString(&tm),
			want:      tm,
			wantErr:   false,
		},
		{
			name:      "invalid timestamp",
			timestamp: "invalid",
			want:      time.Time{},
			wantErr:   true,
		},
		{
			name:      "empty timestamp",
			timestamp: "",
			want:      time.Time{},
			wantErr:   true,
		},
		{
			name:      "invalid format",
			timestamp: "seconds:%d nanos:%d",
			want:      time.Time{},
			wantErr:   true,
		},
		{
			name:      "missing nanos",
			timestamp: "seconds:10",
			want:      time.Time{},
			wantErr:   true,
		},
		{
			name:      "missing seconds",
			timestamp: "nanos:0",
			want:      time.Time{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := timestampFromString(tt.timestamp)
			if (err != nil) != tt.wantErr {
				t.Errorf("timestampFromString() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.want.Equal(got) {
				t.Errorf("timestampFromString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_getFlattenMode(t *testing.T) {
	t.Parallel()
	type args struct {
		ctx        context.Context
		parameters map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    types.FlattenMode
		wantErr bool
	}{
		{
			name: "flattenMode option not set",
			args: args{
				ctx:        t.Context(),
				parameters: map[string]string{},
			},
			want: types.FlattenModeNever,
		},
		{
			name: "flattenMode option set to never",
			args: args{
				ctx: t.Context(),
				parameters: map[string]string{
					flattenModeKey: string(types.FlattenModeNever),
				},
			},
			want: types.FlattenModeNever,
		},
		{
			name: "flattenMode option set to force",
			args: args{
				ctx: t.Context(),
				parameters: map[string]string{
					flattenModeKey: string(types.FlattenModeForce),
				},
			},
			want: types.FlattenModeForce,
		},

		{
			name: "flattenMode option set to invalid value",
			args: args{
				ctx: t.Context(),
				parameters: map[string]string{
					flattenModeKey: "invalid123",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := getFlattenMode(tt.args.ctx, tt.args.parameters)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFlattenMode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getFlattenMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getCurrentReplicationStatus(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tests := []struct {
		name      string
		args      corerbd.GlobalMirrorStatus
		status    replication.GetVolumeReplicationInfoResponse_Status
		statusMsg string
	}{
		{
			name: "test when mirroring is down",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID:  "",
							State:       librbd.MirrorImageStatusStateUnknown,
							Description: "status is unknown",
							Up:          false,
						},
						{
							MirrorUUID:  "remote",
							State:       librbd.MirrorImageStatusStateUnknown,
							Description: "status is unknown",
							Up:          false,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_DEGRADED,
			statusMsg: "status is unknown",
		},
		{
			name: "test when mirroring is up and local is primary",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID:  "",
							State:       librbd.MirrorImageStatusStateStopped,
							Description: "local image is primary",
							Up:          true,
						},
						{
							MirrorUUID: "remote",
							State:      librbd.MirrorImageStatusStateReplaying,
							//nolint:lll // sample output cannot be split into multiple lines.
							Description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
							Up:          true,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_HEALTHY,
			statusMsg: "local image is primary",
		},
		{
			name: "test when mirroring is up and local is secondary",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID: "",
							State:      librbd.MirrorImageStatusStateReplaying,
							//nolint:lll // sample output cannot be split into multiple lines.
							Description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
							Up:          true,
						},
						{
							MirrorUUID:  "remote",
							State:       librbd.MirrorImageStatusStateStopped,
							Description: "local image is primary",
							Up:          true,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_HEALTHY,
			statusMsg: "replaying",
		},
		{
			name: "test when mirroring is up and in error state",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID:  "",
							State:       librbd.MirrorImageStatusStateError,
							Description: "error",
							Up:          true,
						},
						{
							MirrorUUID: "remote",
							State:      librbd.MirrorImageStatusStateReplaying,
							//nolint:lll // sample output cannot be split into multiple lines.
							Description: `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,"last_snapshot_bytes":81920,"last_snapshot_sync_seconds":56743,"local_snapshot_timestamp":1684675261,"remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`,
							Up:          true,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_ERROR,
			statusMsg: "error",
		},
		{
			name: "test when resync is required",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID:  "",
							State:       librbd.MirrorImageStatusStateError,
							Description: "split-brain detected",
							Up:          true,
						},
						{
							MirrorUUID:  "remote",
							State:       librbd.MirrorImageStatusStateStopped,
							Description: "local image is primary",
							Up:          true,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_ERROR,
			statusMsg: "split-brain detected",
		},
		{
			name: "test when both the clusters are secondary and sync is complete",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID:  "",
							State:       librbd.MirrorImageStatusStateUnknown,
							Description: "remote image is not primary",
							Up:          true,
						},
						{
							MirrorUUID:  "remote",
							State:       librbd.MirrorImageStatusStateUnknown,
							Description: "remote image is not primary",
							Up:          true,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_UNKNOWN,
			statusMsg: "remote image is not primary",
		},
		{
			name: "test when secondary is starting replaying",
			args: corerbd.GlobalMirrorStatus{
				GlobalMirrorImageStatus: librbd.GlobalMirrorImageStatus{
					SiteStatuses: []librbd.SiteMirrorImageStatus{
						{
							MirrorUUID:  "",
							State:       librbd.MirrorImageStatusStateStartingReplay,
							Description: "starting replay",
							Up:          true,
						},
						{
							MirrorUUID:  "remote",
							State:       librbd.MirrorImageStatusStateStopped,
							Description: "local image is primary",
							Up:          true,
						},
					},
				},
			},
			status:    replication.GetVolumeReplicationInfoResponse_UNKNOWN,
			statusMsg: "starting replay",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			respStatus, respStatusMsg := getCurrentReplicationStatus(ctx, tt.args)
			if !reflect.DeepEqual(tt.status, respStatus) {
				t.Errorf("getCurrentReplicationStatus() returned status = %v, want = %v", respStatus, tt.status)
			}
			if !reflect.DeepEqual(tt.statusMsg, respStatusMsg) {
				t.Errorf("getCurrentReplicationStatus() returned statusMsg = %v, want = %v", respStatusMsg, tt.statusMsg)
			}
		})
	}
}
