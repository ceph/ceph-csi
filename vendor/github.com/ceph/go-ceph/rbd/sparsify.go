//go:build !nautilus && ceph_preview
// +build !nautilus,ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#include <errno.h>
#include <stdlib.h>
#include <rbd/librbd.h>

extern int sparsifyCallback(uint64_t, uint64_t, uintptr_t);

// inline wrapper to cast uintptr_t to void*
static inline int wrap_rbd_sparsify_with_progress(
		rbd_image_t image, size_t sparse_size, uintptr_t arg) {
	return rbd_sparsify_with_progress(
		image, sparse_size, (librbd_progress_fn_t)sparsifyCallback, (void*)arg);
};
*/
import "C"

import (
	"github.com/ceph/go-ceph/internal/callbacks"
)

// SparsifyCallback defines the function signature needed for the
// SparsifyWithProgress callback.
//
// This callback will be called by SparsifyWithProgress when it wishes to
// report progress on sparse. The callback function will be called with the
// first argument containing the current offset within the image being made
// sparse and the second argument containing the total size of the image. The
// third argument is an opaque value that is passed to the SparsifyWithProgress
// function's data argument and every call to the callback will receive the
// same object. The sparsify operation will be aborted if the progress
// callback returns a non-zero value.
type SparsifyCallback func(uint64, uint64, interface{}) int

var sparsifyCallbacks = callbacks.New()

type sparsifyCallbackCtx struct {
	callback SparsifyCallback
	data     interface{}
}

// SparsifyWithProgress makes an image sparse by deallocating runs of zeros.
// The sparseSize value will be used to find runs of zeros and must be
// a power of two no less than 4096 and no larger than the image size.
// The given progress callback will be called to report on the progress
// of sparse. The operation will be aborted if the progress callback returns
// a non-zero value.
//
// Implements:
//
//	int rbd_sparsify_with_progress(rbd_image_t image, size_t sparse_size,
//								   librbd_progress_fn_t cb, void *cbdata);
func (image *Image) SparsifyWithProgress(
	sparseSize uint, cb SparsifyCallback, data interface{}) error {
	// the provided callback must be a real function
	if cb == nil {
		return rbdError(C.EINVAL)
	}

	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ctx := sparsifyCallbackCtx{
		callback: cb,
		data:     data,
	}
	cbIndex := sparsifyCallbacks.Add(ctx)
	defer diffIterateCallbacks.Remove(cbIndex)

	ret := C.wrap_rbd_sparsify_with_progress(image.image, C.size_t(sparseSize), C.uintptr_t(cbIndex))

	return getError(ret)
}

//export sparsifyCallback
func sparsifyCallback(
	offset, total C.uint64_t, index uintptr) C.int {

	v := sparsifyCallbacks.Lookup(index)
	ctx := v.(sparsifyCallbackCtx)
	return C.int(ctx.callback(uint64(offset), uint64(total), ctx.data))
}
