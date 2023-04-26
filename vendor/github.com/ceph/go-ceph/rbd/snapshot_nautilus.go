//
// Ceph Nautilus introduced rbd_get_parent() and deprecated rbd_get_parent_info().
// Ceph Nautilus introduced rbd_list_children3() and deprecated rbd_list_children().

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
// #include <errno.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
)

// GetParentInfo looks for the parent of the image and stores the pool, name
// and snapshot-name in the byte-arrays that are passed as arguments.
//
// Implements:
//
//	int rbd_get_parent(rbd_image_t image,
//	                   rbd_linked_image_spec_t *parent_image,
//	                   rbd_snap_spec_t *parent_snap)
func (image *Image) GetParentInfo(pool, name, snapname []byte) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	parentImage := C.rbd_linked_image_spec_t{}
	parentSnap := C.rbd_snap_spec_t{}
	ret := C.rbd_get_parent(image.image, &parentImage, &parentSnap)
	if ret != 0 {
		return rbdError(ret)
	}

	defer C.rbd_linked_image_spec_cleanup(&parentImage)
	defer C.rbd_snap_spec_cleanup(&parentSnap)

	strlen := int(C.strlen(parentImage.pool_name))
	if len(pool) < strlen {
		return rbdError(C.ERANGE)
	}
	if copy(pool, C.GoString(parentImage.pool_name)) != strlen {
		return rbdError(C.ERANGE)
	}

	strlen = int(C.strlen(parentImage.image_name))
	if len(name) < strlen {
		return rbdError(C.ERANGE)
	}
	if copy(name, C.GoString(parentImage.image_name)) != strlen {
		return rbdError(C.ERANGE)
	}

	strlen = int(C.strlen(parentSnap.name))
	if len(snapname) < strlen {
		return rbdError(C.ERANGE)
	}
	if copy(snapname, C.GoString(parentSnap.name)) != strlen {
		return rbdError(C.ERANGE)
	}

	return nil
}

// ImageSpec represents the image information.
type ImageSpec struct {
	ImageName string
	PoolName  string
}

// SnapSpec represents the snapshot infomation.
type SnapSpec struct {
	ID       uint64
	SnapName string
}

// ParentInfo represents the parent image and the parent snapshot information.
type ParentInfo struct {
	Image ImageSpec
	Snap  SnapSpec
}

// GetParent looks for the parent of the image and returns the parent image
// information which includes the image name, the pool name and
// the snapshot information.
//
// Implements:
// int rbd_get_parent(rbd_image_t image, rbd_linked_image_spec_t *parent_image, rbd_snap_spec_t *parent_snap)
func (image *Image) GetParent() (*ParentInfo, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	parentImage := C.rbd_linked_image_spec_t{}
	parentSnap := C.rbd_snap_spec_t{}
	ret := C.rbd_get_parent(image.image, &parentImage, &parentSnap)
	if ret != 0 {
		return nil, getError(ret)
	}
	defer C.rbd_linked_image_spec_cleanup(&parentImage)
	defer C.rbd_snap_spec_cleanup(&parentSnap)

	imageSpec := ImageSpec{
		ImageName: C.GoString(parentImage.image_name),
		PoolName:  C.GoString(parentImage.pool_name),
	}

	snapSpec := SnapSpec{
		ID:       uint64(parentSnap.id),
		SnapName: C.GoString(parentSnap.name),
	}

	return &ParentInfo{
		Image: imageSpec,
		Snap:  snapSpec,
	}, nil
}

// ListChildren returns arrays with the pools and names of the images that are
// children of the given image. The index of the pools and images arrays can be
// used to link the two items together.
//
// Implements:
//
//	int rbd_list_children3(rbd_image_t image, rbd_linked_image_spec_t *images,
//	                       size_t *max_images);
func (image *Image) ListChildren() (pools []string, images []string, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, nil, err
	}

	var (
		csize    C.size_t
		children []C.rbd_linked_image_spec_t
	)
	retry.WithSizes(16, 4096, func(size int) retry.Hint {
		csize = C.size_t(size)
		children = make([]C.rbd_linked_image_spec_t, csize)
		ret := C.rbd_list_children3(
			image.image,
			(*C.rbd_linked_image_spec_t)(unsafe.Pointer(&children[0])),
			&csize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(csize)).If(err == errRange)
	})
	if err != nil {
		return nil, nil, err
	}
	defer C.rbd_linked_image_spec_list_cleanup((*C.rbd_linked_image_spec_t)(unsafe.Pointer(&children[0])), csize)

	pools = make([]string, csize)
	images = make([]string, csize)
	for i, child := range children[:csize] {
		pools[i] = C.GoString(child.pool_name)
		images[i] = C.GoString(child.image_name)
	}
	return pools, images, nil
}

// SetSnapByID updates the rbd image (not the Snapshot) such that the snapshot
// is the source of readable data.
//
// Implements:
//
//	int rbd_snap_set_by_id(rbd_image_t image, uint64_t snap_id);
func (image *Image) SetSnapByID(snapID uint64) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ret := C.rbd_snap_set_by_id(image.image, C.uint64_t(snapID))
	return getError(ret)
}
