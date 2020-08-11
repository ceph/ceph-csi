// +build luminous mimic
// +build !nautilus
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

	"github.com/ceph/go-ceph/internal/cutil"
)

// GetParentInfo looks for the parent of the image and stores the pool, name
// and snapshot-name in the byte-arrays that are passed as arguments.
//
// Implements:
//   int rbd_get_parent_info(rbd_image_t image, char *parent_pool_name,
//                           size_t ppool_namelen, char *parent_name,
//                           size_t pnamelen, char *parent_snap_name,
//                           size_t psnap_namelen)
func (image *Image) GetParentInfo(p_pool, p_name, p_snapname []byte) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	ret := C.rbd_get_parent_info(
		image.image,
		(*C.char)(unsafe.Pointer(&p_pool[0])),
		(C.size_t)(len(p_pool)),
		(*C.char)(unsafe.Pointer(&p_name[0])),
		(C.size_t)(len(p_name)),
		(*C.char)(unsafe.Pointer(&p_snapname[0])),
		(C.size_t)(len(p_snapname)))
	if ret == 0 {
		return nil
	} else {
		return rbdError(ret)
	}
}

// ListChildren returns arrays with the pools and names of the images that are
// children of the given image. The index of the pools and images arrays can be
// used to link the two items together.
//
// Implements:
//   ssize_t rbd_list_children(rbd_image_t image, char *pools,
//                             size_t *pools_len,
//                             char *images, size_t *images_len);
func (image *Image) ListChildren() (pools []string, images []string, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, nil, err
	}

	var c_pools_len, c_images_len C.size_t

	ret := C.rbd_list_children(image.image,
		nil, &c_pools_len,
		nil, &c_images_len)
	if ret == 0 {
		return nil, nil, nil
	}
	if ret < 0 && ret != -C.ERANGE {
		return nil, nil, rbdError(ret)
	}

	pools_buf := make([]byte, c_pools_len)
	images_buf := make([]byte, c_images_len)

	ret = C.rbd_list_children(image.image,
		(*C.char)(unsafe.Pointer(&pools_buf[0])),
		&c_pools_len,
		(*C.char)(unsafe.Pointer(&images_buf[0])),
		&c_images_len)
	if ret < 0 {
		return nil, nil, rbdError(ret)
	}

	pools = cutil.SplitSparseBuffer(pools_buf[:c_pools_len])
	images = cutil.SplitSparseBuffer(images_buf[:c_images_len])
	return pools, images, nil
}
