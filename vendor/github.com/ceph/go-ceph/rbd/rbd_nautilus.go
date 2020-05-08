// +build !luminous,!mimic
//
// Ceph Nautilus is the first release that includes rbd_list2().

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rados/librados.h>
// #include <rbd/librbd.h>
// #include <errno.h>
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/ceph/go-ceph/rados"
)

// GetImageNames returns the list of current RBD images.
func GetImageNames(ioctx *rados.IOContext) ([]string, error) {
	size := C.size_t(0)
	ret := C.rbd_list2(cephIoctx(ioctx), nil, &size)
	if ret < 0 && ret != -C.ERANGE {
		return nil, RBDError(ret)
	} else if ret > 0 {
		return nil, fmt.Errorf("rbd_list2() returned %d names, expected 0", ret)
	} else if ret == 0 && size == 0 {
		return nil, nil
	}

	// expected: ret == -ERANGE, size contains number of image names
	images := make([]C.rbd_image_spec_t, size)
	ret = C.rbd_list2(cephIoctx(ioctx), (*C.rbd_image_spec_t)(unsafe.Pointer(&images[0])), &size)
	if ret < 0 {
		return nil, RBDError(ret)
	}
	defer C.rbd_image_spec_list_cleanup((*C.rbd_image_spec_t)(unsafe.Pointer(&images[0])), size)

	names := make([]string, size)
	for i, image := range images {
		names[i] = C.GoString(image.name)
	}
	return names, nil
}
