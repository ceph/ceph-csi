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

package reftype

import (
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/reftracker/errors"
)

// RefType describes type of the reftracker reference.
type RefType int8

const (
	refTypeSize = 1

	// Unknown reftype used to signal error state.
	Unknown RefType = 0

	// Normal type tags the reference to have normal effect on the reference
	// count. Adding Normal reference increments the reference count. Removing
	// Normal reference decrements the reference count.
	//
	// It may be converted to a Mask if it is removed with Mask reftype.
	Normal RefType = 1

	// Mask type tags the reference to be masked, making it not contribute to the
	// overall reference count. The reference will be ignored by all future Add()
	// calls until it is removed with Normal reftype.
	Mask RefType = 2
)

func ToBytes(t RefType) []byte {
	return []byte{byte(t)}
}

func FromBytes(bs []byte) (RefType, error) {
	if len(bs) != refTypeSize {
		return Unknown, errors.UnexpectedReadSize(refTypeSize, len(bs))
	}

	num := RefType(bs[0])
	switch num { //nolint:exhaustive // reftype.Unknown is handled in default case.
	case Normal, Mask:
		return num, nil
	default:
		return Unknown, fmt.Errorf("unknown reftype %d", num)
	}
}
