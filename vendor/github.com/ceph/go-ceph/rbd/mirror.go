// +build !nautilus

// Initially, we're only providing mirroring related functions for octopus as
// that version of ceph deprecated a number of the functions in nautilus. If
// you need mirroring on an earlier supported version of ceph please file an
// issue in our tracker.

package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// MirrorMode is used to indicate an approach used for RBD mirroring.
type MirrorMode int64

const (
	// MirrorModeDisabled disables mirroring.
	MirrorModeDisabled = MirrorMode(C.RBD_MIRROR_MODE_DISABLED)
	// MirrorModeImage enables mirroring on a per-image basis.
	MirrorModeImage = MirrorMode(C.RBD_MIRROR_MODE_IMAGE)
	// MirrorModePool enables mirroring on all journaled images.
	MirrorModePool = MirrorMode(C.RBD_MIRROR_MODE_POOL)
)

// ImageMirrorMode is used to indicate the mirroring approach for an RBD image.
type ImageMirrorMode int64

const (
	// ImageMirrorModeJournal uses journaling to propagate RBD images between
	// ceph clusters.
	ImageMirrorModeJournal = ImageMirrorMode(C.RBD_MIRROR_IMAGE_MODE_JOURNAL)
	// ImageMirrorModeSnapshot uses snapshots to propagate RBD images between
	// ceph clusters.
	ImageMirrorModeSnapshot = ImageMirrorMode(C.RBD_MIRROR_IMAGE_MODE_SNAPSHOT)
)

// SetMirrorMode is used to enable or disable pool level mirroring with either
// an automatic or per-image behavior.
//
// Implements:
//  int rbd_mirror_mode_set(rados_ioctx_t io_ctx,
//                          rbd_mirror_mode_t mirror_mode);
func SetMirrorMode(ioctx *rados.IOContext, mode MirrorMode) error {
	ret := C.rbd_mirror_mode_set(
		cephIoctx(ioctx),
		C.rbd_mirror_mode_t(mode))
	return getError(ret)
}

// GetMirrorMode is used to fetch the current mirroring mode for a pool.
//
// Implements:
//  int rbd_mirror_mode_get(rados_ioctx_t io_ctx,
//                          rbd_mirror_mode_t *mirror_mode);
func GetMirrorMode(ioctx *rados.IOContext) (MirrorMode, error) {
	var mode C.rbd_mirror_mode_t

	ret := C.rbd_mirror_mode_get(
		cephIoctx(ioctx),
		&mode)
	if err := getError(ret); err != nil {
		return MirrorModeDisabled, err
	}
	return MirrorMode(mode), nil
}

// MirrorEnable will enable mirroring for an image using the specified mode.
//
// Implements:
//  int rbd_mirror_image_enable2(rbd_image_t image,
//                               rbd_mirror_image_mode_t mode);
func (image *Image) MirrorEnable(mode ImageMirrorMode) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_enable2(image.image, C.rbd_mirror_image_mode_t(mode))
	return getError(ret)
}

// MirrorDisable will disable mirroring for the image.
//
// Implements:
//  int rbd_mirror_image_disable(rbd_image_t image, bool force);
func (image *Image) MirrorDisable(force bool) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_disable(image.image, C.bool(force))
	return getError(ret)
}

// MirrorPromote will promote the image to primary status.
//
// Implements:
//  int rbd_mirror_image_promote(rbd_image_t image, bool force);
func (image *Image) MirrorPromote(force bool) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_promote(image.image, C.bool(force))
	return getError(ret)
}

// MirrorDemote will demote the image to secondary status.
//
// Implements:
//  int rbd_mirror_image_demote(rbd_image_t image);
func (image *Image) MirrorDemote() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_demote(image.image)
	return getError(ret)
}

// MirrorResync is used to manually resolve split-brain status by triggering
// resynchronization.
//
// Implements:
//  int rbd_mirror_image_resync(rbd_image_t image);
func (image *Image) MirrorResync() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_resync(image.image)
	return getError(ret)
}

// MirrorInstanceID returns a string naming the instance id for the image.
//
// Implements:
//  int rbd_mirror_image_get_instance_id(rbd_image_t image,
//                                       char *instance_id,
//                                       size_t *id_max_length);
func (image *Image) MirrorInstanceID() (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}
	var (
		err   error
		buf   []byte
		cSize C.size_t
	)
	retry.WithSizes(1024, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_mirror_image_get_instance_id(
			image.image,
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	return string(buf[:cSize]), nil
}

// MirrorImageState represents the mirroring state of a RBD image.
type MirrorImageState C.rbd_mirror_image_state_t

const (
	// MirrorImageDisabling is the representation of
	// RBD_MIRROR_IMAGE_DISABLING from librbd.
	MirrorImageDisabling = MirrorImageState(C.RBD_MIRROR_IMAGE_DISABLING)
	// MirrorImageEnabled is the representation of
	// RBD_MIRROR_IMAGE_ENABLED from librbd.
	MirrorImageEnabled = MirrorImageState(C.RBD_MIRROR_IMAGE_ENABLED)
	// MirrorImageDisabled is the representation of
	// RBD_MIRROR_IMAGE_DISABLED from librbd.
	MirrorImageDisabled = MirrorImageState(C.RBD_MIRROR_IMAGE_DISABLED)
)

// MirrorImageInfo represents the mirroring status information of a RBD image.
type MirrorImageInfo struct {
	GlobalID string
	State    MirrorImageState
	Primary  bool
}

// GetMirrorImageInfo fetches the mirroring status information of a RBD image.
//
// Implements:
//  int rbd_mirror_image_get_info(rbd_image_t image,
//                                rbd_mirror_image_info_t *mirror_image_info,
//                                size_t info_size)
func (image *Image) GetMirrorImageInfo() (*MirrorImageInfo, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var cInfo C.rbd_mirror_image_info_t

	ret := C.rbd_mirror_image_get_info(
		image.image,
		&cInfo,
		C.sizeof_rbd_mirror_image_info_t)
	if ret < 0 {
		return nil, getError(ret)
	}

	mii := MirrorImageInfo{
		GlobalID: C.GoString(cInfo.global_id),
		State:    MirrorImageState(cInfo.state),
		Primary:  bool(cInfo.primary),
	}

	// free C memory allocated by C.rbd_mirror_image_get_info call
	C.rbd_mirror_image_get_info_cleanup(&cInfo)
	return &mii, nil
}

// GetImageMirrorMode fetches the mirroring approach for an RBD image.
//
// Implements:
//  int rbd_mirror_image_get_mode(rbd_image_t image, rbd_mirror_image_mode_t *mode);
func (image *Image) GetImageMirrorMode() (ImageMirrorMode, error) {
	var mode C.rbd_mirror_image_mode_t
	if err := image.validate(imageIsOpen); err != nil {
		return ImageMirrorMode(mode), err
	}

	ret := C.rbd_mirror_image_get_mode(image.image, &mode)
	return ImageMirrorMode(mode), getError(ret)
}
