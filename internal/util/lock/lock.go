/*
Copyright 2024 The Ceph-CSI Authors.

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

package lock

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/ceph/go-ceph/rados"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// IOCtxLock provides methods for acquiring and releasing exclusive locks on a volume.
// using rados IO context locks.
type IOCtxLock interface {
	LockExclusive(ctx context.Context) error
	Unlock(ctx context.Context)
}

type lock struct {
	volID      string
	lockName   string
	lockDesc   string
	lockCookie string
	timeout    time.Duration
	ioctx      *rados.IOContext
}

// NewLock returns `lock` type that implements the IOCtxLock interface.
func NewLock(
	ioctx *rados.IOContext,
	volID string,
	lockName string,
	lockCookie string,
	lockDesc string,
	timeout time.Duration,
) IOCtxLock {
	return &lock{
		volID:      volID,
		lockName:   lockName,
		lockDesc:   lockDesc,
		lockCookie: lockCookie,
		timeout:    timeout,
		ioctx:      ioctx,
	}
}

// LockExclusive acquires an exclusive lock on the volume identified by
// the name and cookie pair.
func (lck *lock) LockExclusive(ctx context.Context) error {
	var flags byte = 0
	ret, err := lck.ioctx.LockExclusive(
		lck.volID,
		lck.lockName,
		lck.lockCookie,
		lck.lockDesc,
		lck.timeout,
		&flags)

	if ret != 0 {
		switch ret {
		case -int(syscall.EBUSY):
			return fmt.Errorf("lock is already held by another client and cookie pair for %v volume",
				lck.volID)
		case -int(syscall.EEXIST):
			return fmt.Errorf("lock is already held by the same client and cookie pair for %v volume",
				lck.volID)
		default:
			return fmt.Errorf("failed to lock volume ID %v: %w", lck.volID, err)
		}
	}

	return nil
}

// Unlock releases the exclusive lock on the volume.
func (lck *lock) Unlock(ctx context.Context) {
	ret, err := lck.ioctx.Unlock(lck.volID, lck.lockName, lck.lockCookie)

	switch ret {
	case 0:
		log.DebugLog(ctx, "lock %s for vol id %s successfully released ",
			lck.lockName, lck.volID)
	case -int(syscall.ENOENT):
		log.DebugLog(ctx, "lock is not held by the specified %s, %s pair",
			lck.lockCookie, lck.lockName)
	default:
		log.ErrorLog(ctx, "failed to release lock %s: %v",
			lck.lockName, err)
	}
}
