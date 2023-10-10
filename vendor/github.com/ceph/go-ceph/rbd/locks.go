//go:build !nautilus
// +build !nautilus

package rbd

// #cgo LDFLAGS: -lrbd
// #include <errno.h>
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"
)

// LockMode represents a group of configurable lock modes.
type LockMode C.rbd_lock_mode_t

const (
	// LockModeExclusive is the representation of RBD_LOCK_MODE_EXCLUSIVE from librbd.
	LockModeExclusive = LockMode(C.RBD_LOCK_MODE_EXCLUSIVE)
	// LockModeShared is the representation of RBD_LOCK_MODE_SHARED from librbd.
	LockModeShared = LockMode(C.RBD_LOCK_MODE_SHARED)
)

// LockAcquire takes a lock on the given image as per the provided lock_mode.
//
// Implements:
//
//	int rbd_lock_acquire(rbd_image_t image, rbd_lock_mode_t lock_mode);
func (image *Image) LockAcquire(lockMode LockMode) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ret := C.rbd_lock_acquire(image.image, C.rbd_lock_mode_t(lockMode))

	return getError(ret)
}

// LockBreak breaks the lock of lock_mode on the provided lock_owner.
//
// Implements:
//
//	int rbd_lock_break(rbd_image_t image, rbd_lock_mode_t lock_mode,
//					   const char *lock_owner);
func (image *Image) LockBreak(lockMode LockMode, lockOwner string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cLockOwner := C.CString(lockOwner)
	defer C.free(unsafe.Pointer(cLockOwner))

	ret := C.rbd_lock_break(image.image, C.rbd_lock_mode_t(lockMode), cLockOwner)

	return getError(ret)
}

// LockOwner represents information about a lock owner.
type LockOwner struct {
	Mode  LockMode
	Owner string
}

// LockGetOwners fetches the list of lock owners.
//
// Implements:
//
//	int rbd_lock_get_owners(rbd_image_t image, rbd_lock_mode_t *lock_mode,
//							char **lock_owners, size_t *max_lock_owners);
func (image *Image) LockGetOwners() ([]*LockOwner, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var (
		maxLockOwners  = C.size_t(8)
		cLockOwners    = make([]*C.char, 8)
		lockMode       LockMode
		lockOwnersList []*LockOwner
	)

	for {
		ret := C.rbd_lock_get_owners(image.image, (*C.rbd_lock_mode_t)(&lockMode), &cLockOwners[0], &maxLockOwners)
		if ret >= 0 {
			break
		} else if ret == -C.ENOENT {
			return nil, nil
		} else if ret != -C.ERANGE {
			return nil, getError(ret)
		}
	}

	defer C.rbd_lock_get_owners_cleanup(&cLockOwners[0], maxLockOwners)

	for i := 0; i < int(maxLockOwners); i++ {
		lockOwnersList = append(lockOwnersList, &LockOwner{
			Mode:  LockMode(lockMode),
			Owner: C.GoString(cLockOwners[i]),
		})
	}

	return lockOwnersList, nil
}

// LockIsExclusiveOwner gets the status of the image exclusive lock.
//
// Implements:
//
//	int rbd_is_exclusive_lock_owner(rbd_image_t image, int *is_owner);
func (image *Image) LockIsExclusiveOwner() (bool, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return false, err
	}

	cIsOwner := C.int(0)

	ret := C.rbd_is_exclusive_lock_owner(image.image, &cIsOwner)
	if ret != 0 {
		return false, getError(ret)
	}

	return cIsOwner == 1, nil
}

// LockRelease releases a lock on the image.
//
// Implements:
//
//	int rbd_lock_release(rbd_image_t image);
func (image *Image) LockRelease() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ret := C.rbd_lock_release(image.image)

	return getError(ret)
}
