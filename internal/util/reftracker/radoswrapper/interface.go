/*
Copyright 2022 The Ceph-CSI Authors.

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

package radoswrapper

import (
	"github.com/ceph/go-ceph/rados"
)

// These interfaces are just wrappers around some of go-ceph's rados pkg
// structures and functions. They have two implementations: the "real" one
// (that simply uses go-ceph), and a fake one, used in unit tests.

// IOContextW is a wrapper around rados.IOContext.
type IOContextW interface {
	// GetLastVersion will return the version number of the last object read or
	// written to.
	GetLastVersion() (uint64, error)

	// GetXattr gets an xattr with key `name`, it returns the length of
	// the key read or an error if not successful
	GetXattr(oid string, key string, data []byte) (int, error)

	// CreateWriteOp returns a newly constructed write operation.
	CreateWriteOp() WriteOpW

	// CreateReadOp returns a newly constructed read operation.
	CreateReadOp() ReadOpW
}

// WriteOpW is a wrapper around rados.WriteOp interface.
type WriteOpW interface {
	// Create a rados object.
	Create(exclusive rados.CreateOption)

	// Remove object.
	Remove()

	// SetXattr sets an xattr.
	SetXattr(name string, value []byte)

	// WriteFull writes a given byte slice as the whole object,
	// atomically replacing it.
	WriteFull(b []byte)

	// SetOmap appends the map `pairs` to the omap `oid`.
	SetOmap(pairs map[string][]byte)

	// RmOmapKeys removes the specified `keys` from the omap `oid`.
	RmOmapKeys(keys []string)

	// AssertVersion ensures that the object exists and that its internal version
	// number is equal to "ver" before writing. "ver" should be a version number
	// previously obtained with IOContext.GetLastVersion().
	AssertVersion(ver uint64)

	// Operate will perform the operation(s).
	Operate(oid string) error

	// Release the resources associated with this write operation.
	Release()
}

// ReadOpW is a wrapper around rados.ReadOp.
type ReadOpW interface {
	// Read bytes from offset into buffer.
	// len(buffer) is the maximum number of bytes read from the object.
	// buffer[:ReadOpReadStep.BytesRead] then contains object data.
	Read(offset uint64, buffer []byte) *rados.ReadOpReadStep

	// GetOmapValuesByKeys starts iterating over specific key/value pairs.
	GetOmapValuesByKeys(keys []string) ReadOpOmapGetValsByKeysStepW

	// AssertVersion ensures that the object exists and that its internal version
	// number is equal to "ver" before reading. "ver" should be a version number
	// previously obtained with IOContext.GetLastVersion().
	AssertVersion(ver uint64)

	// Operate will perform the operation(s).
	Operate(oid string) error

	// Release the resources associated with this read operation.
	Release()
}

// ReadOpOmapGetValsByKeysStepW is a wrapper around rados.ReadOpOmapGetValsByKeysStep.
type ReadOpOmapGetValsByKeysStepW interface {
	// Next gets the next omap key/value pair referenced by
	// ReadOpOmapGetValsByKeysStep's internal iterator.
	// If there are no more elements to retrieve, (nil, nil) is returned.
	// May be called only after Operate() finished.
	Next() (*rados.OmapKeyValue, error)
}
