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

package version

import (
	"encoding/binary"

	"github.com/ceph/ceph-csi/internal/util/reftracker/errors"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
)

// reftracker objects are versioned, should the object layout need to change.
// Version is stored in its underlying RADOS object xattr as uint32.

const (
	// Name of the xattr entry in the RADOS object.
	XattrName = "csi.ceph.com/rt-version"

	// SizeBytes is the size of version in bytes.
	SizeBytes = 4
)

func ToBytes(v uint32) []byte {
	bs := make([]byte, SizeBytes)
	binary.BigEndian.PutUint32(bs, v)

	return bs
}

func FromBytes(bs []byte) (uint32, error) {
	if len(bs) != SizeBytes {
		return 0, errors.UnexpectedReadSize(SizeBytes, len(bs))
	}

	return binary.BigEndian.Uint32(bs), nil
}

func Read(ioctx radoswrapper.IOContextW, rtName string) (uint32, error) {
	verBytes := make([]byte, SizeBytes)
	readSize, err := ioctx.GetXattr(rtName, XattrName, verBytes)
	if err != nil {
		return 0, err
	}

	if readSize != SizeBytes {
		return 0, errors.UnexpectedReadSize(SizeBytes, readSize)
	}

	return FromBytes(verBytes)
}
