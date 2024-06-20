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

package lockstate

import (
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/reftracker/errors"
)

// LockState describes type of the reftracker reference.
type LockState int8

const (
	LockStateSize = 1

	Unlocked LockState = 0

	Locked LockState = 1

	Unknown LockState = 2
)

func ToBytes(t LockState) []byte {
	return []byte{byte(t)}
}

func FromBytes(bs []byte) (LockState, error) {
	if len(bs) != LockStateSize {
		return Unlocked, errors.UnexpectedReadSize(LockStateSize, len(bs))
	}

	num := LockState(bs[0])
	switch num { //nolint:exhaustive // LockState.Unknown is handled in default case.
	case Unlocked, Locked:
		return num, nil
	default:
		return Unknown, fmt.Errorf("unknown LockState %d", num)
	}
}
