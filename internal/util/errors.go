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

// ErrKeyNotFound is returned when requested key in omap is not found
type ErrKeyNotFound struct {
	keyName string
	err     error
}

// NewErrKeyNotFound returns a new ErrKeyNotFound error.
func NewErrKeyNotFound(keyName string, err error) ErrKeyNotFound {
	return ErrKeyNotFound{keyName, err}
}

// Error returns the error string for ErrKeyNotFound.
func (e ErrKeyNotFound) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error.
func (e ErrKeyNotFound) Unwrap() error {
	return e.err
}

// ErrObjectExists is returned when named omap is already present in rados
type ErrObjectExists struct {
	objectName string
	err        error
}

// Error returns the error string for ErrObjectExists.
func (e ErrObjectExists) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error.
func (e ErrObjectExists) Unwrap() error {
	return e.err
}

// ErrObjectNotFound is returned when named omap is not found in rados
type ErrObjectNotFound struct {
	oMapName string
	err      error
}

// Error returns the error string for ErrObjectNotFound.
func (e ErrObjectNotFound) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error.
func (e ErrObjectNotFound) Unwrap() error {
	return e.err
}

// ErrSnapNameConflict is generated when a requested CSI snap name already exists on RBD but with
// different properties, and hence is in conflict with the passed in CSI volume name
type ErrSnapNameConflict struct {
	requestName string
	err         error
}

// Error returns the error string for ErrSnapNameConflict.
func (e ErrSnapNameConflict) Error() string {
	return e.err.Error()
}

// Unwrap returns the encapsulated error.
func (e ErrSnapNameConflict) Unwrap() error {
	return e.err
}

// NewErrSnapNameConflict returns a ErrSnapNameConflict error when CSI snap name already exists.
func NewErrSnapNameConflict(name string, err error) ErrSnapNameConflict {
	return ErrSnapNameConflict{name, err}
}

// ErrPoolNotFound is returned when pool is not found
type ErrPoolNotFound struct {
	Pool string
	Err  error
}

// Error returns the error string for ErrPoolNotFound.
func (e ErrPoolNotFound) Error() string {
	return e.Err.Error()
}

// Unwrap returns the encapsulated error.
func (e ErrPoolNotFound) Unwrap() error {
	return e.Err
}

// NewErrPoolNotFound returns a new ErrPoolNotFound error.
func NewErrPoolNotFound(pool string, err error) ErrPoolNotFound {
	return ErrPoolNotFound{pool, err}
}

// ErrSnapNotFound represent snapshot not found
type ErrSnapNotFound struct {
	SnapName string
	Err      error
}

// Error returns a user presentable string of the error.
func (e ErrSnapNotFound) Error() string {
	return e.Err.Error()
}

// Unwrap returns the encapsulated error of ErrSnapNotFound.
func (e ErrSnapNotFound) Unwrap() error {
	return e.Err
}
