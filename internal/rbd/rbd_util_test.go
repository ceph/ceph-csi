/*
Copyright 2020 The Ceph-CSI Authors.

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
	"os"
	"strings"
	"testing"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/stretchr/testify/assert"
)

func TestHasSnapshotFeature(t *testing.T) {
	t.Parallel()
	tests := []struct {
		features   string
		hasFeature bool
	}{
		{"foo", false},
		{"foo,bar", false},
		{"foo,layering,bar", true},
	}

	rv := rbdVolume{}

	for _, test := range tests {
		rv.ImageFeatureSet = librbd.FeatureSetFromNames(strings.Split(test.features, ","))
		if got := rv.hasSnapshotFeature(); got != test.hasFeature {
			t.Errorf("hasSnapshotFeature(%s) = %t, want %t", test.features, got, test.hasFeature)
		}
	}
}

func TestValidateImageFeatures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		imageFeatures string
		rbdVol        *rbdVolume
		isErr         bool
		errMsg        string
	}{
		{
			"",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			false,
			"",
		},
		{
			"layering",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			false,
			"",
		},
		{
			"layering",
			&rbdVolume{
				Mounter: rbdNbdMounter,
			},
			false,
			"",
		},
		{
			"layering,exclusive-lock,journaling",
			&rbdVolume{
				Mounter: rbdNbdMounter,
			},
			false,
			"",
		},
		{
			"layering,exclusive-lock,object-map,fast-diff",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			false,
			"",
		},
		{
			"layering,journaling",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature journaling requires exclusive-lock to be set",
		},
		{
			"object-map,fast-diff",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature object-map requires exclusive-lock to be set",
		},
		{
			"fast-diff",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature fast-diff requires object-map to be set",
		},
		{
			"layering,exclusive-lock,journaling",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature journaling requires rbd-nbd for mounter",
		},
		{
			"layering,exclusive-lock,journaling",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature journaling requires rbd-nbd for mounter",
		},
		{
			"layering,exclusive-loc,journaling",
			&rbdVolume{
				Mounter: rbdNbdMounter,
			},
			true,
			"invalid feature exclusive-loc",
		},
		{
			"ayering",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"invalid feature ayering",
		},
		{
			"deep-flatten",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			false,
			"",
		},
	}

	for _, test := range tests {
		err := test.rbdVol.validateImageFeatures(test.imageFeatures)
		if test.isErr {
			assert.EqualError(t, err, test.errMsg)

			continue
		}
		assert.Nil(t, err)
	}
}

func TestGetCephClientLogFileName(t *testing.T) {
	t.Parallel()
	type args struct {
		id     string
		logDir string
		prefix string
	}
	volID := "0001-0024-fed5480a-f00f-417a-a51d-31d8a8144c03-0000000000000003-eba90b33-0156-11ec-a30b-4678a93686c2"
	tests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			name: "test for empty id",
			args: args{
				id:     "",
				logDir: "/var/log/ceph-csi",
				prefix: "rbd-nbd",
			},
			expected: "/var/log/ceph-csi/rbd-nbd-.log",
		},
		{
			name: "test for empty logDir",
			args: args{
				id:     volID,
				logDir: "",
				prefix: "rbd-nbd",
			},
			expected: "/var/log/ceph/rbd-nbd-" + volID + ".log",
		},
		{
			name: "test for empty prefix",
			args: args{
				id:     volID,
				logDir: "/var/log/ceph-csi",
				prefix: "",
			},
			expected: "/var/log/ceph-csi/ceph-" + volID + ".log",
		},
		{
			name: "test for all unavailable args",
			args: args{
				id:     "",
				logDir: "",
				prefix: "",
			},
			expected: "/var/log/ceph/ceph-.log",
		},
		{
			name: "test for all available args",
			args: args{
				id:     volID,
				logDir: "/var/log/ceph-csi",
				prefix: "rbd-nbd",
			},
			expected: "/var/log/ceph-csi/rbd-nbd-" + volID + ".log",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := getCephClientLogFileName(tt.args.id, tt.args.logDir, tt.args.prefix)
			if val != tt.expected {
				t.Errorf("getCephClientLogFileName() got = %v, expected %v", val, tt.expected)
			}
		})
	}
}

func TestStrategicActionOnLogFile(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()
	tmpDir := t.TempDir()

	var logFile [3]string
	for i := 0; i < 3; i++ {
		f, err := os.CreateTemp(tmpDir, "rbd-*.log")
		if err != nil {
			t.Errorf("creating tempfile failed: %v", err)
		}
		logFile[i] = f.Name()
	}

	type args struct {
		logStrategy string
		logFile     string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test for compress",
			args: args{
				logStrategy: "compress",
				logFile:     logFile[0],
			},
		},
		{
			name: "test for remove",
			args: args{
				logStrategy: "remove",
				logFile:     logFile[1],
			},
		},
		{
			name: "test for preserve",
			args: args{
				logStrategy: "preserve",
				logFile:     logFile[2],
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			strategicActionOnLogFile(ctx, tt.args.logStrategy, tt.args.logFile)

			var err error
			switch tt.args.logStrategy {
			case "compress":
				newExt := strings.ReplaceAll(tt.args.logFile, ".log", ".gz")
				if _, err = os.Stat(newExt); os.IsNotExist(err) {
					t.Errorf("compressed logFile (%s) not found: %v", newExt, err)
				}
				os.Remove(newExt)
			case "remove":
				if _, err = os.Stat(tt.args.logFile); !os.IsNotExist(err) {
					t.Errorf("logFile (%s) not removed: %v", tt.args.logFile, err)
				}
			case "preserve":
				if _, err = os.Stat(tt.args.logFile); os.IsNotExist(err) {
					t.Errorf("logFile (%s) not preserved: %v", tt.args.logFile, err)
				}
				os.Remove(tt.args.logFile)
			}
		})
	}
}

func TestIsKrbdFeatureSupported(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()

	tests := []struct {
		name        string
		featureName string
		isSupported bool
	}{
		{
			name:        "supported feature",
			featureName: "layering",
			isSupported: true,
		},
		{
			name:        "not supported feature",
			featureName: "journaling",
			isSupported: false,
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var err error
			krbdSupportedFeaturesAttr := "0x1"
			krbdFeatures, err = HexStringToInteger(krbdSupportedFeaturesAttr) // initialize krbdFeatures
			if err != nil {
				t.Errorf("HexStringToInteger(%s) failed", krbdSupportedFeaturesAttr)
			}
			// In case /sys/bus/rbd/supported_features is absent and we are
			// not in a position to prepare krbd feature attributes,
			// isKrbdFeatureSupported returns error ErrNotExist
			supported, err := isKrbdFeatureSupported(ctx, tc.featureName)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("isKrbdFeatureSupported(%s) returned error: %v", tc.featureName, err)
			} else if supported != tc.isSupported {
				t.Errorf("isKrbdFeatureSupported(%s) returned supported status, expected: %t, got: %t",
					tc.featureName, tc.isSupported, supported)
			}
		})
	}
}

func Test_checkValidImageFeatures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		imageFeatures string
		ok            bool
		want          bool
	}{
		{
			name:          "test for valid image features",
			imageFeatures: "layering,exclusive-lock,object-map,fast-diff,deep-flatten",
			ok:            true,
			want:          true,
		},
		{
			name:          "test for empty image features",
			imageFeatures: "",
			ok:            true,
			want:          false,
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := checkValidImageFeatures(tc.imageFeatures, tc.ok); got != tc.want {
				t.Errorf("checkValidImageFeatures() = %v, want %v", got, tc.want)
			}
		})
	}
}
