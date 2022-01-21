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
	"fmt"
)

var (
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

type pairError struct {
	first  error
	second error
}

func (e pairError) Error() string {
	return fmt.Sprintf("%v: %v", e.first, e.second)
}

// Is checks if target error is wrapped in the first error.
func (e pairError) Is(target error) bool {
	return errors.Is(e.first, target)
}

// Unwrap returns the second error.
func (e pairError) Unwrap() error {
	return e.second
}

// JoinErrors combines two errors. Of the returned error, Is() follows the first
// branch, Unwrap() follows the second branch.
func JoinErrors(e1, e2 error) error {
	return pairError{e1, e2}
}
