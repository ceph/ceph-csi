// +build luminous mimic
// +build !nautilus
//
// Ceph Nautilus includes rbd_list2() and marked rbd_list() deprecated.

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rados/librados.h>
// #include <rbd/librbd.h>
// #include <errno.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/cutil"
	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// GetImageNames returns the list of current RBD images.
func GetImageNames(ioctx *rados.IOContext) (names []string, err error) {
	var (
		buf   []byte
		csize C.size_t
	)
	// from 4KiB to 32KiB
	retry.WithSizes(4096, 1<<15, func(size int) retry.Hint {
		csize = C.size_t(size)
		buf = make([]byte, csize)
		ret := C.rbd_list(cephIoctx(ioctx),
			(*C.char)(unsafe.Pointer(&buf[0])), &csize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(csize)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}
	names = cutil.SplitSparseBuffer(buf[:csize])
	return names, nil
}
