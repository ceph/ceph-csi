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

package rbd // nolint:testpackage // we're testing internals of the package.

import (
	"strings"
	"testing"

	librbd "github.com/ceph/go-ceph/rbd"
)

func TestIsLegacyVolumeID(t *testing.T) {
	tests := []struct {
		volID    string
		isLegacy bool
	}{
		{"prefix-bda37d42-9979-420f-9389-74362f3f98f6", false},
		{"csi-rbd-vo-f997e783-ff00-48b0-8cc7-30cb36c3df3d", false},
		{"csi-rbd-vol-this-is-clearly-not-a-valid-UUID----", false},
		{"csi-rbd-vol-b82f27de-3b3a-43f2-b5e7-9f8d0aad04e9", true},
	}

	for _, test := range tests {
		if got := isLegacyVolumeID(test.volID); got != test.isLegacy {
			t.Errorf("isLegacyVolumeID(%s) = %t, want %t", test.volID, got, test.isLegacy)
		}
	}
}

func TestHasSnapshotFeature(t *testing.T) {
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
