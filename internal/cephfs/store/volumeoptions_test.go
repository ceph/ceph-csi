/*
Copyright 2023 The Ceph-CSI Authors.

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

package store

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func TestIsVolumeCreateRO(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		caps []*csi.VolumeCapability
		isRO bool
	}{
		{
			name: "valid access mode",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
				},
			},
			isRO: true,
		},
		{
			name: "Invalid access mode",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
			isRO: false,
		},
		{
			name: "valid access mode",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
					},
				},
			},
			isRO: true,
		},
		{
			name: "Invalid access mode",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			isRO: false,
		},
		{
			name: "Invalid access mode",
			caps: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
					},
				},
			},
			isRO: false,
		},
	}
	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.name, func(t *testing.T) {
			t.Parallel()
			wantErr := IsVolumeCreateRO(newtt.caps)
			if wantErr != newtt.isRO {
				t.Errorf("isVolumeCreateRO() wantErr = %v, isRO %v", wantErr, newtt.isRO)
			}
		})
	}
}

func TestIsShallowVolumeSupported(t *testing.T) {
	t.Parallel()
	type args struct {
		req *csi.CreateVolumeRequest
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Invalid request",
			args: args{
				req: &csi.CreateVolumeRequest{
					Name: "",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Block{},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
							},
						},
					},

					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Volume{
							Volume: &csi.VolumeContentSource_VolumeSource{
								VolumeId: "vol",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Invalid request",
			args: args{
				req: &csi.CreateVolumeRequest{
					Name: "",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Block{},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
							},
						},
					},

					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Volume{
							Volume: &csi.VolumeContentSource_VolumeSource{
								VolumeId: "vol",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Invalid request",
			args: args{
				req: &csi.CreateVolumeRequest{
					Name: "",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Block{},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
							},
						},
					},

					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: "snap",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "valid request",
			args: args{
				req: &csi.CreateVolumeRequest{
					Name: "",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Block{},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
							},
						},
					},

					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: "snap",
							},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.name, func(t *testing.T) {
			t.Log(newtt.args.req.GetVolumeContentSource().GetSnapshot())
			t.Log(IsVolumeCreateRO(newtt.args.req.GetVolumeCapabilities()))
			t.Parallel()
			if got := IsShallowVolumeSupported(newtt.args.req); got != newtt.want {
				t.Errorf("IsShallowVolumeSupported() = %v, want %v", got, newtt.want)
			}
		})
	}
}
