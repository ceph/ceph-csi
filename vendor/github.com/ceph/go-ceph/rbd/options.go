package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	// RBD image options.

	// ImageOptionFormat is the representation of RBD_IMAGE_OPTION_FORMAT from
	// librbd
	ImageOptionFormat = C.RBD_IMAGE_OPTION_FORMAT
	// ImageOptionFeatures is the representation of RBD_IMAGE_OPTION_FEATURES
	// from librbd
	ImageOptionFeatures = C.RBD_IMAGE_OPTION_FEATURES
	// ImageOptionOrder is the representation of RBD_IMAGE_OPTION_ORDER from
	// librbd
	ImageOptionOrder = C.RBD_IMAGE_OPTION_ORDER
	// ImageOptionStripeUnit is the representation of
	// RBD_IMAGE_OPTION_STRIPE_UNIT from librbd
	ImageOptionStripeUnit = C.RBD_IMAGE_OPTION_STRIPE_UNIT
	// ImageOptionStripeCount is the representation of
	// RBD_IMAGE_OPTION_STRIPE_COUNT from librbd
	ImageOptionStripeCount = C.RBD_IMAGE_OPTION_STRIPE_COUNT
	// ImageOptionJournalOrder is the representation of
	// RBD_IMAGE_OPTION_JOURNAL_ORDER from librbd
	ImageOptionJournalOrder = C.RBD_IMAGE_OPTION_JOURNAL_ORDER
	// ImageOptionJournalSplayWidth is the representation of
	// RBD_IMAGE_OPTION_JOURNAL_SPLAY_WIDTH from librbd
	ImageOptionJournalSplayWidth = C.RBD_IMAGE_OPTION_JOURNAL_SPLAY_WIDTH
	// ImageOptionJournalPool is the representation of
	// RBD_IMAGE_OPTION_JOURNAL_POOL from librbd
	ImageOptionJournalPool = C.RBD_IMAGE_OPTION_JOURNAL_POOL
	// ImageOptionFeaturesSet is the representation of
	// RBD_IMAGE_OPTION_FEATURES_SET from librbd
	ImageOptionFeaturesSet = C.RBD_IMAGE_OPTION_FEATURES_SET
	// ImageOptionFeaturesClear is the representation of
	// RBD_IMAGE_OPTION_FEATURES_CLEAR from librbd
	ImageOptionFeaturesClear = C.RBD_IMAGE_OPTION_FEATURES_CLEAR
	// ImageOptionDataPool is the representation of RBD_IMAGE_OPTION_DATA_POOL
	// from librbd
	ImageOptionDataPool = C.RBD_IMAGE_OPTION_DATA_POOL
	// ImageOptionFlatten is the representation of RBD_IMAGE_OPTION_FLATTEN
	// from librbd
	ImageOptionFlatten = C.RBD_IMAGE_OPTION_FLATTEN
	// ImageOptionCloneFormat is the representation of
	// RBD_IMAGE_OPTION_CLONE_FORMAT from librbd
	ImageOptionCloneFormat = C.RBD_IMAGE_OPTION_CLONE_FORMAT

	// RbdImageOptionFormat deprecated alias for ImageOptionFormat
	RbdImageOptionFormat = ImageOptionFormat
	// RbdImageOptionFeatures deprecated alias for ImageOptionFeatures
	RbdImageOptionFeatures = ImageOptionFeatures
	// RbdImageOptionOrder deprecated alias for ImageOptionOrder
	RbdImageOptionOrder = ImageOptionOrder
	// RbdImageOptionStripeUnit deprecated alias for ImageOptionStripeUnit
	RbdImageOptionStripeUnit = ImageOptionStripeUnit
	// RbdImageOptionStripeCount deprecated alias for ImageOptionStripeCount
	RbdImageOptionStripeCount = ImageOptionStripeCount
	// RbdImageOptionJournalOrder deprecated alias for ImageOptionJournalOrder
	RbdImageOptionJournalOrder = ImageOptionJournalOrder
	// RbdImageOptionJournalSplayWidth deprecated alias for
	RbdImageOptionJournalSplayWidth = ImageOptionJournalSplayWidth
	// RbdImageOptionJournalPool deprecated alias for ImageOptionJournalPool
	RbdImageOptionJournalPool = ImageOptionJournalPool
	// RbdImageOptionFeaturesSet deprecated alias for ImageOptionFeaturesSet
	RbdImageOptionFeaturesSet = ImageOptionFeaturesSet
	// RbdImageOptionFeaturesClear deprecated alias for ImageOptionFeaturesClear
	RbdImageOptionFeaturesClear = ImageOptionFeaturesClear
	// RbdImageOptionDataPool deprecated alias for ImageOptionDataPool
	RbdImageOptionDataPool = ImageOptionDataPool
)

// ImageOptions represents a group of configurable image options.
type ImageOptions struct {
	options C.rbd_image_options_t
}

