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

package v1

import (
	"context"
	"fmt"
	"time"

	"github.com/ceph/ceph-csi/internal/util/log"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/errors"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/lock"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/lockstate"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/retryoptions"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/version"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"

	"github.com/ceph/go-ceph/rados"
)

const (
	Version = 1
)

// Init atomically initializes a new lock object.
func Init(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	lockOwner string,
) error {
	// Create lock instance.
	lockData := lock.Lock{
		LockOwner:  lockOwner,
		LockState:  lockstate.Locked,
		LockExpiry: time.Now().Add(30 * time.Second),
	}

	// Serialize the lock.
	lockBytes, err := lockData.ToBytes()
	if err != nil {
		return err
	}

	// Perform the write.
	w := ioctx.CreateWriteOp()
	defer w.Release()

	w.Create(rados.CreateExclusive)
	w.SetXattr(version.XattrName, version.ToBytes(Version))
	w.SetOmap(map[string][]byte{lockName: lockBytes})

	return errors.FailedObjectWrite(w.Operate(lockName))
}

func TryToAquireLock(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	lockOwner string,
	retryOptions retryoptions.RetryOptions,
) (lock.Lock, error) {

	var lastLock lock.Lock
	var lastErr error

	for i := 0; i < retryOptions.MaxAttempts; i++ {
		lock, err := aquireLock(ctx, ioctx, lockName, lockOwner)
		if err == nil {
			return lock, nil
		}
		lastLock = lock
		lastErr = err
		time.Sleep(retryOptions.SleepDuration)
	}

	return lastLock, fmt.Errorf("Lock could not be acquired after %d attempts: %w", retryOptions.MaxAttempts, lastErr)
}

// Atomically trying to acquire an existing lock object.
func aquireLock(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	lockOwner string,
) (lock.Lock, error) {

	fmt.Println("Trying to get lock for my owner: ")
	fmt.Println(lockOwner)
	var currentLock lock.Lock
	gen, err := ioctx.GetLastVersion()
	if err != nil {
		return currentLock, fmt.Errorf("failed to get RADOS object version: %w", err)
	}

	w := ioctx.CreateWriteOp()
	defer w.Release()

	w.AssertVersion(gen)

	currentLock, err = ReadLock(ioctx, lockName, gen)
	if err != nil {
		return currentLock, errors.FailedObjectRead(err)
	}

	if currentLock.LockState == lockstate.Locked && !currentLock.LockExpiry.Before(time.Now()) {
		log.DebugLog(ctx, "Could not acquire lock as it is owned by %s and expires in %s",
			currentLock.LockOwner,
			currentLock.LockExpiry,
		)
		return currentLock, fmt.Errorf("Cannot acquire lock")
	}

	if currentLock.LockExpiry.Before((time.Now())) {
		log.DebugLog(ctx, "Lock owned by %s has expired at %s, try to aquire",
			currentLock.LockOwner,
			currentLock.LockExpiry,
		)
	}

	newLockStatus := lockstate.Locked
	newExpiryTime := time.Now().Add(30 * time.Second)

	newLock := lock.Lock{
		LockOwner:  lockOwner,
		LockState:  newLockStatus,
		LockExpiry: newExpiryTime,
	}

	newLockBytes, err := newLock.ToBytes()
	if err != nil {
		return lock.Lock{}, err
	}

	writeOp := ioctx.CreateWriteOp()
	defer writeOp.Release()
	writeOp.AssertVersion(gen)

	writeOp.SetOmap(map[string][]byte{lockName: newLockBytes})
	err = writeOp.Operate(lockName)

	if err != nil {
		return currentLock, err
	}
	log.DebugLog(ctx, "Succesfully aquired the lock and moved as new owner")
	return newLock, nil
}

func ReadLock(
	ioctx radoswrapper.IOContextW,
	lockName string,
	gen uint64,
) (lock.Lock, error) {

	w := ioctx.CreateReadOp()
	defer w.Release()
	w.AssertVersion(gen)

	var lockBytes []byte
	s := w.GetOmapValuesByKeys([]string{lockName})

	err := w.Operate(lockName)
	if err != nil {
		return lock.Lock{}, errors.FailedObjectRead(err)
	}

	kvPair, err := s.Next()
	lockBytes = kvPair.Value

	if len(lockBytes) == 0 {
		return lock.Lock{}, nil
	}

	var lockData lock.Lock
	err = lockData.FromBytes(lockBytes)
	if err != nil {
		return lock.Lock{}, err
	}

	return lockData, nil
}

// ReleaseLock frees a lock from the RADOS pool to be aquired.
func ReleaseLock(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	lockOwner string,
	gen uint64,
) error {
	var currentLock lock.Lock
	gen, err := ioctx.GetLastVersion()
	if err != nil {
		return fmt.Errorf("failed to get RADOS object version: %w", err)
	}

	w := ioctx.CreateWriteOp()
	defer w.Release()

	w.AssertVersion(gen)

	currentLock, err = ReadLock(ioctx, lockName, gen)
	if err != nil {
		return errors.FailedObjectRead(err)
	}

	if currentLock.LockOwner != lockOwner {
		fmt.Println("Cannot release a lock that is owned by another party")
		return fmt.Errorf("Cannot release a lock that is owned by another party")
	}

	releasedLock := lock.Lock{
		LockState:  lockstate.Unlocked,
		LockOwner:  "",
		LockExpiry: time.Time{},
	}

	newLockBytes, err := releasedLock.ToBytes()
	if err != nil {
		return err
	}

	writeOp := ioctx.CreateWriteOp()
	defer writeOp.Release()
	writeOp.AssertVersion(gen)

	writeOp.SetOmap(map[string][]byte{lockName: newLockBytes})
	err = writeOp.Operate(lockName)

	if err != nil {
		return err
	}

	return nil
}

// DeleteLock deletes a lock object from the RADOS pool.
func DeleteLock(ctx context.Context, ioctx radoswrapper.IOContextW, lockName string, gen uint64) error {
	deleteOp := ioctx.CreateWriteOp()
	defer deleteOp.Release()
	deleteOp.AssertVersion(gen)

	deleteOp.Remove()
	if err := deleteOp.Operate(lockName); err != nil {
		return fmt.Errorf("failed to operate on lock object: %w", err)
	}

	fmt.Printf("Lock object %s deleted successfully\n", lockName)
	return nil
}
