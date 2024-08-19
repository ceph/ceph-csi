//go:build ceph_preview

package rbd

/*
#cgo LDFLAGS: -lrbd
#include <errno.h>
#include <stdlib.h>
#include <rados/librados.h>
#include <rbd/librbd.h>

// rbd_clone4_fn matches the rbd_clone4 function signature.
typedef int(*rbd_clone4_fn)(rados_ioctx_t p_ioctx, const char *p_name,
                            uint64_t p_snap_id, rados_ioctx_t c_ioctx,
			    const char *c_name, rbd_image_options_t c_opts);

// rbd_clone4_dlsym take *fn as rbd_clone4_fn and calls the dynamically loaded
// rbd_clone4 function passed as 1st argument.
static inline int rbd_clone4_dlsym(void *fn, rados_ioctx_t p_ioctx,
				   const char *p_name, uint64_t p_snap_id,
				   rados_ioctx_t c_ioctx, const char *c_name,
                                   rbd_image_options_t c_opts) {
  // cast function pointer fn to rbd_clone4 and call the function
  return ((rbd_clone4_fn) fn)(p_ioctx, p_name, p_snap_id, c_ioctx, c_name, c_opts);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ceph/go-ceph/internal/dlsym"
	"github.com/ceph/go-ceph/rados"
)

var (
	rbdClone4Once sync.Once
	rbdClone4     unsafe.Pointer
	rbdClone4Err  error
)

// CloneImageByID creates a clone of the image from a snapshot with the given
// ID in the provided io-context with the given name and image options.
//
// Implements:
//
//	int rbd_clone4(rados_ioctx_t p_ioctx, const char *p_name,
//	               uint64_t p_snap_id, rados_ioctx_t c_ioctx,
//	               const char *c_name, rbd_image_options_t c_opts);
func CloneImageByID(ioctx *rados.IOContext, parentName string, snapID uint64,
	destctx *rados.IOContext, name string, rio *ImageOptions) error {
	if rio == nil {
		return rbdError(C.EINVAL)
	}

	rbdClone4Once.Do(func() {
		rbdClone4, rbdClone4Err = dlsym.LookupSymbol("rbd_clone4")
	})

	if rbdClone4Err != nil {
		return fmt.Errorf("%w: %w", ErrNotImplemented, rbdClone4Err)
	}

	cParentName := C.CString(parentName)
	defer C.free(unsafe.Pointer(cParentName))
	cCloneName := C.CString(name)
	defer C.free(unsafe.Pointer(cCloneName))

	// call rbd_clone4_dlsym with the function pointer to rbd_clone4 as 1st
	// argument
	ret := C.rbd_clone4_dlsym(
		rbdClone4,
		cephIoctx(ioctx),
		cParentName,
		C.uint64_t(snapID),
		cephIoctx(destctx),
		cCloneName,
		C.rbd_image_options_t(rio.options))

	return getError(ret)
}