// ImageOption values are unique keys for configurable options.
type ImageOption C.int

// revive:disable:exported Deprecated aliases

// RbdImageOptions deprecated alias for ImageOptions
type RbdImageOptions = ImageOptions

// RbdImageOption is a deprecated alias for ImageOption
type RbdImageOption = ImageOption

//revive:enable:exported

// NewRbdImageOptions creates a new RbdImageOptions struct. Call
// RbdImageOptions.Destroy() to free the resources.
//
// Implements:
//
//	void rbd_image_options_create(rbd_image_options_t* opts)
func NewRbdImageOptions() *ImageOptions {
	rio := &ImageOptions{}
	C.rbd_image_options_create(&rio.options)
	return rio
}

// Destroy a RbdImageOptions struct and free the associated resources.
//
// Implements:
//
//	void rbd_image_options_destroy(rbd_image_options_t opts);
func (rio *ImageOptions) Destroy() {
	C.rbd_image_options_destroy(rio.options)
}

// SetString sets the value of the RbdImageOption to the given string.
//
// Implements:
//
//	int rbd_image_options_set_string(rbd_image_options_t opts, int optname,
//	        const char* optval);
func (rio *ImageOptions) SetString(option ImageOption, value string) error {
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))

	ret := C.rbd_image_options_set_string(rio.options, C.int(option), cValue)
	if ret != 0 {
		return fmt.Errorf("%v, could not set option %v to \"%v\"",
			getError(ret), option, value)
	}

	return nil
}

// GetString returns the string value of the RbdImageOption.
//
// Implements:
//
//	int rbd_image_options_get_string(rbd_image_options_t opts, int optname,
//	        char* optval, size_t maxlen);
func (rio *ImageOptions) GetString(option ImageOption) (string, error) {
	value := make([]byte, 4096)

	ret := C.rbd_image_options_get_string(rio.options, C.int(option),
		(*C.char)(unsafe.Pointer(&value[0])),
		C.size_t(len(value)))
	if ret != 0 {
		return "", fmt.Errorf("%v, could not get option %v", getError(ret), option)
	}

	return C.GoString((*C.char)(unsafe.Pointer(&value[0]))), nil
}

// SetUint64 sets the value of the RbdImageOption to the given uint64.
//
// Implements:
//
//	int rbd_image_options_set_uint64(rbd_image_options_t opts, int optname,
//	        const uint64_t optval);
func (rio *ImageOptions) SetUint64(option ImageOption, value uint64) error {
	cValue := C.uint64_t(value)

	ret := C.rbd_image_options_set_uint64(rio.options, C.int(option), cValue)
	if ret != 0 {
		return fmt.Errorf("%v, could not set option %v to \"%v\"",
			getError(ret), option, value)
	}

	return nil
}

// GetUint64 returns the uint64 value of the RbdImageOption.
//
// Implements:
//
//	int rbd_image_options_get_uint64(rbd_image_options_t opts, int optname,
//	        uint64_t* optval);
func (rio *ImageOptions) GetUint64(option ImageOption) (uint64, error) {
	var cValue C.uint64_t

	ret := C.rbd_image_options_get_uint64(rio.options, C.int(option), &cValue)
	if ret != 0 {
		return 0, fmt.Errorf("%v, could not get option %v", getError(ret), option)
	}

	return uint64(cValue), nil
}

// IsSet returns a true if the RbdImageOption is set, false otherwise.
//
// Implements:
//
//	int rbd_image_options_is_set(rbd_image_options_t opts, int optname,
//	        bool* is_set);
func (rio *ImageOptions) IsSet(option ImageOption) (bool, error) {
	var cSet C.bool

	ret := C.rbd_image_options_is_set(rio.options, C.int(option), &cSet)
	if ret != 0 {
		return false, fmt.Errorf("%v, could not check option %v", getError(ret), option)
	}

	return bool(cSet), nil
}

// Unset a given RbdImageOption.
//
// Implements:
//
//	int rbd_image_options_unset(rbd_image_options_t opts, int optname)
func (rio *ImageOptions) Unset(option ImageOption) error {
	ret := C.rbd_image_options_unset(rio.options, C.int(option))
	if ret != 0 {
		return fmt.Errorf("%v, could not unset option %v", getError(ret), option)
	}

	return nil
}

// Clear all options in the RbdImageOptions.
//
// Implements:
//
//	void rbd_image_options_clear(rbd_image_options_t opts)
func (rio *ImageOptions) Clear() {
	C.rbd_image_options_clear(rio.options)
}

// IsEmpty returns true if there are no options set in the RbdImageOptions,
// false otherwise.
//
// Implements:
//
//	int rbd_image_options_is_empty(rbd_image_options_t opts)
func (rio *ImageOptions) IsEmpty() bool {
	ret := C.rbd_image_options_is_empty(rio.options)
	return ret != 0
}
