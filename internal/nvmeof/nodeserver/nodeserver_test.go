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

package nodeserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	mount "k8s.io/mount-utils"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
)

// TestMountVolumeToStagePathExt4Options verifies that the ext4 mount options
// that are passed to the mounter when staging a volume contain the default of
// errors=remount-ro, unless the volume capability sets an errors= option.
func TestMountVolumeToStagePathExt4Options(t *testing.T) {
	t.Parallel()

	// SafeFormatAndMount runs blkid and mkfs.ext4 on the (image) file
	for _, tool := range []string{"blkid", "mkfs.ext4"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available: %v", tool, err)
		}
	}

	tests := []struct {
		name       string
		fsType     string
		mountFlags []string
		want       []string
		notWant    []string
	}{
		{
			name:   "default",
			fsType: "ext4",
			want:   []string{"errors=remount-ro", "_netdev"},
		},
		{
			name:   "no fsType defaults to ext4",
			fsType: "",
			want:   []string{"errors=remount-ro", "_netdev"},
		},
		{
			name:       "errors=continue set in the StorageClass",
			fsType:     "ext4",
			mountFlags: []string{"errors=continue"},
			want:       []string{"errors=continue", "_netdev"},
			notWant:    []string{"errors=remount-ro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			devicePath := filepath.Join(dir, "image")
			stagingPath := filepath.Join(dir, "staging")

			img, err := os.Create(devicePath)
			require.NoError(t, err)
			require.NoError(t, img.Truncate(16<<20))
			require.NoError(t, img.Close())
			require.NoError(t, os.Mkdir(stagingPath, 0o750))

			mounter := mount.NewFakeMounter(nil)
			ns := &NodeServer{
				DefaultNodeServer: csicommon.DefaultNodeServer{Mounter: mounter},
			}
			req := &csi.NodeStageVolumeRequest{
				VolumeId:          "vol-id",
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType:     tt.fsType,
							MountFlags: tt.mountFlags,
						},
					},
				},
			}

			err = ns.mountVolumeToStagePath(context.Background(), req, stagingPath, devicePath)
			require.NoError(t, err)

			mounts, err := mounter.List()
			require.NoError(t, err)
			require.Len(t, mounts, 1)
			require.Equal(t, devicePath, mounts[0].Device)
			require.Equal(t, "ext4", mounts[0].Type)
			for _, opt := range tt.want {
				require.Contains(t, mounts[0].Opts, opt)
			}
			for _, opt := range tt.notWant {
				require.NotContains(t, mounts[0].Opts, opt)
			}
		})
	}
}
