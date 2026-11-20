//go:build ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
import "C"

const (
	// FlagObjectMapInvalid indicates that the image's object map is invalid.
	// It represents RBD_FLAG_OBJECT_MAP_INVALID from librbd.
	FlagObjectMapInvalid = uint64(C.RBD_FLAG_OBJECT_MAP_INVALID)

	// FlagFastDiffInvalid indicates that the image's fast-diff metadata is invalid.
	// It represents RBD_FLAG_FAST_DIFF_INVALID from librbd.
	FlagFastDiffInvalid = uint64(C.RBD_FLAG_FAST_DIFF_INVALID)
)

// GetFlags returns the status flags bitmask for the image HEAD or currently
// selected snapshot. It can be called on a read-only image handle.
// The returned mask preserves all flag bits reported by librbd.
// Use GetFeatures to determine which image features are enabled.
//
// Implements:
//
//	int rbd_get_flags(rbd_image_t image, uint64_t *flags);
func (image *Image) GetFlags() (uint64, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	var flags C.uint64_t
	if ret := C.rbd_get_flags(image.image, &flags); ret < 0 {
		return 0, getError(ret)
	}
	return uint64(flags), nil
}
