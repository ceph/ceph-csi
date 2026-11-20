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

import "strings"

const (
	// Ext4 is the type of the ext4 filesystem.
	Ext4 = "ext4"
	// XFS is the type of the XFS filesystem.
	XFS = "xfs"

	// ext4ErrorsOption is the prefix of the ext4 mount option that selects
	// the behavior when the filesystem detects an error.
	ext4ErrorsOption = "errors="
)

// DefaultMountOptions returns the mount options that are always used for
// filesystems of type fsType. flags are the mount options already requested
// for the volume (for example through the mountOptions of the StorageClass).
// An entry in flags may hold several options separated by commas (for example
// "discard,errors=continue").
//
// For ext4, errors=remount-ro is returned, so that the filesystem turns
// read-only when ext4 detects an error (for example a directory block that
// fails its checksum), instead of letting the workload continue on a damaged
// filesystem with the kernel default of errors=continue. Nothing is returned
// when flags already contain an errors= option, so that the requested option
// takes precedence over the default.
//
// For XFS, nouuid is always returned, so that a filesystem with the same UUID
// as an already mounted one (for example from a snapshot or clone) can be
// mounted.
//
// For other filesystems, no options are returned.
func DefaultMountOptions(fsType string, flags []string) []string {
	switch fsType {
	case Ext4:
		for _, flag := range flags {
			for opt := range strings.SplitSeq(flag, ",") {
				if strings.HasPrefix(strings.TrimSpace(opt), ext4ErrorsOption) {
					return nil
				}
			}
		}

		return []string{ext4ErrorsOption + "remount-ro"}
	case XFS:
		return []string{"nouuid"}
	}

	return nil
}
