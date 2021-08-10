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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateSchedulingInterval(tt.interval)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSchedulingInterval() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateSchedulingInterval() = %v, want %v", got, tt.want)
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
		{
			"when no parameters and scheduling details are specified",
			map[string]string{},
			admin.NoInterval,
			admin.NoStartTime,
			false,
		},
		{
			"when no mirroring mode is specified",
			map[string]string{
				schedulingIntervalKey:  "1h",
				schedulingStartTimeKey: "14:00:00-05:00",
			},
			admin.Interval("1h"),
			admin.StartTime("14:00:00-05:00"),
			false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			interval, startTime, err := getSchedulingDetails(tt.parameters)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSchedulingDetails() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(interval, tt.wantInterval) {
				t.Errorf("getSchedulingDetails() interval = %v, want %v", interval, tt.wantInterval)
			}
			if !reflect.DeepEqual(startTime, tt.wantStartTime) {
				t.Errorf("getSchedulingDetails() startTime = %v, want %v", startTime, tt.wantStartTime)
			}
		})
	}
}
