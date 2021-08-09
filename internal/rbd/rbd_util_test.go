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
		rv.imageFeatureSet = librbd.FeatureSetFromNames(strings.Split(test.features, ","))
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
			"layering,journaling",
			&rbdVolume{
				Mounter: rbdNbdMounter,
			},
			true,
			"feature journaling requires exclusive-lock to be set",
		},
		{
			"layering,exclusive-lock,journaling",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature exclusive-lock requires rbd-nbd for mounter",
		},
		{
			"layering,exclusive-lock,journaling",
			&rbdVolume{
				Mounter: rbdDefaultMounter,
			},
			true,
			"feature exclusive-lock requires rbd-nbd for mounter",
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

func TestGetMappedID(t *testing.T) {
	t.Parallel()
	type args struct {
		key   string
		value string
		id    string
	}
	tests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			name: "test for matching key",
			args: args{
				key:   "cluster1",
				value: "cluster2",
				id:    "cluster1",
			},
			expected: "cluster2",
		},
		{
			name: "test for matching value",
			args: args{
				key:   "cluster1",
				value: "cluster2",
				id:    "cluster2",
			},
			expected: "cluster1",
		},
		{
			name: "test for invalid match",
			args: args{
				key:   "cluster1",
				value: "cluster2",
				id:    "cluster3",
			},
			expected: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := getMappedID(tt.args.key, tt.args.value, tt.args.id)
			if val != tt.expected {
				t.Errorf("getMappedID() got = %v, expected %v", val, tt.expected)
			}
		})
	}
}
