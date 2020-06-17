// +build !luminous
//
// Ceph Mimic is the first version that supports watchers through librbd.

package rbd

// #cgo LDFLAGS: -lrbd
// #include <errno.h>
// #include <rbd/librbd.h>
import "C"

import (
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
//   Only supported in Ceph Mimic and newer.
//
// Implements:
//   int rbd_watchers_list(rbd_image_t image,
//                         rbd_image_watcher_t *watchers, size_t *max_watchers)
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
