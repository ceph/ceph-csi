// +build !luminous,!mimic
//
// Ceph Nautilus is the first release that includes rbd_pool_metadata_get(),
// rbd_pool_metadata_set() and rbd_pool_metadata_remove().

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rados/librados.h>
// #include <rbd/librbd.h>
// #include <stdlib.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// GetPoolMetadata returns pool metadata associated with the given key.
//
// Implements:
//  int rbd_pool_metadata_get(rados_ioctx_t io_ctx, const char *key, char *value, size_t *val_len);
func GetPoolMetadata(ioctx *rados.IOContext, key string) (string, error) {
	if ioctx == nil {
		return "", ErrNoIOContext
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	var (
		buf []byte
		err error
	)
	retry.WithSizes(4096, 262144, func(size int) retry.Hint {
		cSize := C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_pool_metadata_get(cephIoctx(ioctx),
			cKey,
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getError(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})

	if err != nil {
		return "", err
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
}

// SetPoolMetadata updates the pool metadata string associated with the given key.
//
// Implements:
//  int rbd_pool_metadata_set(rados_ioctx_t io_ctx, const char *key, const char *value);
func SetPoolMetadata(ioctx *rados.IOContext, key, value string) error {
	if ioctx == nil {
		return ErrNoIOContext
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))

	ret := C.rbd_pool_metadata_set(cephIoctx(ioctx), cKey, cValue)
	return getError(ret)
}

// RemovePoolMetadata removes the pool metadata value for a given pool metadata key.
//
// Implements:
//  int rbd_pool_metadata_remove(rados_ioctx_t io_ctx, const char *key)
func RemovePoolMetadata(ioctx *rados.IOContext, key string) error {
	if ioctx == nil {
		return ErrNoIOContext
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	ret := C.rbd_pool_metadata_remove(cephIoctx(ioctx), cKey)
	return getError(ret)
}
