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

package types

import "testing"

func TestGetExportPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		volumeID   string
		exportName string
		want       string
	}{
		{
			name:     "falls back to the volumeID when no export name is set",
			volumeID: "0001-0024-fed5480a-f00d-4c66-bede-11112222",
			want:     "/0001-0024-fed5480a-f00d-4c66-bede-11112222",
		},
		{
			name:       "uses the friendly export name when set",
			volumeID:   "0001-0024-fed5480a-f00d-4c66-bede-11112222",
			exportName: "default/my-pvc",
			want:       "/default/my-pvc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nv := &NFSVolume{
				volumeID:   tt.volumeID,
				exportName: tt.exportName,
			}
			if got := nv.GetExportPath(); got != tt.want {
				t.Errorf("GetExportPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
