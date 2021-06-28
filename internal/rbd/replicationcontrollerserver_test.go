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

	"github.com/ceph/go-ceph/rbd/admin"
)

func TestValidateSchedulingInterval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		interval string
		want     admin.Interval
		wantErr  bool
	}{
		{
			"valid interval in minutes",
			"3m",
			admin.Interval("3m"),
			false,
		},
		{
			"valid interval in hour",
			"22h",
			admin.Interval("22h"),
			false,
		},
		{
			"valid interval in days",
			"13d",
			admin.Interval("13d"),
			false,
		},
		{
			"invalid interval without number",
			"d",
			admin.Interval(""),
			true,
		},
		{
			"invalid interval without (m|h|d) suffix",
			"12",
			admin.Interval(""),
			true,
		},
	}
	for _, tt := range tests {
		st := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateSchedulingInterval(st.interval)
			if (err != nil) != st.wantErr {
				t.Errorf("validateSchedulingInterval() error = %v, wantErr %v", err, st.wantErr)
				return
			}
			if !reflect.DeepEqual(got, st.want) {
				t.Errorf("validateSchedulingInterval() = %v, want %v", got, st.want)
			}
		})
	}
}

func TestGetSchedulingDetails(t *testing.T) {
	tests := []struct {
		name          string
		parameters    map[string]string
		wantInterval  admin.Interval
		wantStartTime admin.StartTime
		wantErr       bool
	}{
		{
			"valid parameters",
			map[string]string{
				imageMirroringKey:      string(imageMirrorModeSnapshot),
				schedulingIntervalKey:  "1h",
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			admin.Interval("1h"),
			admin.StartTime("14:00:00-05:00"),
			false,
		},
		{
			"valid parameters when optional startTime is missing",
			map[string]string{
				imageMirroringKey:     string(imageMirrorModeSnapshot),
				schedulingIntervalKey: "1h",
			},
			admin.Interval("1h"),
			admin.NoStartTime,
			false,
		},
		{
			"when mirroring mode is journal",
			map[string]string{
				imageMirroringKey:     "journal",
				schedulingIntervalKey: "1h",
			},
			admin.NoInterval,
			admin.NoStartTime,
			true,
		},
		{
			"when startTime is specified without interval",
			map[string]string{
				imageMirroringKey:      string(imageMirrorModeSnapshot),
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			admin.NoInterval,
			admin.NoStartTime,
			true,
		},
		{
			"when no scheduling is specified",
			map[string]string{
				imageMirroringKey: string(imageMirrorModeSnapshot),
			},
			admin.NoInterval,
			admin.NoStartTime,
			false,
		},
	}
	for _, tt := range tests {
		st := tt
		t.Run(tt.name, func(t *testing.T) {
			interval, startTime, err := getSchedulingDetails(st.parameters)
			if (err != nil) != st.wantErr {
				t.Errorf("getSchedulingDetails() error = %v, wantErr %v", err, st.wantErr)
				return
			}
			if !reflect.DeepEqual(interval, st.wantInterval) {
				t.Errorf("getSchedulingDetails() interval = %v, want %v", interval, st.wantInterval)
			}
			if !reflect.DeepEqual(startTime, st.wantStartTime) {
				t.Errorf("getSchedulingDetails() startTime = %v, want %v", startTime, st.wantStartTime)
			}
		})
	}
}
