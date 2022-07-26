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

package nodeserver

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func Test_validateNodePublishVolumeRequest(t *testing.T) {
	t.Parallel()
	type args struct {
		req *csi.NodePublishVolumeRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "passing testcase",
			args: args{
				req: &csi.NodePublishVolumeRequest{
					VolumeId:         "123",
					TargetPath:       "/target",
					VolumeCapability: &csi.VolumeCapability{},
				},
			},
			wantErr: false,
		},
		{
			name: "missing VolumeId",
			args: args{
				req: &csi.NodePublishVolumeRequest{
					VolumeId:         "",
					TargetPath:       "/target",
					VolumeCapability: &csi.VolumeCapability{},
				},
			},
			wantErr: true,
		},
		{
			name: "missing TargetPath",
			args: args{
				req: &csi.NodePublishVolumeRequest{
					VolumeId:         "123",
					TargetPath:       "",
					VolumeCapability: &csi.VolumeCapability{},
				},
			},
			wantErr: true,
		},
		{
			name: "missing VolumeCapability",
			args: args{
				req: &csi.NodePublishVolumeRequest{
					VolumeId:         "123",
					TargetPath:       "/target",
					VolumeCapability: nil,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			t.Parallel()
			err := validateNodePublishVolumeRequest(currentTT.args.req)
			if (err != nil) != currentTT.wantErr {
				t.Errorf("validateNodePublishVoluemRequest() error = %v, wantErr %v", err, currentTT.wantErr)
			}
		})
	}
}

func Test_getSource(t *testing.T) {
	t.Parallel()
	type args struct {
		volContext map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "hostname as address",
			args: args{
				volContext: map[string]string{
					paramServer: "example.io",
					paramShare:  "/a",
				},
			},
			want:    "example.io:/a",
			wantErr: false,
		},
		{
			name: "ipv4 address",
			args: args{
				volContext: map[string]string{
					paramServer: "10.12.1.0",
					paramShare:  "/a",
				},
			},
			want:    "10.12.1.0:/a",
			wantErr: false,
		},
		{
			name: "ipv6 address",
			args: args{
				volContext: map[string]string{
					paramServer: "2001:0db8:3c4d:0015:0000:0000:1a2f:1a2b",
					paramShare:  "/a",
				},
			},
			want:    "[2001:0db8:3c4d:0015:0000:0000:1a2f:1a2b]:/a",
			wantErr: false,
		},
		{
			name: "missing server parameter",
			args: args{
				volContext: map[string]string{
					paramServer: "",
					paramShare:  "/a",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "missing share parameter",
			args: args{
				volContext: map[string]string{
					paramServer: "10.12.1.0",
					paramShare:  "",
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			t.Parallel()
			got, err := getSource(currentTT.args.volContext)
			if (err != nil) != currentTT.wantErr {
				t.Errorf("getSource() error = %v, wantErr %v", err, currentTT.wantErr)

				return
			}
			if got != currentTT.want {
				t.Errorf("getSource() = %v, want %v", got, currentTT.want)
			}
		})
	}
}
