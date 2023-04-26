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

	ts "github.com/ceph/go-ceph/internal/timespec"
	"github.com/ceph/go-ceph/rados"
)

// GetImageNames returns the list of current RBD images.
func GetImageNames(ioctx *rados.IOContext) ([]string, error) {
	var images []C.rbd_image_spec_t
	size := C.size_t(4096)
	for {
		images = make([]C.rbd_image_spec_t, size)
		ret := C.rbd_list2(
			cephIoctx(ioctx),
			(*C.rbd_image_spec_t)(unsafe.Pointer(&images[0])),
			&size)
		err := getErrorIfNegative(ret)
		if err != nil {
			if err == errRange {
				continue
			}
			return nil, err
		}
		break
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
//
//	int rbd_get_create_timestamp(rbd_image_t image, struct timespec *timestamp);
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
//
//	int rbd_get_access_timestamp(rbd_image_t image, struct timespec *timestamp);
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
//
//	int rbd_get_modify_timestamp(rbd_image_t image, struct timespec *timestamp);
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

// Sparsify makes an image sparse by deallocating runs of zeros.
// The sparseSize value will be used to find runs of zeros and must be
// a power of two no less than 4096 and no larger than the image size.
//
// Implements:
//
//	int rbd_sparsify(rbd_image_t image, size_t sparse_size);
func (image *Image) Sparsify(sparseSize uint) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	return getError(C.rbd_sparsify(image.image, C.size_t(sparseSize)))
}
