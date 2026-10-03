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

package filesystems

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMountOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fsType string
		flags  []string
		want   []string
	}{
		{
			name:   "ext4 without mount flags",
			fsType: "ext4",
			flags:  nil,
			want:   []string{"errors=remount-ro"},
		},
		{
			name:   "ext4 with unrelated mount flags",
			fsType: "ext4",
			flags:  []string{"noatime", "discard"},
			want:   []string{"errors=remount-ro"},
		},
		{
			name:   "ext4 with errors=continue set by the user",
			fsType: "ext4",
			flags:  []string{"noatime", "errors=continue"},
			want:   nil,
		},
		{
			name:   "ext4 with errors=panic set by the user",
			fsType: "ext4",
			flags:  []string{"errors=panic"},
			want:   nil,
		},
		{
			name:   "xfs",
			fsType: "xfs",
			flags:  []string{"errors=continue"},
			want:   []string{"nouuid"},
		},
		{
			name:   "unknown filesystem",
			fsType: "btrfs",
			flags:  nil,
			want:   nil,
		},
		{
			name:   "no filesystem",
			fsType: "",
			flags:  nil,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, DefaultMountOptions(tt.fsType, tt.flags))
		})
	}
}
