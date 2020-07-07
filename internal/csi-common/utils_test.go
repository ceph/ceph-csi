/*
Copyright 2019 ceph-csi authors.

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

package csicommon // nolint:testpackage // we're testing internals of the package.

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

var fakeID = "fake-id"

func TestGetReqID(t *testing.T) {
	req := []interface{}{
		&csi.CreateVolumeRequest{
			Name: fakeID,
		},
		&csi.DeleteVolumeRequest{
			VolumeId: fakeID,
		},
		&csi.CreateSnapshotRequest{
			Name: fakeID,
		},
		&csi.DeleteSnapshotRequest{
			SnapshotId: fakeID,
		},

		&csi.ControllerExpandVolumeRequest{
			VolumeId: fakeID,
		},
		&csi.NodeStageVolumeRequest{
			VolumeId: fakeID,
		},
		&csi.NodeUnstageVolumeRequest{
			VolumeId: fakeID,
		},
		&csi.NodePublishVolumeRequest{
			VolumeId: fakeID,
		},
		&csi.NodeUnpublishVolumeRequest{
			VolumeId: fakeID,
		},
		&csi.NodeExpandVolumeRequest{
			VolumeId: fakeID,
		},
	}
	for _, r := range req {
		if got := getReqID(r); got != fakeID {
			t.Errorf("getReqID() = %v, want %v", got, fakeID)
		}
	}

	// test for nil request
	if got := getReqID(nil); got != "" {
		t.Errorf("getReqID() = %v, want empty string", got)
	}
}
