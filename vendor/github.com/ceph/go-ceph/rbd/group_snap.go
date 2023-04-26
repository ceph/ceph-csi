package rbd

/*
#cgo LDFLAGS: -lrbd
#include <errno.h>
#include <stdlib.h>
#include <rbd/librbd.h>

extern int snapRollbackCallback(uint64_t, uint64_t, uintptr_t);

// inline wrapper to cast uintptr_t to void*
static inline int wrap_rbd_group_snap_rollback_with_progress(
		rados_ioctx_t group_p, const char *group_name,
		const char *snap_name, uintptr_t arg) {
	return rbd_group_snap_rollback_with_progress(
		group_p, group_name, snap_name, (librbd_progress_fn_t)snapRollbackCallback, (void*)arg);
};
*/
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/callbacks"
	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// GroupSnapCreate will create a group snapshot.
//
// Implements:
//
//	int rbd_group_snap_create(rados_ioctx_t group_p,
//	                          const char *group_name,
//	                          const char *snap_name);
func GroupSnapCreate(ioctx *rados.IOContext, group, snap string) error {
	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))
	cSnapName := C.CString(snap)
	defer C.free(unsafe.Pointer(cSnapName))

	ret := C.rbd_group_snap_create(cephIoctx(ioctx), cGroupName, cSnapName)
	return getError(ret)
}

// GroupSnapRemove removes an existing group snapshot.
//
// Implements:
//
//	int rbd_group_snap_remove(rados_ioctx_t group_p,
//	                          const char *group_name,
//	                          const char *snap_name);
func GroupSnapRemove(ioctx *rados.IOContext, group, snap string) error {
	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))
	cSnapName := C.CString(snap)
	defer C.free(unsafe.Pointer(cSnapName))

	ret := C.rbd_group_snap_remove(cephIoctx(ioctx), cGroupName, cSnapName)
	return getError(ret)
}

// GroupSnapRename will rename an existing group snapshot.
//
// Implements:
//
//	int rbd_group_snap_rename(rados_ioctx_t group_p,
//	                          const char *group_name,
//	                          const char *old_snap_name,
//	                          const char *new_snap_name);
func GroupSnapRename(ioctx *rados.IOContext, group, src, dest string) error {
	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))
	cOldSnapName := C.CString(src)
	defer C.free(unsafe.Pointer(cOldSnapName))
	cNewSnapName := C.CString(dest)
	defer C.free(unsafe.Pointer(cNewSnapName))

	ret := C.rbd_group_snap_rename(
		cephIoctx(ioctx), cGroupName, cOldSnapName, cNewSnapName)
	return getError(ret)
}

// GroupSnapState represents the state of a group snapshot in GroupSnapInfo.
type GroupSnapState int

const (
	// GroupSnapStateIncomplete is equivalent to RBD_GROUP_SNAP_STATE_INCOMPLETE.
	GroupSnapStateIncomplete = GroupSnapState(C.RBD_GROUP_SNAP_STATE_INCOMPLETE)
	// GroupSnapStateComplete is equivalent to RBD_GROUP_SNAP_STATE_COMPLETE.
	GroupSnapStateComplete = GroupSnapState(C.RBD_GROUP_SNAP_STATE_COMPLETE)
)

// GroupSnapInfo values are returned by GroupSnapList, representing the
// snapshots that are part of an rbd group.
type GroupSnapInfo struct {
	Name  string
	State GroupSnapState
}

