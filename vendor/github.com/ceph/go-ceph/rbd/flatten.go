//go:build ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#include <errno.h>
#include <stdlib.h>
#include <rbd/librbd.h>

extern int flattenCallback(uint64_t, uint64_t, uintptr_t);

// inline wrapper to cast uintptr_t to void*
static inline int wrap_rbd_flatten_with_progress(
		rbd_image_t image, uintptr_t arg) {
	return rbd_flatten_with_progress(
		image, (librbd_progress_fn_t)flattenCallback, (void*)arg);
};
*/
import "C"

import (
	"github.com/ceph/go-ceph/internal/callbacks"
)

// FlattenCallback defines the function signature needed for the
// FlattenWithProgress callback.
//
// The callback will be called by FlattenWithProgress when it wishes to
// report progress on the flatten operation. The first argument is the
// number of objects flattened so far and the second argument is the total
// number of objects to flatten. The third argument is an opaque value
// that is passed to the FlattenWithProgress function's data argument and
// every call to the callback will receive the same object. The flatten
// operation will be aborted if the progress callback returns a non-zero
// value.
type FlattenCallback func(offset uint64, total uint64, data interface{}) int

var flattenCallbacks = callbacks.New()

type flattenCallbackCtx struct {
	callback FlattenCallback
	data     interface{}
}

// FlattenWithProgress removes snapshot references from the image, reporting
// progress via the supplied callback. The flatten operation will be aborted
// if the callback returns a non-zero value.
//
// Implements:
//
//	int rbd_flatten_with_progress(rbd_image_t image,
//	                              librbd_progress_fn_t cb,
//	                              void *cbdata);
func (image *Image) FlattenWithProgress(cb FlattenCallback, data interface{}) error {
	// the provided callback must be a real function
	if cb == nil {
		return getError(C.EINVAL)
	}

	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ctx := flattenCallbackCtx{
		callback: cb,
		data:     data,
	}
	cbIndex := flattenCallbacks.Add(ctx)
	defer flattenCallbacks.Remove(cbIndex)

	ret := C.wrap_rbd_flatten_with_progress(image.image, C.uintptr_t(cbIndex))
	return getError(ret)
}

//export flattenCallback
func flattenCallback(offset, total C.uint64_t, index uintptr) C.int {
	v := flattenCallbacks.Lookup(index)
	ctx := v.(flattenCallbackCtx)
	return C.int(ctx.callback(uint64(offset), uint64(total), ctx.data))
}
