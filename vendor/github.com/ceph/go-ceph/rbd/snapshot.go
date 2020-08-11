package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"
)

// Snapshot represents a snapshot on a particular rbd image.
type Snapshot struct {
	image *Image
	name  string
}

// CreateSnapshot returns a new Snapshot objects after creating
// a snapshot of the rbd image.
//
// Implements:
//  int rbd_snap_create(rbd_image_t image, const char *snapname);
func (image *Image) CreateSnapshot(snapname string) (*Snapshot, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	c_snapname := C.CString(snapname)
	defer C.free(unsafe.Pointer(c_snapname))

	ret := C.rbd_snap_create(image.image, c_snapname)
	if ret < 0 {
		return nil, rbdError(ret)
	}

	return &Snapshot{
		image: image,
		name:  snapname,
	}, nil
}

// validate the attributes listed in the req bitmask, and return an error in
// case the attribute is not set
// Calls snapshot.image.validate(req) to validate the image attributes.
func (snapshot *Snapshot) validate(req uint32) error {
	if hasBit(req, snapshotNeedsName) && snapshot.name == "" {
		return ErrSnapshotNoName
	} else if snapshot.image != nil {
		return snapshot.image.validate(req)
	}

	return nil
}

// GetSnapshot constructs a snapshot object for the image given
// the snap name. It does not validate that this snapshot exists.
func (image *Image) GetSnapshot(snapname string) *Snapshot {
	return &Snapshot{
		image: image,
		name:  snapname,
	}
}

// Remove the snapshot from the connected rbd image.
//
// Implements:
//  int rbd_snap_remove(rbd_image_t image, const char *snapname);
func (snapshot *Snapshot) Remove() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	c_snapname := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(c_snapname))

	return getError(C.rbd_snap_remove(snapshot.image.image, c_snapname))
}

// Rollback the image to the snapshot.
//
// Implements:
//  int rbd_snap_rollback(rbd_image_t image, const char *snapname);
func (snapshot *Snapshot) Rollback() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	c_snapname := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(c_snapname))

	return getError(C.rbd_snap_rollback(snapshot.image.image, c_snapname))
}

// Protect a snapshot from unwanted deletion.
//
// Implements:
//  int rbd_snap_protect(rbd_image_t image, const char *snap_name);
func (snapshot *Snapshot) Protect() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	c_snapname := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(c_snapname))

	return getError(C.rbd_snap_protect(snapshot.image.image, c_snapname))
}

// Unprotect stops protecting the snapshot.
//
// Implements:
//  int rbd_snap_unprotect(rbd_image_t image, const char *snap_name);
func (snapshot *Snapshot) Unprotect() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	c_snapname := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(c_snapname))

	return getError(C.rbd_snap_unprotect(snapshot.image.image, c_snapname))
}

// IsProtected returns true if the snapshot is currently protected.
//
// Implements:
//  int rbd_snap_is_protected(rbd_image_t image, const char *snap_name,
//               int *is_protected);
func (snapshot *Snapshot) IsProtected() (bool, error) {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return false, err
	}

	var c_is_protected C.int

	c_snapname := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(c_snapname))

	ret := C.rbd_snap_is_protected(snapshot.image.image, c_snapname,
		&c_is_protected)
	if ret < 0 {
		return false, rbdError(ret)
	}

	return c_is_protected != 0, nil
}

// Set updates the rbd image (not the Snapshot) such that the snapshot
// is the source of readable data.
//
// Implements:
//  int rbd_snap_set(rbd_image_t image, const char *snapname);
func (snapshot *Snapshot) Set() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	c_snapname := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(c_snapname))

	return getError(C.rbd_snap_set(snapshot.image.image, c_snapname))
}
