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
	"reflect"
	"testing"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
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
				imageMirroringKey:     "journal",
				schedulingIntervalKey: "1h",
			},
			true,
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
			err := validateSchedulingDetails(tt.parameters)
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
			name: "test for unknown state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateUnknown,
			},
			wantErr: false,
		},
		{
			name: "test for error state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateError,
			},
			wantErr: true,
		},
		{
			name: "test for syncing state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateSyncing,
			},
			wantErr: true,
		},
		{
			name: "test for starting_replay state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateStartingReplay,
			},
			wantErr: true,
		},
		{
			name: "test for replaying state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateReplaying,
			},
			wantErr: false,
		},
		{
			name: "test for stopping_replay state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateStoppingReplay,
			},
			wantErr: true,
		},
		{
			name: "test for stopped state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusStateStopped,
			},
			wantErr: true,
		},
		{
			name: "test for invalid state",
			args: librbd.SiteMirrorImageStatus{
				State: librbd.MirrorImageStatusState(100),
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
