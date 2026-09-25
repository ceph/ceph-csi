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

package csicommon

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
)

func TestDefaultMountOptions(t *testing.T) {
	t.Parallel()

	mountCap := func(flags ...string) *csi.VolumeCapability {
		return &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{MountFlags: flags},
			},
		}
	}

	tests := []struct {
		name   string
		fsType string
		volCap *csi.VolumeCapability
		want   []string
	}{
		{
			name:   "ext4 without mount flags",
			fsType: "ext4",
			volCap: mountCap(),
			want:   []string{"errors=remount-ro"},
		},
		{
			name:   "ext4 with unrelated mount flags",
			fsType: "ext4",
			volCap: mountCap("noatime", "discard"),
			want:   []string{"errors=remount-ro"},
		},
		{
			name:   "ext4 with errors=continue set by the user",
			fsType: "ext4",
			volCap: mountCap("noatime", "errors=continue"),
			want:   nil,
		},
		{
			name:   "ext4 with errors=panic set by the user",
			fsType: "ext4",
			volCap: mountCap("errors=panic"),
			want:   nil,
		},
		{
			name:   "ext4 with a nil volume capability",
			fsType: "ext4",
			volCap: nil,
			want:   []string{"errors=remount-ro"},
		},
		{
			name:   "ext4 with a block volume capability",
			fsType: "ext4",
			volCap: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
			},
			want: []string{"errors=remount-ro"},
		},
		{
			name:   "xfs",
			fsType: "xfs",
			volCap: mountCap("errors=continue"),
			want:   []string{"nouuid"},
		},
		{
			name:   "unknown filesystem",
			fsType: "btrfs",
			volCap: mountCap(),
			want:   nil,
		},
		{
			name:   "no filesystem",
			fsType: "",
			volCap: mountCap(),
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, DefaultMountOptions(tt.fsType, tt.volCap))
		})
	}
}
