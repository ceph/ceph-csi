// +build !luminous,!mimic

package rados

// #cgo LDFLAGS: -lrados
// #include <rados/librados.h>
//
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
)

// GetNamespace gets the namespace used for objects within this IO context.
//
// Implements:
//  int rados_ioctx_get_namespace(rados_ioctx_t io, char *buf,
//                                unsigned maxlen);
func (ioctx *IOContext) GetNamespace() (string, error) {
	if err := ioctx.validate(); err != nil {
		return "", err
	}
	var (
		err error
		buf []byte
		ret C.int
	)
	retry.WithSizes(128, 8192, func(size int) retry.Hint {
		buf = make([]byte, size)
		ret = C.rados_ioctx_get_namespace(
			ioctx.ioctx,
			(*C.char)(unsafe.Pointer(&buf[0])),
			C.unsigned(len(buf)))
		err = getErrorIfNegative(ret)
		return retry.DoubleSize.If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	return string(buf[:ret]), nil
}
