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

func (e ErrKeyNotFound) Error() string {
	return e.err.Error()
}

// ErrObjectExists is returned when named omap is already present in rados
type ErrObjectExists struct {
	objectName string
	err        error
}

func (e ErrObjectExists) Error() string {
	return e.err.Error()
}

// ErrObjectNotFound is returned when named omap is not found in rados
type ErrObjectNotFound struct {
	oMapName string
	err      error
}

func (e ErrObjectNotFound) Error() string {
	return e.err.Error()
}

// ErrSnapNameConflict is generated when a requested CSI snap name already exists on RBD but with
// different properties, and hence is in conflict with the passed in CSI volume name
type ErrSnapNameConflict struct {
	requestName string
	err         error
}

func (e ErrSnapNameConflict) Error() string {
	return e.err.Error()
}

// ErrPoolNotFound is returned when pool is not found
type ErrPoolNotFound struct {
	Pool string
	Err  error
}

func (e ErrPoolNotFound) Error() string {
	return e.Err.Error()
}
