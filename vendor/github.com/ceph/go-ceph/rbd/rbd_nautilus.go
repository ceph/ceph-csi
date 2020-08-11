// +build !luminous,!mimic
//
// Ceph Nautilus is the first release that includes rbd_list2() and
// rbd_get_create_timestamp().

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rados/librados.h>
// #include <rbd/librbd.h>
// #include <errno.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
	ts "github.com/ceph/go-ceph/internal/timespec"
	"github.com/ceph/go-ceph/rados"
)

// GetImageNames returns the list of current RBD images.
func GetImageNames(ioctx *rados.IOContext) ([]string, error) {
	var (
		err    error
		images []C.rbd_image_spec_t
		size   C.size_t
	)
	retry.WithSizes(32, 4096, func(s int) retry.Hint {
		size = C.size_t(s)
		images = make([]C.rbd_image_spec_t, size)
		ret := C.rbd_list2(
			cephIoctx(ioctx),
			(*C.rbd_image_spec_t)(unsafe.Pointer(&images[0])),
			&size)
		err = getErrorIfNegative(ret)
		return retry.Size(int(size)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}
	defer C.rbd_image_spec_list_cleanup((*C.rbd_image_spec_t)(unsafe.Pointer(&images[0])), size)

	names := make([]string, size)
	for i, image := range images[:size] {
		names[i] = C.GoString(image.name)
	}
	return names, nil
}

// GetCreateTimestamp returns the time the rbd image was created.
//
// Implements:
//  int rbd_get_create_timestamp(rbd_image_t image, struct timespec *timestamp);
func (image *Image) GetCreateTimestamp() (Timespec, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return Timespec{}, err
	}

	var cts C.struct_timespec

	if ret := C.rbd_get_create_timestamp(image.image, &cts); ret < 0 {
		return Timespec{}, getError(ret)
	}

	return Timespec(ts.CStructToTimespec(ts.CTimespecPtr(&cts))), nil
}

// GetAccessTimestamp returns the time the rbd image was last accessed.
//
// Implements:
//  int rbd_get_access_timestamp(rbd_image_t image, struct timespec *timestamp);
func (image *Image) GetAccessTimestamp() (Timespec, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return Timespec{}, err
	}

	var cts C.struct_timespec

	if ret := C.rbd_get_access_timestamp(image.image, &cts); ret < 0 {
		return Timespec{}, getError(ret)
	}

	return Timespec(ts.CStructToTimespec(ts.CTimespecPtr(&cts))), nil
}

// GetModifyTimestamp returns the time the rbd image was last modified.
//
// Implements:
//  int rbd_get_modify_timestamp(rbd_image_t image, struct timespec *timestamp);
func (image *Image) GetModifyTimestamp() (Timespec, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return Timespec{}, err
	}

	var cts C.struct_timespec

	if ret := C.rbd_get_modify_timestamp(image.image, &cts); ret < 0 {
		return Timespec{}, getError(ret)
	}

	return Timespec(ts.CStructToTimespec(ts.CTimespecPtr(&cts))), nil
}
