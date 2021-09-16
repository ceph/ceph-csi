/*
Copyright 2019 The Ceph-CSI Authors.

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

package errors

import (
	coreError "errors"
)

// Error strings for comparison with CLI errors.
const (
	// VolumeNotEmpty is returned when the volume is not empty.
	VolumeNotEmpty = "Directory not empty"
)

var (
	// ErrCloneInProgress is returned when snapshot clone state is `in progress`.
	ErrCloneInProgress = coreError.New("clone from snapshot is already in progress")

	// ErrClonePending is returned when snapshot clone state is `pending`.
	ErrClonePending = coreError.New("clone from snapshot is pending")

	// ErrInvalidClone is returned when the clone state is invalid.
	ErrInvalidClone = coreError.New("invalid clone state")

	// ErrCloneFailed is returned when the clone state is failed.
	ErrCloneFailed = coreError.New("clone from snapshot failed")

	// ErrInvalidVolID is returned when a CSI passed VolumeID is not conformant to any known volume ID
	// formats.
	ErrInvalidVolID = coreError.New("invalid VolumeID")
	// ErrNonStaticVolume is returned when a volume is detected as not being
	// statically provisioned.
	ErrNonStaticVolume = coreError.New("volume not static")

	// ErrSnapProtectionExist is returned when the snapshot is already protected.
	ErrSnapProtectionExist = coreError.New("snapshot  protection already exists")

	// ErrSnapNotFound is returned when snap name passed is not found in the list
	// of snapshots for the given image.
	ErrSnapNotFound = coreError.New("snapshot not found")

	// ErrVolumeNotFound is returned when a subvolume is not found in CephFS.
	ErrVolumeNotFound = coreError.New("volume not found")

	// ErrInvalidCommand is returned when a command is not known to the cluster.
	ErrInvalidCommand = coreError.New("invalid command")

	// ErrVolumeHasSnapshots is returned when a subvolume has snapshots.
	ErrVolumeHasSnapshots = coreError.New("volume has snapshots")
)

// IsCloneRetryError returns true if the clone error is pending,in-progress
// error.
func IsCloneRetryError(err error) bool {
	return coreError.Is(err, ErrCloneInProgress) || coreError.Is(err, ErrClonePending)
}
