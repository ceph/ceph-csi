/*
Copyright 2026 The Ceph-CSI Authors.

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

import (
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/ceph/go-ceph/rados"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
)

const (
	rbdHeaderPrefix = "rbd_header."
	clsRBD          = "rbd"
	clsMetadataSet  = "metadata_set"
	clsMetadataRm   = "metadata_remove"
)

// appendClsString appends a ceph encoded string (u32 little endian length
// followed by the bytes) to buf.
func appendClsString(buf []byte, s string) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(s)))

	return append(buf, s...)
}

// encodeClsMetadataSet encodes pairs as a map<string,bufferlist>, the input of
// the cls_rbd metadata_set method. Keys are sorted for a stable encoding.
func encodeClsMetadataSet(pairs map[string]string) []byte {
	buf := binary.LittleEndian.AppendUint32(nil, uint32(len(pairs)))
	for _, k := range slices.Sorted(maps.Keys(pairs)) {
		buf = appendClsString(buf, k)
		buf = appendClsString(buf, pairs[k])
	}

	return buf
}

// encodeClsMetadataRemove encodes the input of the cls_rbd metadata_remove
// method.
func encodeClsMetadataRemove(key string) []byte {
	return appendClsString(nil, key)
}

// clsMetadataOp calls a cls_rbd metadata method directly on the image header
// object. Unlike librbd, this neither acquires nor requests the exclusive lock
// of the image, so it can not cause the lock owner to be blocklisted.
func (ri *rbdImage) clsMetadataOp(method string, in []byte) error {
	err := ri.openIoctx()
	if err != nil {
		return err
	}
	err = ri.getImageID()
	if err != nil {
		return err
	}

	op := rados.CreateWriteOp()
	defer op.Release()
	// do not recreate an empty header object of a deleted image
	op.AssertExists()
	op.Exec(clsRBD, method, in)

	err = op.Operate(ri.ioctx, rbdHeaderPrefix+ri.ImageID, rados.OperationNoFlag)
	if errors.Is(err, rados.ErrNotFound) {
		return fmt.Errorf("%w: image %q: %w", rbderrors.ErrImageNotFound, ri, err)
	}

	return err
}
