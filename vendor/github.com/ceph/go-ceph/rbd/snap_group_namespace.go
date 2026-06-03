package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
import "C"

// SnapGroupNamespace provides details about a single snapshot that was taken
// as part of an RBD group.
type SnapGroupNamespace struct {
	Pool          uint64
	GroupName     string
	GroupSnapName string
}

// GetSnapGroupNamespace returns the SnapGroupNamespace of the snapshot which
// is part of a group. The caller should make sure that the snapshot ID passed
// in this function belongs to a snapshot that was taken as part of a group
// snapshot.
//
// Implements:
//
//		int rbd_snap_get_group_namespace(rbd_image_t image, uint64_t snap_id,
//	                                      rbd_snap_group_namespace_t *group_snap,
//	                                      size_t group_snap_size)
func (image *Image) GetSnapGroupNamespace(snapID uint64) (*SnapGroupNamespace, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var (
		err error
		sgn C.rbd_snap_group_namespace_t
	)

	ret := C.rbd_snap_get_group_namespace(image.image,
		C.uint64_t(snapID),
		&sgn,
		C.sizeof_rbd_snap_group_namespace_t)
	err = getError(ret)
	if err != nil {
		return nil, err
	}

	defer C.rbd_snap_group_namespace_cleanup(&sgn, C.sizeof_rbd_snap_group_namespace_t)

	return &SnapGroupNamespace{
		Pool:          uint64(sgn.group_pool),
		GroupName:     C.GoString(sgn.group_name),
		GroupSnapName: C.GoString(sgn.group_snap_name),
	}, nil
}
