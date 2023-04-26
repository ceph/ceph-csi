//go:build !nautilus
// +build !nautilus

package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
)

// GetSnapID returns the snapshot ID for the given snapshot name.
//
// Implements:
//
//	int rbd_snap_get_id(rbd_image_t image, const char *snapname, uint64_t *snap_id)
func (image *Image) GetSnapID(snapName string) (uint64, error) {
	var snapID C.uint64_t
	if err := image.validate(imageIsOpen); err != nil {
		return uint64(snapID), err
	}
	if snapName == "" {
		return uint64(snapID), ErrSnapshotNoName
	}

	cSnapName := C.CString(snapName)
	defer C.free(unsafe.Pointer(cSnapName))

	ret := C.rbd_snap_get_id(image.image, cSnapName, &snapID)
	return uint64(snapID), getError(ret)
}

// GetSnapByID returns the snapshot name for the given snapshot ID.
//
// Implements:
//
//	int rbd_snap_get_name(rbd_image_t image, uint64_t snap_id, char *snapname, size_t *name_len)
func (image *Image) GetSnapByID(snapID uint64) (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}

	var (
		buf []byte
		err error
	)
	// range from 1k to 64KiB
	retry.WithSizes(1024, 1<<16, func(len int) retry.Hint {
		cLen := C.size_t(len)
		buf = make([]byte, cLen)
		ret := C.rbd_snap_get_name(
			image.image,
			(C.uint64_t)(snapID),
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cLen)
		err = getError(ret)
		return retry.Size(int(cLen)).If(err == errRange)
	})

	if err != nil {
		return "", err
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
}