// GroupSnapList returns a slice of snapshots in a group.
//
// Implements:
//
//	int rbd_group_snap_list(rados_ioctx_t group_p,
//	                        const char *group_name,
//	                        rbd_group_snap_info_t *snaps,
//	                        size_t group_snap_info_size,
//	                        size_t *num_entries);
func GroupSnapList(ioctx *rados.IOContext, group string) ([]GroupSnapInfo, error) {
	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))

	var (
		cSnaps []C.rbd_group_snap_info_t
		cSize  C.size_t
		err    error
	)
	retry.WithSizes(1024, 262144, func(size int) retry.Hint {
		cSize = C.size_t(size)
		cSnaps = make([]C.rbd_group_snap_info_t, cSize)
		ret := C.rbd_group_snap_list(
			cephIoctx(ioctx),
			cGroupName,
			(*C.rbd_group_snap_info_t)(unsafe.Pointer(&cSnaps[0])),
			C.sizeof_rbd_group_snap_info_t,
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})

	if err != nil {
		return nil, err
	}

	snaps := make([]GroupSnapInfo, cSize)
	for i := range snaps {
		snaps[i].Name = C.GoString(cSnaps[i].name)
		snaps[i].State = GroupSnapState(cSnaps[i].state)
	}

	// free C memory allocated by C.rbd_group_snap_list call
	ret := C.rbd_group_snap_list_cleanup(
		(*C.rbd_group_snap_info_t)(unsafe.Pointer(&cSnaps[0])),
		C.sizeof_rbd_group_snap_info_t,
		cSize)
	return snaps, getError(ret)
}

// GroupSnapRollback will roll back the images in the group to that of the
// given snapshot.
//
// Implements:
//
//	int rbd_group_snap_rollback(rados_ioctx_t group_p,
//	                            const char *group_name,
//	                            const char *snap_name);
func GroupSnapRollback(ioctx *rados.IOContext, group, snap string) error {
	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))
	cSnapName := C.CString(snap)
	defer C.free(unsafe.Pointer(cSnapName))

	ret := C.rbd_group_snap_rollback(cephIoctx(ioctx), cGroupName, cSnapName)
	return getError(ret)
}

// GroupSnapRollbackCallback defines the function signature needed for the
// GroupSnapRollbackWithProgress callback.
//
// This callback will be called by GroupSnapRollbackWithProgress when it
// wishes to report progress rolling back a group snapshot.
type GroupSnapRollbackCallback func(uint64, uint64, interface{}) int

var groupSnapRollbackCallbacks = callbacks.New()

// GroupSnapRollbackWithProgress will roll back the images in the group
// to that of given snapshot. The given progress callback will be called
// to report on the progress of the snapshot rollback.
//
// Implements:
//
//	int rbd_group_snap_rollback_with_progress(rados_ioctx_t group_p,
//	                                          const char *group_name,
//	                                          const char *snap_name,
//	                                          librbd_progress_fn_t cb,
//	                                          void *cbdata);
func GroupSnapRollbackWithProgress(
	ioctx *rados.IOContext, group, snap string,
	cb GroupSnapRollbackCallback, data interface{}) error {
	// the provided callback must be a real function
	if cb == nil {
		return rbdError(C.EINVAL)
	}

	cGroupName := C.CString(group)
	defer C.free(unsafe.Pointer(cGroupName))
	cSnapName := C.CString(snap)
	defer C.free(unsafe.Pointer(cSnapName))

	ctx := gsnapRollbackCallbackCtx{
		callback: cb,
		data:     data,
	}
	cbIndex := groupSnapRollbackCallbacks.Add(ctx)
	defer diffIterateCallbacks.Remove(cbIndex)

	ret := C.wrap_rbd_group_snap_rollback_with_progress(
		cephIoctx(ioctx),
		cGroupName,
		cSnapName,
		C.uintptr_t(cbIndex))

	return getError(ret)
}

type gsnapRollbackCallbackCtx struct {
	callback GroupSnapRollbackCallback
	data     interface{}
}

//export snapRollbackCallback
func snapRollbackCallback(
	offset, total C.uint64_t, index uintptr) C.int {

	v := groupSnapRollbackCallbacks.Lookup(index)
	ctx := v.(gsnapRollbackCallbackCtx)
	return C.int(ctx.callback(uint64(offset), uint64(total), ctx.data))
}
