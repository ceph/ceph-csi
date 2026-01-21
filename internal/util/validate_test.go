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
		name     string
		volumeID string
		wantErr  bool
	}{
		// Valid IDs
		{"valid standard", "0001-0024-rook-ceph-pool-uuid", false},
		{"valid short", "0001-0024-pool-abc", false},
		{"valid long", "0001-0024-cluster-pool-0000-0000-0000-0001", false},
		{"valid very long", "0001-0024-a-very-long-list-of-characters-and-numbers-000-a-12", false},

		// Path traversal attempts
		{"traversal dots", "0001-0024/../../../tmp", true},
		{"traversal unix", "../../../etc/passwd", true},
		{"traversal windows", "..\\..\\windows", true},
		{"traversal embedded", "vol-id/../etc", true},

		// Path separator injection
		{"forward slash", "0001-0024/etc/passwd", true},
		{"backslash", "vol\\id", true},
		{"mixed separators", "vol/..\\etc", true},

		// Format violations
		{"missing prefix", "rook-ceph-pool", true},
		{"wrong prefix format", "001-024-pool", true},
		{"special chars", "0001-0024-pool$pwned", true},
		{"spaces", "0001-0024-pool pwned", true},
		{"null byte", "0001-0024-pool\x00etc", true},
		{"unicode", "0001-0024-pöōl", true},

		// Edge cases
		{"empty", "", true},
		{"only hyphens", "----", true},
		{"only numbers", "00010024", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateVolumeID(tt.volumeID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumeID(%q) error = %v, wantErr %v", tt.volumeID, err, tt.wantErr)
			}
		})
	}
}
