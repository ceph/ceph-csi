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
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	ext4FsType = "ext4"
	xfsFsType  = "xfs"

	// ext4ErrorsOption is the prefix of the ext4 mount option that selects
	// the behavior when the filesystem detects an error.
	ext4ErrorsOption = "errors="
)

// DefaultMountOptions returns the mount options that are always used for
// filesystems of type fsType. Options set on the volume capability (through
// the mountOptions of the StorageClass) take precedence over the defaults, a
// default is not returned if the volume capability already sets that option.
//
// For ext4, errors=remount-ro is used instead of the kernel default of
// errors=continue, so that the filesystem turns read-only when ext4 detects an
// error (for example a directory block that fails its checksum), instead of
// letting the workload continue on a damaged filesystem.
func DefaultMountOptions(fsType string, volCap *csi.VolumeCapability) []string {
	switch fsType {
	case ext4FsType:
		for _, flag := range volCap.GetMount().GetMountFlags() {
			if strings.HasPrefix(flag, ext4ErrorsOption) {
				return nil
			}
		}

		return []string{ext4ErrorsOption + "remount-ro"}
	case xfsFsType:
		return []string{"nouuid"}
	}

	return nil
}
