/*
Copyright 2024 The Ceph-CSI Authors.

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

package cephfs

import (
	"context"
	"testing"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestControllerServer_validateCreateVolumeGroupSnapshotRequest(t *testing.T) {
	t.Parallel()
	cs := ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(
			csicommon.NewCSIDriver("cephfs.csi.ceph.com", "1.0.0", "test")),
	}

	type args struct {
		ctx context.Context
		req *csi.CreateVolumeGroupSnapshotRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		code    codes.Code
	}{
		{
			"valid CreateVolumeGroupSnapshotRequest",
			args{
				context.Background(), &csi.CreateVolumeGroupSnapshotRequest{
					Name:            "vg-snap-1",
					SourceVolumeIds: []string{"vg-1"},
					Parameters: map[string]string{
						"clusterID": "value",
						"fsName":    "value",
					},
				},
			},
			false,
			codes.OK,
		},
		{
			"empty request name in CreateVolumeGroupSnapshotRequest",
			args{
				context.Background(), &csi.CreateVolumeGroupSnapshotRequest{
					SourceVolumeIds: []string{"vg-1"},
				},
			},
			true,
			codes.InvalidArgument,
		},
		{
			"empty SourceVolumeIds in CreateVolumeGroupSnapshotRequest",
			args{
				context.Background(), &csi.CreateVolumeGroupSnapshotRequest{
					Name:            "vg-snap-1",
					SourceVolumeIds: []string{"vg-1"},
				},
			},
			true,
			codes.InvalidArgument,
		},
		{
			"empty clusterID in CreateVolumeGroupSnapshotRequest",
			args{
				context.Background(), &csi.CreateVolumeGroupSnapshotRequest{
					Name:            "vg-snap-1",
					SourceVolumeIds: []string{"vg-1"},
					Parameters:      map[string]string{"fsName": "value"},
				},
			},
			true,
			codes.InvalidArgument,
		},
		{
			"empty fsName in CreateVolumeGroupSnapshotRequest",
			args{
				context.Background(), &csi.CreateVolumeGroupSnapshotRequest{
					Name:            "vg-snap-1",
					SourceVolumeIds: []string{"vg-1"},
					Parameters:      map[string]string{"clusterID": "value"},
				},
			},
			true,
			codes.InvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := cs.validateCreateVolumeGroupSnapshotRequest(tt.args.ctx, tt.args.req)
			if tt.wantErr {
				c := status.Code(err)
				if c != tt.code {
					t.Errorf("ControllerServer.validateVolumeGroupSnapshotRequest() error = %v, want code %v", err, c)
				}
			}
		})
	}
}
