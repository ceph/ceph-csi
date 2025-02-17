/*
Copyright 2025 The Ceph-CSI Authors.

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
	"testing"
	"time"
)

func TestToCSISnapshot(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tests := []struct {
		name    string
		rs      *rbdSnapshot
		wantErr bool
	}{
		{
			name: "all attributes set",
			rs: &rbdSnapshot{
				rbdImage: rbdImage{
					VolID:     "0001-unique-snapshot-id",
					CreatedAt: &now,
				},
				SourceVolumeID: "0001-unique-volume-id",
			},
			wantErr: false,
		},
		{
			name: "missing volume-id",
			rs: &rbdSnapshot{
				rbdImage: rbdImage{
					VolID:     "",
					CreatedAt: &now,
				},
				SourceVolumeID: "0001-unique-volume-id",
			},
			wantErr: true,
		},
		{
			name: "missing source-volume-id",
			rs: &rbdSnapshot{
				rbdImage: rbdImage{
					VolID:     "0001-unique-snapshot-id",
					CreatedAt: &now,
				},
				SourceVolumeID: "",
			},
			wantErr: true,
		},
		{
			name: "missing creation-time",
			rs: &rbdSnapshot{
				rbdImage: rbdImage{
					VolID:     "0001-unique-snapshot-id",
					CreatedAt: nil,
				},
				SourceVolumeID: "0001-unique-volume-id",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.rs.ToCSI(context.TODO()); (err != nil) != tt.wantErr {
				t.Errorf("ToCSI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
