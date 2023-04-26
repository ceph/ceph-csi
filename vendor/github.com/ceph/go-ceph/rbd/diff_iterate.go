package rbd

/*
#cgo LDFLAGS: -lrbd
#undef _GNU_SOURCE
#include <errno.h>
#include <stdlib.h>
#include <rbd/librbd.h>

extern int diffIterateCallback(uint64_t, size_t, int, uintptr_t);

// inline wrapper to cast uintptr_t to void*
static inline int wrap_rbd_diff_iterate2(rbd_image_t image,
	const char *fromsnapname, uint64_t ofs, uint64_t len, uint8_t include_parent,
	uint8_t whole_object, uintptr_t arg) {
		return rbd_diff_iterate2(image, fromsnapname, ofs, len, include_parent,
			whole_object, (void*)diffIterateCallback, (void*)arg);
};
*/
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/callbacks"
)

var diffIterateCallbacks = callbacks.New()

// DiffIncludeParent values control if the difference should include the parent
// image.
type DiffIncludeParent uint8

// DiffWholeObject values control if the diff extents should cover the whole
// object.
type DiffWholeObject uint8

// DiffIterateCallback defines the function signature needed for the
// DiffIterate callback.
//
// The function will be called with the arguments: offset, length, exists, and
// data. The offset and length correspond to the changed region of the image.
// The exists value is set to zero if the region is known to be zeros,
// otherwise it is set to 1. The data value is the extra data parameter that
// was set on the DiffIterateConfig and is meant to be used for passing
// arbitrary user-defined items to the callback function.
//
// The callback can trigger the iteration to terminate early by returning
// a non-zero error code.
type DiffIterateCallback func(uint64, uint64, int, interface{}) int

// DiffIterateConfig is used to define the parameters of a DiffIterate call.
// Callback, Offset, and Length should always be specified when passed to
// DiffIterate. The other values are optional.
type DiffIterateConfig struct {
	SnapName      string
	Offset        uint64
	Length        uint64
	IncludeParent DiffIncludeParent
	WholeObject   DiffWholeObject
	Callback      DiffIterateCallback
	Data          interface{}
}

const (
	// ExcludeParent will exclude the parent from the diff.
	ExcludeParent = DiffIncludeParent(0)
	// IncludeParent will include the parent in the diff.
	IncludeParent = DiffIncludeParent(1)

	// DisableWholeObject will not use the whole object in the diff.
	DisableWholeObject = DiffWholeObject(0)
	// EnableWholeObject will use the whole object in the diff.
	EnableWholeObject = DiffWholeObject(1)
)

// DiffIterate calls a callback on changed extents of an image.
//
// Calling DiffIterate will cause the callback specified in the
// DiffIterateConfig to be called as many times as there are changed
// regions in the image (controlled by the parameters as passed to librbd).
//
// See the documentation of DiffIterateCallback for a description of the
// arguments to the callback and the return behavior.
//
// Implements:
//
//	int rbd_diff_iterate2(rbd_image_t image,
//	                      const char *fromsnapname,
//	                      uint64_t ofs, uint64_t len,
//	                      uint8_t include_parent, uint8_t whole_object,
//	                      int (*cb)(uint64_t, size_t, int, void *),
//	                      void *arg);
func (image *Image) DiffIterate(config DiffIterateConfig) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	if config.Callback == nil {
		return rbdError(C.EINVAL)
	}

	var cSnapName *C.char
	if config.SnapName != NoSnapshot {
		cSnapName = C.CString(config.SnapName)
		defer C.free(unsafe.Pointer(cSnapName))
	}

	cbIndex := diffIterateCallbacks.Add(config)
	defer diffIterateCallbacks.Remove(cbIndex)

	ret := C.wrap_rbd_diff_iterate2(
		image.image,
		cSnapName,
		C.uint64_t(config.Offset),
		C.uint64_t(config.Length),
		C.uint8_t(config.IncludeParent),
		C.uint8_t(config.WholeObject),
		C.uintptr_t(cbIndex))

	return getError(ret)
}

//export diffIterateCallback
func diffIterateCallback(
	offset C.uint64_t, length C.size_t, exists C.int, index uintptr) C.int {

	v := diffIterateCallbacks.Lookup(index)
	config := v.(DiffIterateConfig)
	return C.int(config.Callback(
		uint64(offset), uint64(length), int(exists), config.Data))
}
