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

package util

import (
	"testing"
)

func TestValidateVolumeID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		volumeID   string
		skipFormat bool
		wantErr    bool
	}{
		// Dynamic volumes, must adhere to format.
		{"valid standard", "0001-0024-rook-ceph-pool-uuid", false, false},
		{"valid short", "0001-0024-pool-abc", false, false},
		{"valid long", "0001-0024-cluster-pool-0000-0000-0000-0001", false, false},
		{"valid very long", "0001-000b-clusterID-1-0000000000000001-c156bd07-e430-435f-b175-56c61a2d9297", false, false},
		{"invalid very long", "00fg-01bg-clusterID-1-0000000000000001-c156bd07-e430-435f-b175-56c61a2d9297", false, true},

		// Static Volumes, skip enforcing format.
		{"valid static", "this-is-a-static-volume", true, false},
		{"invalid static with path traversal", "this-is-a/../static-volume", true, true},
		{"invalid static with path separator", "this-is-a\\static-volume", true, true},

		// Path traversal attempts.
		{"traversal dots", "0001-0024/../../../tmp", false, true},
		{"traversal unix", "../../../etc/passwd", false, true},
		{"traversal windows", "..\\..\\windows", false, true},
		{"traversal embedded", "vol-id/../etc", false, true},

		// Path separator injection.
		{"forward slash", "0001-0024/etc/passwd", false, true},
		{"backslash", "vol\\id", false, true},
		{"mixed separators", "vol/..\\etc", false, true},

		// Format violations.
		{"missing prefix", "rook-ceph-pool", false, true},
		{"wrong prefix format", "001-024-pool", false, true},
		{"special chars", "0001-0024-pool$pwned", false, true},
		{"spaces", "0001-0024-pool pwned", false, true},
		{"null byte", "0001-0024-pool\x00etc", false, true},
		{"unicode", "0001-0024-pöōl", false, true},

		// Edge cases.
		{"empty", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateVolumeID(tt.volumeID, tt.skipFormat)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumeID(%q) error = %v, wantErr %v", tt.volumeID, err, tt.wantErr)
			}
		})
	}
}
