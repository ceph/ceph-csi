//go:build ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
)

// GetBlockNamePrefix returns the prefix of the names of the RADOS objects
// that hold the image's data. Unlike the Block_name_prefix field of the
// ImageInfo returned by Stat, the value is not truncated.
//
// Implements:
//
//	int rbd_get_block_name_prefix(rbd_image_t image, char *prefix,
//	                              size_t prefix_len);
func (image *Image) GetBlockNamePrefix() (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}
	var (
		err error
		buf []byte
	)
	// librbd caps the prefix at 43 characters; retry in case that grows
	retry.WithSizes(64, 8192, func(size int) retry.Hint {
		buf = make([]byte, size)
		ret := C.rbd_get_block_name_prefix(
			image.image,
			(*C.char)(unsafe.Pointer(&buf[0])),
			C.size_t(size))
		err = getErrorIfNegative(ret)
		return retry.DoubleSize.If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
}
