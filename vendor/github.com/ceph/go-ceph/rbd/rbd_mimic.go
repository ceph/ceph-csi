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
	"bytes"
	"unsafe"

	"github.com/ceph/go-ceph/rados"
)

// GetImageNames returns the list of current RBD images.
func GetImageNames(ioctx *rados.IOContext) (names []string, err error) {
	buf := make([]byte, 4096)
	for {
		size := C.size_t(len(buf))
		ret := C.rbd_list(cephIoctx(ioctx),
			(*C.char)(unsafe.Pointer(&buf[0])), &size)
		if ret == -C.ERANGE {
			buf = make([]byte, size)
			continue
		} else if ret < 0 {
			return nil, RBDError(ret)
		}
		tmp := bytes.Split(buf[:size-1], []byte{0})
		for _, s := range tmp {
			if len(s) > 0 {
				name := C.GoString((*C.char)(unsafe.Pointer(&s[0])))
				names = append(names, name)
			}
		}
		return names, nil
	}
}
