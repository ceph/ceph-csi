//
// Ceph Mimic introduced rbd_snap_get_namespace_type().

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
)

// SnapNamespaceType indicates the namespace to which the snapshot belongs to.
type SnapNamespaceType C.rbd_snap_namespace_type_t

const (
	// SnapNamespaceTypeUser indicates that the snapshot belongs to user namespace.
	SnapNamespaceTypeUser = SnapNamespaceType(C.RBD_SNAP_NAMESPACE_TYPE_USER)

	// SnapNamespaceTypeGroup indicates that the snapshot belongs to group namespace.
	// Such snapshots will have associated group information.
	SnapNamespaceTypeGroup = SnapNamespaceType(C.RBD_SNAP_NAMESPACE_TYPE_GROUP)

	// SnapNamespaceTypeTrash indicates that the snapshot belongs to trash namespace.
	SnapNamespaceTypeTrash = SnapNamespaceType(C.RBD_SNAP_NAMESPACE_TYPE_TRASH)
)

// GetSnapNamespaceType gets the type of namespace to which the snapshot belongs to,
// returns error on failure.
//
// Implements:
//
//	int rbd_snap_get_namespace_type(rbd_image_t image, uint64_t snap_id, rbd_snap_namespace_type_t *namespace_type)
func (image *Image) GetSnapNamespaceType(snapID uint64) (SnapNamespaceType, error) {
	var nsType SnapNamespaceType

	if err := image.validate(imageIsOpen); err != nil {
		return nsType, err
	}

	ret := C.rbd_snap_get_namespace_type(image.image,
		C.uint64_t(snapID),
		(*C.rbd_snap_namespace_type_t)(&nsType))
	return nsType, getError(ret)
}

// GetSnapTrashNamespace returns the original name of the snapshot which was
// moved to the Trash. The caller should make sure that the snapshot ID passed in this
// function belongs to a snapshot already in the Trash.
//
// Implements:
//
//	int rbd_snap_get_trash_namespace(rbd_image_t image, uint64_t snap_id, char *original_name, size_t max_length)
func (image *Image) GetSnapTrashNamespace(snapID uint64) (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}

	var (
		buf []byte
		err error
	)
	retry.WithSizes(4096, 262144, func(length int) retry.Hint {
		cLength := C.size_t(length)
		buf = make([]byte, cLength)
		ret := C.rbd_snap_get_trash_namespace(image.image,
			C.uint64_t(snapID),
			(*C.char)(unsafe.Pointer(&buf[0])),
			cLength)
		err = getError(ret)
		return retry.Size(int(cLength)).If(err == errRange)
	})

	if err != nil {
		return "", err
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
}
