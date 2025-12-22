//go:build ceph_preview
// +build ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#include <rbd/librbd.h>
*/
import "C"

// RemoveSnapByID removes an existing snapshot. This can be used to manually
// remove a snapshot from the trash.
//
// Implements:
//
//	int rbd_snap_remove_by_id(rbd_image_t image, uint64_t snap_id);
func (image *Image) RemoveSnapByID(snapID uint64) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ret := C.rbd_snap_remove_by_id(image.image, C.uint64_t(snapID))
	return getError(ret)
}
