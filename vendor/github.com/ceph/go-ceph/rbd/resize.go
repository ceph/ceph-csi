//go:build ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#define _POSIX_C_SOURCE 200112L
#undef _GNU_SOURCE
#include <errno.h>
#include <stdlib.h>
#include <rados/librados.h>
#include <rbd/librbd.h>

extern int resize2Callback(uint64_t, uint64_t, uintptr_t);

// inline wrapper to cast uintptr_t to void*
static inline int wrap_rbd_resize2(
		rbd_image_t image, uint64_t size, bool allow_shrink, uintptr_t arg) {
	return rbd_resize2(
		image, size, allow_shrink, (librbd_progress_fn_t)resize2Callback, (void*)arg);
};
*/
import "C"

import (
	"github.com/ceph/go-ceph/internal/callbacks"
)

// Resize2ProgressCallback is the callback function type for Image.Resize2.
type Resize2ProgressCallback func(progress uint64, total uint64, data interface{}) int

var resizeCallbacks = callbacks.New()

type resizeProgressCallbackCtx struct {
	callback Resize2ProgressCallback
	data     interface{}
}

//export resize2Callback
func resize2Callback(
	offset, total C.uint64_t, index uintptr,
) C.int {
	v := resizeCallbacks.Lookup(index)
	ctx := v.(resizeProgressCallbackCtx)
	return C.int(ctx.callback(uint64(offset), uint64(total), ctx.data))
}

// Resize2 resizes an rbd image and allows configuration of allow_shrink and a callback function. The callback
// function will be called with the first argument as the progress, the second argument as the total, and the third
// argument as an opaque value that is passed to the Resize2 function's data argument in each callback execution.
// The resize operation will be aborted if the progress callback returns a non-zero value.
//
// Implements:
//
//	int rbd_resize(rbd_image_t image, uint64_t size, allow_shrink bool, librbd_progress_fn_t cb, void *cbdata);
func (image *Image) Resize2(size uint64, allowShrink bool, cb Resize2ProgressCallback, data interface{}) error {
	// the provided callback must be a real function
	if cb == nil {
		return rbdError(C.EINVAL)
	}

	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ctx := resizeProgressCallbackCtx{
		callback: cb,
		data:     data,
	}
	cbIndex := resizeCallbacks.Add(ctx)
	defer resizeCallbacks.Remove(cbIndex)

	ret := C.wrap_rbd_resize2(image.image, C.uint64_t(size), C.bool(allowShrink), C.uintptr_t(cbIndex))

	return getError(ret)

}
