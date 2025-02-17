/*
Copyright 2022 The Ceph-CSI Authors.

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
)

func TestValidateStriping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters map[string]string
		wantErr    bool
	}{
		{
			name: "when stripeUnit is not specified",
			parameters: map[string]string{
				"stripeUnit":  "",
				"stripeCount": "10",
				"objectSize":  "2",
			},
			wantErr: true,
		},
		{
			name: "when stripeCount is not specified",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "",
				"objectSize":  "2",
			},
			wantErr: true,
		},
		{
			name: "when objectSize is not power of 2",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "8",
				"objectSize":  "3",
			},
			wantErr: true,
		},
		{
			name: "when objectSize is 0",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "8",
				"objectSize":  "0",
			},
			wantErr: true,
		},
		{
			name: "when valid stripe parameters are specified",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "8",
				"objectSize":  "131072",
			},
			wantErr: false,
		},
		{
			name:       "when no stripe parameters are specified",
			parameters: map[string]string{},
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateStriping(tt.parameters); (err != nil) != tt.wantErr {
				t.Errorf("validateStriping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToCSIVolume(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rv      *rbdVolume
		wantErr bool
	}{
		{
			name: "all attributes set",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "ecpool",
					JournalPool:  "replicapool",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: false,
		},
		{
			name: "missing volume-id",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "",
					Pool:         "ecpool",
					JournalPool:  "replicapool",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: true,
		},
		{
			name: "missing pool",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "",
					JournalPool:  "replicapool",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: true,
		},
		{
			name: "missing journal-pool",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "ecpool",
					JournalPool:  "",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: true,
		},
		{
			name: "missing image-name",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "ecpool",
					JournalPool:  "",
					RbdImageName: "",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.rv.ToCSI(context.TODO()); (err != nil) != tt.wantErr {
				t.Errorf("ToCSI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
