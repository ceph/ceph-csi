package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"
)

// Rename a snapshot.
//
// Implements:
//
//	int rbd_snap_rename(rbd_image_t image, const char *snapname,
//				 const char* dstsnapsname);
func (snapshot *Snapshot) Rename(destName string) error {
	if err := snapshot.validate(imageNeedsIOContext | imageIsOpen | imageNeedsName | snapshotNeedsName); err != nil {
		return err
	}

	cSrcName := C.CString(snapshot.name)
	cDestName := C.CString(destName)
	defer C.free(unsafe.Pointer(cSrcName))
	defer C.free(unsafe.Pointer(cDestName))

	err := C.rbd_snap_rename(snapshot.image.image, cSrcName, cDestName)
	if err != 0 {
		return getError(err)
	}

	snapshot.name = destName
	return nil
}
