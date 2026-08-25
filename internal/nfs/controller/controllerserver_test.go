/*
Copyright 2026 The Ceph-CSI Authors.

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

package controller

import (
	"testing"

	nfs "github.com/ceph/ceph-csi/internal/nfs/types"
)

func Test_friendlyExportName(t *testing.T) {
	t.Parallel()
	type args struct {
		nfsParams map[string]string
		csiParams map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "friendlyExportNames unset",
			args: args{
				nfsParams: map[string]string{},
				csiParams: map[string]string{
					"csi.storage.k8s.io/pvc/namespace": "default",
					"csi.storage.k8s.io/pvc/name":      "my-pvc",
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "friendlyExportNames false",
			args: args{
				nfsParams: map[string]string{
					nfs.ParameterFriendlyExportNames: "false",
				},
				csiParams: map[string]string{
					"csi.storage.k8s.io/pvc/namespace": "default",
					"csi.storage.k8s.io/pvc/name":      "my-pvc",
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "friendlyExportNames true with metadata present",
			args: args{
				nfsParams: map[string]string{
					nfs.ParameterFriendlyExportNames: "true",
				},
				csiParams: map[string]string{
					"csi.storage.k8s.io/pvc/namespace": "default",
					"csi.storage.k8s.io/pvc/name":      "my-pvc",
				},
			},
			want:    "default/my-pvc",
			wantErr: false,
		},
		{
			name: "friendlyExportNames true without extra-create-metadata",
			args: args{
				nfsParams: map[string]string{
					nfs.ParameterFriendlyExportNames: "true",
				},
				csiParams: map[string]string{},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "friendlyExportNames true with only the namespace present",
			args: args{
				nfsParams: map[string]string{
					nfs.ParameterFriendlyExportNames: "true",
				},
				csiParams: map[string]string{
					"csi.storage.k8s.io/pvc/namespace": "default",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "friendlyExportNames not a boolean",
			args: args{
				nfsParams: map[string]string{
					nfs.ParameterFriendlyExportNames: "yes-please",
				},
				csiParams: map[string]string{
					"csi.storage.k8s.io/pvc/namespace": "default",
					"csi.storage.k8s.io/pvc/name":      "my-pvc",
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := friendlyExportName(tt.args.nfsParams, tt.args.csiParams)
			if (err != nil) != tt.wantErr {
				t.Errorf("friendlyExportName() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if got != tt.want {
				t.Errorf("friendlyExportName() = %v, want %v", got, tt.want)
			}
		})
	}
}
