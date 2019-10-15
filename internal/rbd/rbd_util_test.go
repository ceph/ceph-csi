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
	"testing"

	"github.com/stretchr/testify/assert"
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

	for _, test := range tests {
		if got := hasSnapshotFeature(test.features); got != test.hasFeature {
			t.Errorf("hasSnapshotFeature(%s) = %t, want %t", test.features, got, test.hasFeature)
		}
	}
}

func TestValidateImageFeatures(t *testing.T) {
	tests := []struct {
		imageFeatures string
		mounter       string
		isErr         bool
		errMsg        string
	}{
		{
			"layering",
			rbdDefaultMounter,
			false,
			"",
		},
		{
			"layering",
			rbdNbdMounter,
			false,
			"",
		},
		{
			"layering,exclusive-lock,journaling",
			rbdNbdMounter,
			false,
			"",
		},
		{
			"layering,journaling",
			rbdNbdMounter,
			true,
			"feature journaling requires exclusive-lock",
		},
		{
			"layering,exclusive-lock,journaling",
			"",
			true,
			"feature exclusive-lock requires rbd-nbd for mounter",
		},
		{
			"layering,exclusive-lock,journaling",
			rbdDefaultMounter,
			true,
			"feature exclusive-lock requires rbd-nbd for mounter",
		},
		{
			"layering,exclusive-loc,journaling",
			rbdNbdMounter,
			true,
			"invalid feature exclusive-loc for volume csi-rbdplugin",
		},
		{
			"ayering",
			rbdDefaultMounter,
			true,
			"invalid feature ayering for volume csi-rbdplugin",
		},
	}

	for _, test := range tests {
		err := validateImageFeatures(test.imageFeatures, test.mounter)
		if test.isErr {
			assert.EqualError(t, err, test.errMsg)
		} else {
			assert.Nil(t, err)
		}
	}
}
