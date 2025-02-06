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

package util

import (
	"errors"

	"github.com/ceph/go-ceph/rados"
)

var (
	// ErrImageNotFound is returned when image name is not found in the cluster on the given pool and/or namespace.
	ErrImageNotFound = errors.New("image not found")
	// ErrKeyNotFound is returned when requested key in omap is not found.
	ErrKeyNotFound = errors.New("key not found")
	// ErrObjectExists is returned when named omap is already present in rados.
	ErrObjectExists = errors.New("object exists")
	// ErrObjectNotFound is returned when named omap is not found in rados.
	ErrObjectNotFound = errors.New("object not found")
	// ErrSnapNameConflict is generated when a requested CSI snap name already exists on RBD but with
	// different properties, and hence is in conflict with the passed in CSI volume name.
	ErrSnapNameConflict = errors.New("snapshot name conflict")
	// ErrPoolNotFound is returned when pool is not found.
	ErrPoolNotFound = errors.New("pool not found")
	// ErrClusterIDNotSet is returned when cluster id is not set.
	ErrClusterIDNotSet = errors.New("clusterID must be set")
	// ErrMissingConfigForMonitor is returned when clusterID is not found for the mon.
	ErrMissingConfigForMonitor = errors.New("missing configuration of cluster ID for monitor")
)

// ShouldRetryVolumeGeneration determines whether the process of finding or generating
// volumes should continue based on the type of error encountered.
//
// It checks if the given error matches any of the following known errors:
//   - util.ErrKeyNotFound: The key required to locate the volume is missing in Rados omap.
//   - util.ErrPoolNotFound: The rbd pool where the volume/omap is expected doesn't exist.
//   - ErrImageNotFound: The image doesn't exist in the rbd pool.
//   - rados.ErrPermissionDenied: Permissions to access the pool is denied.
//
// If any of these errors are encountered, the function returns `true`, indicating
// that the volume search should continue because of known error. Otherwise, it
// returns `false`, meaning the search should stop.
//
// This helper function is used in scenarios where multiple attempts may be made
// to retrieve or generate volume information, and we want to gracefully handle
// specific failure cases while retrying for others.
func ShouldRetryVolumeGeneration(err error) bool {
	if err == nil {
		return false // No error, do not retry
	}
	// Continue searching for specific known errors
	return (errors.Is(err, ErrKeyNotFound) ||
		errors.Is(err, ErrPoolNotFound) ||
		errors.Is(err, ErrImageNotFound) ||
		errors.Is(err, rados.ErrPermissionDenied))
}
