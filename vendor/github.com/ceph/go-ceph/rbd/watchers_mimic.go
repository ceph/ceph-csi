// +build !luminous
//
// Ceph Mimic is the first version that supports watchers through librbd.

package rbd

// #cgo LDFLAGS: -lrbd
// #include <errno.h>
// #include <rbd/librbd.h>
import "C"

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

	count := C.ulong(0)
	ret := C.rbd_watchers_list(image.image, nil, &count)
	if ret != 0 && ret != -C.ERANGE {
		return nil, getError(ret)
	}
	if ret == 0 && count == 0 {
		return nil, nil
	}

	watchers := make([]C.rbd_image_watcher_t, count)
	ret = C.rbd_watchers_list(image.image, &watchers[0], &count)
	if ret != 0 && ret != -C.ERANGE {
		return nil, getError(ret)
	}
	defer C.rbd_watchers_list_cleanup(&watchers[0], count)

	imageWatchers := make([]ImageWatcher, len(watchers))
	for i, watcher := range watchers {
		imageWatchers[i].Addr = C.GoString(watcher.addr)
		imageWatchers[i].Id = int64(watcher.id)
		imageWatchers[i].Cookie = uint64(watcher.cookie)
	}

	return imageWatchers, nil
}
