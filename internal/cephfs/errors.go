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

package cephfs

// ErrInvalidVolID is returned when a CSI passed VolumeID is not conformant to any known volume ID
// formats
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

// ErrNonStaticVolume is returned when a volume is detected as not being
// statically provisioned
type ErrNonStaticVolume struct {
	err error
}

// Error returns a user presentable string of the error.
func (e ErrNonStaticVolume) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrNonStaticVolume.
func (e ErrNonStaticVolume) Unwrap() error {
	return e.err
}

// ErrVolumeNotFound is returned when a subvolume is not found in CephFS
type ErrVolumeNotFound struct {
	err error
}

// Error returns a user presentable string of the error.
func (e ErrVolumeNotFound) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrVolumeNotFound.
func (e ErrVolumeNotFound) Unwrap() error {
	return e.err
}

// ErrCloneInProgress for cloned subvolume
type ErrCloneInProgress struct {
	err error
}

// Error returns a user presentable string of the error.
func (e ErrCloneInProgress) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of ErrCloneInProgress.
func (e ErrCloneInProgress) Unwrap() error {
	return e.err
}

// InvalidCommand represents invalid command
type InvalidCommand struct {
	err error
}

// Error returns a user presentable string of the error.
func (e InvalidCommand) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error of InvalidCommand.
func (e InvalidCommand) Unwrap() error {
	return e.err
}
