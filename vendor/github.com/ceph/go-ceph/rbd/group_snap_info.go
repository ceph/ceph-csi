//go:build ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#include <errno.h>
#include <stdlib.h>
#include <rbd/librbd.h>

// Types and constants are copied from librbd.h with added "_" as prefix. This
// prevents redefinition of the types on librbd versions that have them
// already.

typedef enum {
  _RBD_GROUP_SNAP_NAMESPACE_TYPE_USER = 0
} _rbd_group_snap_namespace_type_t;

typedef struct {
  char *image_name;
  int64_t pool_id;
  uint64_t snap_id;
} _rbd_group_image_snap_info_t;

typedef struct {
  char *id;
  char *name;
  char *image_snap_name;
  rbd_group_snap_state_t state;
  _rbd_group_snap_namespace_type_t namespace_type;
  size_t image_snaps_count;
  _rbd_group_image_snap_info_t *image_snaps;
} _rbd_group_snap_info2_t;

// rbd_group_snap_get_info_fn matches the rbd_group_snap_get_info function signature.
typedef int(*rbd_group_snap_get_info_fn)(rados_ioctx_t group_p,
                                         const char *group_name,
                                         const char *snap_name,
                                         _rbd_group_snap_info2_t *snaps);

// rbd_group_snap_get_info_dlsym take *fn as rbd_group_snap_get_info_fn and
// calls the dynamically loaded rbd_group_snap_get_info function passed as 1st
// argument.
static inline int rbd_group_snap_get_info_dlsym(void *fn,
                                                rados_ioctx_t group_p,
                                                const char *group_name,
                                                const char *snap_name,
                                                _rbd_group_snap_info2_t *snaps) {
  // cast function pointer fn to rbd_group_snap_get_info and call the function
  return ((rbd_group_snap_get_info_fn) fn)(group_p, group_name, snap_name, snaps);
}

// rbd_group_snap_get_info_cleanup_fn matches the rbd_group_snap_get_info_cleanup function signature.
typedef int(*rbd_group_snap_get_info_cleanup_fn)(_rbd_group_snap_info2_t *snaps);

// rbd_group_snap_get_info_cleanup_dlsym take *fn as rbd_group_snap_get_info_cleanup_fn and
// calls the dynamically loaded rbd_group_snap_get_info_cleanup function passed as 1st
// argument.
static inline int rbd_group_snap_get_info_cleanup_dlsym(void *fn,
                                                _rbd_group_snap_info2_t *snaps) {
  // cast function pointer fn to rbd_group_snap_get_info_cleanup and call the function
  return ((rbd_group_snap_get_info_cleanup_fn) fn)(snaps);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ceph/go-ceph/internal/cutil"
	"github.com/ceph/go-ceph/internal/dlsym"
	"github.com/ceph/go-ceph/rados"
)

type imgSnapInfoArray [cutil.MaxIdx]C._rbd_group_image_snap_info_t

var (
	rbdGroupGetSnapInfoOnce sync.Once
	rbdGroupGetSnapInfo     unsafe.Pointer
	rbdGroupGetSnapInfoErr  error

	rbdGroupSnapGetInfoCleanupOnce sync.Once
	rbdGroupSnapGetInfoCleanup     unsafe.Pointer
	rbdGroupSnapGetInfoCleanupErr  error
)

// GroupSnapGetInfo returns a slice of RBD image snapshots that are part of a
// group snapshot.
//
// Implements:
//
//	int rbd_group_snap_get_info(rados_ioctx_t group_p,
//	                        const char *group_name,
//	                        const char *snap_name,
//	                        rbd_group_snap_info2_t *snaps);
func GroupSnapGetInfo(ioctx *rados.IOContext, group, snap string) (GroupSnapInfo, error) {
	rbdGroupGetSnapInfoOnce.Do(func() {
		rbdGroupGetSnapInfo, rbdGroupGetSnapInfoErr = dlsym.LookupSymbol("rbd_group_snap_get_info")
	})

	if rbdGroupGetSnapInfoErr != nil {
		return GroupSnapInfo{}, fmt.Errorf("%w: %w", ErrNotImplemented, rbdGroupGetSnapInfoErr)
	}

	rbdGroupSnapGetInfoCleanupOnce.Do(func() {
		rbdGroupSnapGetInfoCleanup, rbdGroupSnapGetInfoCleanupErr = dlsym.LookupSymbol("rbd_group_snap_get_info_cleanup")
	})

	if rbdGroupSnapGetInfoCleanupErr != nil {
		return GroupSnapInfo{}, fmt.Errorf("%w: %w", ErrNotImplemented, rbdGroupSnapGetInfoCleanupErr)
	}

	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))
	cSnapName := C.CString(snap)
	defer C.free(unsafe.Pointer(cSnapName))

	cSnapInfo := C._rbd_group_snap_info2_t{}

	ret := C.rbd_group_snap_get_info_dlsym(
		rbdGroupGetSnapInfo,
		cephIoctx(ioctx),
		cGroupName,
		cSnapName,
		&cSnapInfo)
	err := getErrorIfNegative(ret)
	if err != nil {
		return GroupSnapInfo{}, err
	}

	snapCount := uint64(cSnapInfo.image_snaps_count)

	snapInfo := GroupSnapInfo{
		ID:        C.GoString(cSnapInfo.id),
		Name:      C.GoString(cSnapInfo.name),
		SnapName:  C.GoString(cSnapInfo.image_snap_name),
		State:     GroupSnapState(cSnapInfo.state),
		Snapshots: make([]GroupSnap, snapCount),
	}

	imgSnaps := (*imgSnapInfoArray)(unsafe.Pointer(cSnapInfo.image_snaps))[0:snapCount]

	for i, imgSnap := range imgSnaps {
		snapInfo.Snapshots[i].Name = C.GoString(imgSnap.image_name)
		snapInfo.Snapshots[i].PoolID = uint64(imgSnap.pool_id)
		snapInfo.Snapshots[i].SnapID = uint64(imgSnap.snap_id)
	}

	// free C memory allocated by C.rbd_group_snap_get_info call
	C.rbd_group_snap_get_info_cleanup_dlsym(rbdGroupSnapGetInfoCleanup, &cSnapInfo)
	return snapInfo, nil
}
