package rbd

/*
#cgo LDFLAGS: -lrbd
#include <rbd/librbd.h>

extern void imageWatchCallback(uintptr_t);

// inline wrapper to cast uintptr_t to void*
static inline int wrap_rbd_update_watch(rbd_image_t image, uint64_t *handle,
	uintptr_t arg) {
		return rbd_update_watch(image, handle, (void*)imageWatchCallback, (void*)arg);
	};

*/
import "C"

import (
	"github.com/ceph/go-ceph/internal/callbacks"
	"github.com/ceph/go-ceph/internal/retry"
)

// ImageWatcher is a representation of the rbd_image_watcher_t from librbd.h
type ImageWatcher struct {
	Addr   string
	Id     int64
	Cookie uint64
}

// ListWatchers returns the watchers on an RBD image. In case of an error, nil
// and an error are returned.
//
// Note:
//
//	Only supported in Ceph Mimic and newer.
//
// Implements:
//
//	int rbd_watchers_list(rbd_image_t image,
//	                      rbd_image_watcher_t *watchers, size_t *max_watchers)
func (image *Image) ListWatchers() ([]ImageWatcher, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var (
		err      error
		count    C.size_t
		watchers []C.rbd_image_watcher_t
	)
	retry.WithSizes(16, 4096, func(size int) retry.Hint {
		count = C.size_t(size)
		watchers = make([]C.rbd_image_watcher_t, count)
		ret := C.rbd_watchers_list(image.image, &watchers[0], &count)
		err = getErrorIfNegative(ret)
		return retry.Size(int(count)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}
	defer C.rbd_watchers_list_cleanup(&watchers[0], count)

	imageWatchers := make([]ImageWatcher, count)
	for i, watcher := range watchers[:count] {
		imageWatchers[i].Addr = C.GoString(watcher.addr)
		imageWatchers[i].Id = int64(watcher.id)
		imageWatchers[i].Cookie = uint64(watcher.cookie)
	}
	return imageWatchers, nil
}

// watchCallbacks tracks the active callbacks for rbd watches
var watchCallbacks = callbacks.New()

// WatchCallback defines the function signature needed for the UpdateWatch
// callback.
type WatchCallback func(interface{})

type watchCallbackCtx struct {
	callback WatchCallback
	data     interface{}
}

// Watch represents an ongoing image metadata watch.
type Watch struct {
	image   *Image
	wcc     watchCallbackCtx
	handle  C.uint64_t
	cbIndex uintptr
}

// UpdateWatch updates the image object to watch metadata changes to the
// image, returning a Watch object.
//
// Implements:
//
//	int rbd_update_watch(rbd_image_t image, uint64_t *handle,
//	                     rbd_update_callback_t watch_cb, void *arg);
func (image *Image) UpdateWatch(cb WatchCallback, data interface{}) (*Watch, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}
	wcc := watchCallbackCtx{
		callback: cb,
		data:     data,
	}
	w := &Watch{
		image:   image,
		wcc:     wcc,
		cbIndex: watchCallbacks.Add(wcc),
	}

	ret := C.wrap_rbd_update_watch(
		image.image,
		&w.handle,
		C.uintptr_t(w.cbIndex))
	if ret != 0 {
		return nil, getError(ret)
	}
	return w, nil
}

// Unwatch un-registers the image watch.
//
// Implements:
//
//	int rbd_update_unwatch(rbd_image_t image, uint64_t handle);
func (w *Watch) Unwatch() error {
	if w.image == nil {
		return ErrImageNotOpen
	}
	if err := w.image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_update_unwatch(w.image.image, w.handle)
	watchCallbacks.Remove(w.cbIndex)
	return getError(ret)
}

//export imageWatchCallback
func imageWatchCallback(index uintptr) {
	v := watchCallbacks.Lookup(index)
	wcc := v.(watchCallbackCtx)
	wcc.callback(wcc.data)
}
