//go:build !(octopus || nautilus) && ceph_preview
// +build !octopus,!nautilus,ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rados/librados.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/rados"
)

// MigrationImageState denotes the current migration status of a given image.
type MigrationImageState int

const (
	// MigrationImageUnknown is the representation of
	// RBD_IMAGE_MIGRATION_STATE_UNKNOWN from librbd.
	MigrationImageUnknown = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_UNKNOWN)
	// MigrationImageError is the representation of
	// RBD_IMAGE_MIGRATION_STATE_ERROR from librbd.
	MigrationImageError = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_ERROR)
	// MigrationImagePreparing is the representation of
	// RBD_IMAGE_MIGRATION_STATE_PREPARING from librbd.
	MigrationImagePreparing = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_PREPARING)
	// MigrationImagePrepared is the representation of
	// RBD_IMAGE_MIGRATION_STATE_PREPARED from librbd.
	MigrationImagePrepared = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_PREPARED)
	// MigrationImageExecuting is the representation of
	// RBD_IMAGE_MIGRATION_STATE_EXECUTING from librbd.
	MigrationImageExecuting = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_EXECUTING)
	// MigrationImageExecuted is the representation of
	// RBD_IMAGE_MIGRATION_STATE_EXECUTED from librbd.
	MigrationImageExecuted = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_EXECUTED)
	// MigrationImageAborting is the representation of
	// RBD_IMAGE_MIGRATION_STATE_ABORTING from librbd.
	MigrationImageAborting = MigrationImageState(C.RBD_IMAGE_MIGRATION_STATE_ABORTING)
)

// MigrationImageStatus provides information about the
// live migration progress of an image.
type MigrationImageStatus struct {
	SourcePoolID        int
	SourcePoolNamespace string
	SourceImageName     string
	SourceImageID       string
	DestPoolID          int
	DestPoolNamespace   string
	DestImageName       string
	DestImageID         string
	State               MigrationImageState
	StateDescription    string
}

// MigrationPrepare prepares a migration
// creating a target image with a link
// to source and making source read-only.
//
// Implements:
//
//	int rbd_migration_prepare(rados_ioctx_t ioctx,
//	                          const char *image_name,
//	                          rados_ioctx_t dest_ioctx,
//	                          const char *dest_image_name,
//	                          rbd_image_options_t opts);
func MigrationPrepare(ioctx *rados.IOContext, sourceImageName string, destIoctx *rados.IOContext, destImageName string, rio *ImageOptions) error {
	cSourceImageName := C.CString(sourceImageName)
	cDestImageName := C.CString(destImageName)
	defer func() {
		C.free(unsafe.Pointer(cSourceImageName))
		C.free(unsafe.Pointer(cDestImageName))
	}()

	ret := C.rbd_migration_prepare(
		cephIoctx(ioctx),
		cSourceImageName,
		cephIoctx(destIoctx),
		cDestImageName,
		C.rbd_image_options_t(rio.options))

	return getError(ret)
}

// MigrationPrepareImport prepares a migration for import
// from a specified source to a new target image.
//
// Implements:
//
//	int rbd_migration_prepare_import(const char *source_spec,
//	                                 rados_ioctx_t dest_ioctx,
//	                                 const char *dest_image_name,
//	                                 rbd_image_options_t opts);
func MigrationPrepareImport(sourceSpec string, ioctx *rados.IOContext, destImageName string, rio *ImageOptions) error {
	cSourceSpec := C.CString(sourceSpec)
	cDestImageName := C.CString(destImageName)
	defer func() {
		C.free(unsafe.Pointer(cSourceSpec))
		C.free(unsafe.Pointer(cDestImageName))
	}()

	ret := C.rbd_migration_prepare_import(
		cSourceSpec,
		cephIoctx(ioctx),
		cDestImageName,
		C.rbd_image_options_t(rio.options))

	return getError(ret)
}

// MigrationExecute starts copying the image blocks
// from the source image to the target image.
//
// Implements:
//
//	int rbd_migration_execute(rados_ioctx_t ioctx,
//	                          const char *image_name);
func MigrationExecute(ioctx *rados.IOContext, name string) error {
	cName := C.CString(name)

	defer func() {
		C.free(unsafe.Pointer(cName))
	}()

	ret := C.rbd_migration_execute(
		cephIoctx(ioctx),
		cName)
	return getError(ret)
}

// MigrationCommit commits a migration after execution
// breaking the relationship of image to the source.
//
// Implements:
//
//	int rbd_migration_commit(rados_ioctx_t ioctx,
//	                         const char *image_name);
func MigrationCommit(ioctx *rados.IOContext, name string) error {
	cName := C.CString(name)

	defer func() {
		C.free(unsafe.Pointer(cName))
	}()

	ret := C.rbd_migration_commit(
		cephIoctx(ioctx),
		cName)
	return getError(ret)
}

// MigrationAbort aborts a migration in progress
// breaking the relationship of image to the source.
//
// Implements:
//
//	int rbd_migration_abort(rados_ioctx_t ioctx,
//	                        const char *image_name);
func MigrationAbort(ioctx *rados.IOContext, name string) error {
	cName := C.CString(name)

	defer func() {
		C.free(unsafe.Pointer(cName))
	}()

	ret := C.rbd_migration_abort(
		cephIoctx(ioctx),
		cName)
	return getError(ret)
}

// MigrationStatus retrieve status of a live migration
// for the specified image.
//
// Implements:
//
//	int rbd_migration_status(rados_ioctx_t ioctx,
//	                         const char *image_name,
//	                         rbd_image_migration_status_t *status,
//	                         size_t status_size);
func MigrationStatus(ioctx *rados.IOContext, name string) (*MigrationImageStatus, error) {
	cName := C.CString(name)

	defer func() {
		C.free(unsafe.Pointer(cName))
	}()

	var status C.rbd_image_migration_status_t
	ret := C.rbd_migration_status(
		cephIoctx(ioctx),
		cName,
		&status,
		C.sizeof_rbd_image_migration_status_t)

	if ret != 0 {
		return nil, getError(ret)
	}

	defer func() {
		C.rbd_migration_status_cleanup(&status)
	}()

	return &MigrationImageStatus{
		SourcePoolID:        int(status.source_pool_id),
		SourcePoolNamespace: C.GoString(status.source_pool_namespace),
		SourceImageName:     C.GoString(status.source_image_name),
		SourceImageID:       C.GoString(status.source_image_id),
		DestPoolID:          int(status.dest_pool_id),
		DestPoolNamespace:   C.GoString(status.dest_pool_namespace),
		DestImageName:       C.GoString(status.dest_image_name),
		DestImageID:         C.GoString(status.dest_image_id),
		State:               MigrationImageState(status.state),
		StateDescription:    C.GoString(status.state_description),
	}, nil

}
