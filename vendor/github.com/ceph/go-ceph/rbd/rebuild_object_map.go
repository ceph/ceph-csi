//go:build ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#include <errno.h>
#include <stdint.h>
#include <rbd/librbd.h>

extern int rebuildObjectMapCallback(uint64_t, uint64_t, uintptr_t);

static int rebuild_object_map_callback(
		uint64_t objectNumber, uint64_t objectCount, void *arg) {
	return rebuildObjectMapCallback(objectNumber, objectCount, (uintptr_t)arg);
}

static inline int wrap_rbd_rebuild_object_map(rbd_image_t image, uintptr_t arg) {
	return rbd_rebuild_object_map(
		image, rebuild_object_map_callback, (void*)arg);
}
*/
import "C"

import "github.com/ceph/go-ceph/internal/callbacks"

// RebuildObjectMapCallback defines the function signature needed by
// RebuildObjectMapWithProgress.
//
// The callback receives the current object number, the total number of
// objects, and the opaque data passed to RebuildObjectMapWithProgress. A
// negative return value requests that a locally executing rebuild abort. The
// callback may be invoked from a Ceph-managed thread, so callers must
// synchronize access to shared data.
type RebuildObjectMapCallback func(objectNumber uint64, objectCount uint64, data interface{}) int

var rebuildObjectMapCallbacks = callbacks.New()

type rebuildObjectMapCallbackCtx struct {
	callback RebuildObjectMapCallback
	data     interface{}
}

// RebuildObjectMapWithProgress rebuilds the object map for the image HEAD or
// currently selected snapshot, reporting progress via the supplied callback.
// The image must have the object-map feature enabled, and the image handle
// must not have been explicitly opened read-only.
// A negative callback return value requests that a locally executing rebuild
// abort; in-flight work may finish before the operation returns.
// If the rebuild is forwarded to a remote exclusive-lock owner, callback
// cancellation cannot be propagated to that client.
//
// Implements:
//
//	int rbd_rebuild_object_map(rbd_image_t image,
//	                           librbd_progress_fn_t cb, void *cbdata);
func (image *Image) RebuildObjectMapWithProgress(
	cb RebuildObjectMapCallback, data interface{},
) error {
	if cb == nil {
		return getError(-C.EINVAL)
	}

	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ctx := rebuildObjectMapCallbackCtx{
		callback: cb,
		data:     data,
	}
	cbIndex := rebuildObjectMapCallbacks.Add(ctx)
	defer rebuildObjectMapCallbacks.Remove(cbIndex)

	ret := C.wrap_rbd_rebuild_object_map(image.image, C.uintptr_t(cbIndex))
	return getError(ret)
}

//export rebuildObjectMapCallback
func rebuildObjectMapCallback(objectNumber, objectCount C.uint64_t, index uintptr) C.int {
	v := rebuildObjectMapCallbacks.Lookup(index)
	ctx := v.(rebuildObjectMapCallbackCtx)
	return C.int(ctx.callback(uint64(objectNumber), uint64(objectCount), ctx.data))
}
