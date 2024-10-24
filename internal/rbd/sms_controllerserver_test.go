/*
Copyright 2025 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rbd

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func Test_validateMetadataAllocatedReq(t *testing.T) {
	t.Parallel()
	type args struct {
		req *csi.GetMetadataAllocatedRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid request",
			args: args{
				req: &csi.GetMetadataAllocatedRequest{
					SnapshotId:     "snap-12345",
					StartingOffset: int64(0),
					MaxResults:     int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid request - missing snapshot ID",
			args: args{
				req: &csi.GetMetadataAllocatedRequest{
					SnapshotId:     "",
					StartingOffset: int64(0),
					MaxResults:     int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - negative starting offset",
			args: args{
				req: &csi.GetMetadataAllocatedRequest{
					SnapshotId:     "snap-12345",
					StartingOffset: int64(-1),
					MaxResults:     int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - negative max results",
			args: args{
				req: &csi.GetMetadataAllocatedRequest{
					SnapshotId:     "snap-12345",
					StartingOffset: int64(0),
					MaxResults:     int32(-100), // MaxResults should be greater than 0
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty secrets",
			args: args{
				req: &csi.GetMetadataAllocatedRequest{
					SnapshotId:     "snap-12345",
					StartingOffset: int64(0),
					MaxResults:     int32(100),
					Secrets:        map[string]string{},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateMetadataAllocatedReq(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMetadataAllocatedReq() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateMetadataDeltaReq(t *testing.T) {
	t.Parallel()
	type args struct {
		req *csi.GetMetadataDeltaRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid request",
			args: args{
				req: &csi.GetMetadataDeltaRequest{
					BaseSnapshotId:   "base-snap-12345",
					TargetSnapshotId: "target-snap-67890",
					StartingOffset:   int64(0),
					MaxResults:       int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid request - missing base snapshot ID",
			args: args{
				req: &csi.GetMetadataDeltaRequest{
					BaseSnapshotId:   "",
					TargetSnapshotId: "target-snap-67890",
					StartingOffset:   int64(0),
					MaxResults:       int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - missing target snapshot ID",
			args: args{
				req: &csi.GetMetadataDeltaRequest{
					BaseSnapshotId:   "base-snap-12345",
					TargetSnapshotId: "",
					StartingOffset:   int64(0),
					MaxResults:       int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - negative starting offset",
			args: args{
				req: &csi.GetMetadataDeltaRequest{
					BaseSnapshotId:   "base-snap-12345",
					TargetSnapshotId: "target-snap-67890",
					StartingOffset:   int64(-1),
					MaxResults:       int32(100),
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - negative max results",
			args: args{
				req: &csi.GetMetadataDeltaRequest{
					BaseSnapshotId:   "base-snap-12345",
					TargetSnapshotId: "target-snap-67890",
					StartingOffset:   int64(0),
					MaxResults:       int32(-100), // MaxResults should be greater than 0
					Secrets: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty secrets",
			args: args{
				req: &csi.GetMetadataDeltaRequest{
					BaseSnapshotId:   "base-snap-12345",
					TargetSnapshotId: "target-snap-67890",
					StartingOffset:   int64(0),
					MaxResults:       int32(100),
					Secrets:          map[string]string{},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateMetadataDeltaReq(tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("validateMetadataDeltaReq() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_normalizeMaxResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		requestedMaxResults int32
		want                int32
	}{
		{
			name:                "zero value returns default",
			requestedMaxResults: 0,
			want:                defaultMaxResults,
		},
		{
			name:                "value exceeding maximum gets capped",
			requestedMaxResults: defaultMaxResults + 100,
			want:                defaultMaxResults,
		},
		{
			name:                "valid value within limits returns as-is",
			requestedMaxResults: 50,
			want:                50,
		},
		{
			name:                "value equal to maximum returns maximum",
			requestedMaxResults: defaultMaxResults,
			want:                defaultMaxResults,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			got := normalizeMaxResults(ctx, tt.requestedMaxResults)
			if got != tt.want {
				t.Errorf("normalizeMaxResults() = %v, want %v", got, tt.want)
			}
		})
	}
}
