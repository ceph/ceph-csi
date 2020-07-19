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

// ErrImageNotFound is returned when image name is not found in the cluster on the given pool.
type ErrImageNotFound struct {
	imageName string
	err       error
}

// Error returns a user presentable string of the error.
func (e ErrImageNotFound) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrImageNotFound.
func (e ErrImageNotFound) Unwrap() error {
	return e.err
}

// ErrSnapNotFound is returned when snap name passed is not found in the list of snapshots for the
// given image.
type ErrSnapNotFound struct {
	snapName string
	err      error
}

// Error returns a user presentable string of the error.
func (e ErrSnapNotFound) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrSnapNotFound.
func (e ErrSnapNotFound) Unwrap() error {
	return e.err
}

// ErrVolNameConflict is generated when a requested CSI volume name already exists on RBD but with
// different properties, and hence is in conflict with the passed in CSI volume name.
type ErrVolNameConflict struct {
	requestName string
	err         error
}

// Error returns a user presentable string of the error.
func (e ErrVolNameConflict) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrVolNameConflict.
func (e ErrVolNameConflict) Unwrap() error {
	return e.err
}

// ErrInvalidVolID is returned when a CSI passed VolumeID does not conform to any known volume ID
// formats.
type ErrInvalidVolID struct {
	err error
}

// Error returns a user presentable string of the error.
func (e ErrInvalidVolID) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrInvalidVolID.
func (e ErrInvalidVolID) Unwrap() error {
	return e.err
}

// ErrMissingStash is returned when the image metadata stash file is not found.
type ErrMissingStash struct {
	err error
}

// Error returns a user presentable string of the error.
func (e ErrMissingStash) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrMissingStash.
func (e ErrMissingStash) Unwrap() error {
	return e.err
}

// ErrFlattenInProgress is returned when flatten is inprogess for an image.
type ErrFlattenInProgress struct {
	err error
}

// Error returns a user presentable string of the error.
func (e ErrFlattenInProgress) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrFlattenInProgress.
func (e ErrFlattenInProgress) Unwrap() error {
	return e.err
}
