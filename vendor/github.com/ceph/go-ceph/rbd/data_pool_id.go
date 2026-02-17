package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
import "C"

// GetDataPoolID returns the ID of the data pool assigned to an image
//
// Implements:
//
//	int64_t rbd_get_data_pool_id(rbd_image_t image)
func (image *Image) GetDataPoolID() (int64, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return -1, err
	}

	return int64(C.rbd_get_data_pool_id(image.image)), nil
}
