package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	ts "github.com/ceph/go-ceph/internal/timespec"
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
//
//	int rbd_snap_create(rbd_image_t image, const char *snapname);
func (image *Image) CreateSnapshot(snapname string) (*Snapshot, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	cSnapName := C.CString(snapname)
	defer C.free(unsafe.Pointer(cSnapName))

	ret := C.rbd_snap_create(image.image, cSnapName)
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
//
//	int rbd_snap_remove(rbd_image_t image, const char *snapname);
func (snapshot *Snapshot) Remove() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	cSnapName := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(cSnapName))

	return getError(C.rbd_snap_remove(snapshot.image.image, cSnapName))
}

// Rollback the image to the snapshot.
//
// Implements:
//
//	int rbd_snap_rollback(rbd_image_t image, const char *snapname);
func (snapshot *Snapshot) Rollback() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	cSnapName := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(cSnapName))

	return getError(C.rbd_snap_rollback(snapshot.image.image, cSnapName))
}

// Protect a snapshot from unwanted deletion.
//
// Implements:
//
//	int rbd_snap_protect(rbd_image_t image, const char *snap_name);
func (snapshot *Snapshot) Protect() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	cSnapName := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(cSnapName))

	return getError(C.rbd_snap_protect(snapshot.image.image, cSnapName))
}

// Unprotect stops protecting the snapshot.
//
// Implements:
//
//	int rbd_snap_unprotect(rbd_image_t image, const char *snap_name);
func (snapshot *Snapshot) Unprotect() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	cSnapName := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(cSnapName))

	return getError(C.rbd_snap_unprotect(snapshot.image.image, cSnapName))
}

// IsProtected returns true if the snapshot is currently protected.
//
// Implements:
//
//	int rbd_snap_is_protected(rbd_image_t image, const char *snap_name,
//	             int *is_protected);
func (snapshot *Snapshot) IsProtected() (bool, error) {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return false, err
	}

	var cIsProtected C.int

	cSnapName := C.CString(snapshot.name)
	defer C.free(unsafe.Pointer(cSnapName))

	ret := C.rbd_snap_is_protected(snapshot.image.image, cSnapName,
		&cIsProtected)
	if ret < 0 {
		return false, rbdError(ret)
	}

	return cIsProtected != 0, nil
}

// Set updates the rbd image (not the Snapshot) such that the snapshot is the
// source of readable data.
//
// Deprecated: use the SetSnapshot method of the Image type instead
//
// Implements:
//
//	int rbd_snap_set(rbd_image_t image, const char *snapname);
func (snapshot *Snapshot) Set() error {
	if err := snapshot.validate(snapshotNeedsName | imageIsOpen); err != nil {
		return err
	}

	return snapshot.image.SetSnapshot(snapshot.name)
}

// GetSnapTimestamp returns the timestamp of a snapshot for an image.
// For a non-existing snap ID, GetSnapTimestamp() may trigger an assertion
// and crash in the ceph library.
// Check https://tracker.ceph.com/issues/47287 for details.
//
// Implements:
//
//	int rbd_snap_get_timestamp(rbd_image_t image, uint64_t snap_id, struct timespec *timestamp)
func (image *Image) GetSnapTimestamp(snapID uint64) (Timespec, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return Timespec{}, err
	}

	var cts C.struct_timespec

	ret := C.rbd_snap_get_timestamp(image.image, C.uint64_t(snapID), &cts)
	if ret < 0 {
		return Timespec{}, getError(ret)
	}

	return Timespec(ts.CStructToTimespec(ts.CTimespecPtr(&cts))), nil
}
