package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/cutil"
	"github.com/ceph/go-ceph/internal/retry"
)

// GetMetadata returns the metadata string associated with the given key.
//
// Implements:
//
//	int rbd_metadata_get(rbd_image_t image, const char *key, char *value, size_t *vallen)
func (image *Image) GetMetadata(key string) (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	var (
		buf []byte
		err error
	)
	retry.WithSizes(4096, 262144, func(size int) retry.Hint {
		csize := C.size_t(size)
		buf = make([]byte, csize)
		// rbd_metadata_get is a bit quirky and *does not* update the size
		// value if the size passed in >= the needed size.
		ret := C.rbd_metadata_get(
			image.image, cKey, (*C.char)(unsafe.Pointer(&buf[0])), &csize)
		err = getError(ret)
		return retry.Size(int(csize)).If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
}

// SetMetadata updates the metadata string associated with the given key.
//
// Implements:
//
//	int rbd_metadata_set(rbd_image_t image, const char *key, const char *value)
func (image *Image) SetMetadata(key string, value string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cKey := C.CString(key)
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cKey))
	defer C.free(unsafe.Pointer(cValue))

	ret := C.rbd_metadata_set(image.image, cKey, cValue)
	if ret < 0 {
		return rbdError(ret)
	}

	return nil
}

// RemoveMetadata clears the metadata associated with the given key.
//
// Implements:
//
//	int rbd_metadata_remove(rbd_image_t image, const char *key)
func (image *Image) RemoveMetadata(key string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	ret := C.rbd_metadata_remove(image.image, cKey)
	if ret < 0 {
		return rbdError(ret)
	}

	return nil
}

// ListMetadata returns a map containing all metadata assigned to the RBD image.
//
// Implements:
//
//	int rbd_metadata_list(rbd_image_t image, const char *start, uint64_t max,
//	                      char *keys, size_t *key_len, char *values, size_t *vals_len);
func (image *Image) ListMetadata() (map[string]string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var (
		err      error
		keysbuf  []byte
		keysSize C.size_t
		valsbuf  []byte
		valsSize C.size_t
	)
	retry.WithSizes(4096, 262144, func(size int) retry.Hint {
		keysbuf = make([]byte, size)
		keysSize = C.size_t(size)
		valsbuf = make([]byte, size)
		valsSize = C.size_t(size)
		// the rbd_metadata_list function can use a start point and a limit.
		// we do not use it and prefer our retry helper and just allocating
		// buffers large enough to take all the keys and values
		ret := C.rbd_metadata_list(
			image.image,
			(*C.char)(unsafe.Pointer(&empty[0])), // always start at the beginning (no paging)
			0,                                    // fetch all key-value pairs
			(*C.char)(unsafe.Pointer(&keysbuf[0])),
			&keysSize,
			(*C.char)(unsafe.Pointer(&valsbuf[0])),
			&valsSize)

		err = getError(ret)
		nextSize := valsSize
		if keysSize > nextSize {
			nextSize = keysSize
		}
		return retry.Size(int(nextSize)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}

	m := map[string]string{}
	keys := cutil.SplitBuffer(keysbuf[:keysSize])
	vals := cutil.SplitBuffer(valsbuf[:valsSize])
	if len(keys) != len(vals) {
		// this should not happen (famous last words)
		return nil, errRange
	}
	for i := range keys {
		m[keys[i]] = vals[i]
	}
	return m, nil
}

// rather than allocate memory every time that ListMetadata is called,
// define a static byte slice to stand in for the C "empty string"
var empty = []byte{0}
