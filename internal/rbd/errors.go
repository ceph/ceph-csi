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

package rbd

import "errors"

var (
	// ErrImageNotFound is returned when image name is not found in the cluster on the given pool and/or namespace.
	ErrImageNotFound = errors.New("image not found")
	// ErrSnapNotFound is returned when snap name passed is not found in the list of snapshots for the
	// given image.
	ErrSnapNotFound = errors.New("snapshot not found")
	// ErrVolNameConflict is generated when a requested CSI volume name already exists on RBD but with
	// different properties, and hence is in conflict with the passed in CSI volume name.
	ErrVolNameConflict = errors.New("volume name conflict")
	// ErrInvalidVolID is returned when a CSI passed VolumeID does not conform to any known volume ID
	// formats.
	ErrInvalidVolID = errors.New("invalid VolumeID")
	// ErrMissingStash is returned when the image metadata stash file is not found.
	ErrMissingStash = errors.New("missing stash")
	// ErrFlattenInProgress is returned when flatten is in progress for an image.
	ErrFlattenInProgress = errors.New("flatten in progress")
	// ErrMissingMonitorsInVolID is returned when monitor information is missing in migration volID.
	ErrMissingMonitorsInVolID = errors.New("monitor information can not be empty in volID")
	// ErrMissingPoolNameInVolID is returned when pool information is missing in migration volID.
	ErrMissingPoolNameInVolID = errors.New("pool information can not be empty in volID")
	// ErrMissingImageNameInVolID is returned when image name information is missing in migration volID.
	ErrMissingImageNameInVolID = errors.New("rbd image name information can not be empty in volID")
	// ErrDecodeClusterIDFromMonsInVolID is returned when mons hash decoding on migration volID.
	ErrDecodeClusterIDFromMonsInVolID = errors.New("failed to get clusterID from monitors hash in volID")
	// ErrLastSyncTimeNotFound is returned when last sync time is not found for
	// the image.
	ErrLastSyncTimeNotFound = errors.New("last sync time not found")
	// ErrFailedPrecondition is returned when operation is rejected because the system is not in a state
	// required for the operation's execution.
	ErrFailedPrecondition = errors.New("system is not in a state required for the operation's execution")
	// ErrUnavailable is returned when the image needs to be recreated
	// locally and may be corrected by retrying with a backoff.
	ErrUnavailable = errors.New("image needs to be recreated")
	// ErrAborted is returned when the operation is aborted.
	ErrAborted = errors.New("operation got aborted")
	// ErrInvalidArgument is returned when the client specified an invalid argument.
	ErrInvalidArgument = errors.New("invalid arguments provided")
	// ErrFetchingLocalState is returned when the operation to fetch local state fails.
	ErrFetchingLocalState = errors.New("failed to get local state")
	// ErrDisableImageMirroringFailed is returned when the operation to disable image mirroring fails.
	ErrDisableImageMirroringFailed = errors.New("failed to disable image mirroring")
	// ErrFetchingMirroringInfo is returned when the operation to fetch mirroring info of image fails.
	ErrFetchingMirroringInfo = errors.New("failed to get mirroring info of image")
	// ErrResyncImageFailed is returned when the operation to resync the image fails.
	ErrResyncImageFailed = errors.New("failed to resync image")
)
