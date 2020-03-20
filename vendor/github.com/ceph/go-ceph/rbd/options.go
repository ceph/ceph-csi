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
	RbdImageOptionFormat            = C.RBD_IMAGE_OPTION_FORMAT
	RbdImageOptionFeatures          = C.RBD_IMAGE_OPTION_FEATURES
	RbdImageOptionOrder             = C.RBD_IMAGE_OPTION_ORDER
	RbdImageOptionStripeUnit        = C.RBD_IMAGE_OPTION_STRIPE_UNIT
	RbdImageOptionStripeCount       = C.RBD_IMAGE_OPTION_STRIPE_COUNT
	RbdImageOptionJournalOrder      = C.RBD_IMAGE_OPTION_JOURNAL_ORDER
	RbdImageOptionJournalSplayWidth = C.RBD_IMAGE_OPTION_JOURNAL_SPLAY_WIDTH
	RbdImageOptionJournalPool       = C.RBD_IMAGE_OPTION_JOURNAL_POOL
	RbdImageOptionFeaturesSet       = C.RBD_IMAGE_OPTION_FEATURES_SET
	RbdImageOptionFeaturesClear     = C.RBD_IMAGE_OPTION_FEATURES_CLEAR
	RbdImageOptionDataPool          = C.RBD_IMAGE_OPTION_DATA_POOL
	// introduced with Ceph Mimic
	//RbdImageOptionFlatten = C.RBD_IMAGE_OPTION_FLATTEN
)

type RbdImageOptions struct {
	options C.rbd_image_options_t
}

type RbdImageOption C.int

// NewRbdImageOptions creates a new RbdImageOptions struct. Call
// RbdImageOptions.Destroy() to free the resources.
//
// Implements:
//  void rbd_image_options_create(rbd_image_options_t* opts)
func NewRbdImageOptions() *RbdImageOptions {
	rio := &RbdImageOptions{}
	C.rbd_image_options_create(&rio.options)
	return rio
}

// Destroy a RbdImageOptions struct and free the associated resources.
//
// Implements:
//  void rbd_image_options_destroy(rbd_image_options_t opts);
func (rio *RbdImageOptions) Destroy() {
	C.rbd_image_options_destroy(rio.options)
}

// SetString sets the value of the RbdImageOption to the given string.
//
// Implements:
//  int rbd_image_options_set_string(rbd_image_options_t opts, int optname,
//          const char* optval);
func (rio *RbdImageOptions) SetString(option RbdImageOption, value string) error {
	c_value := C.CString(value)
	defer C.free(unsafe.Pointer(c_value))

	ret := C.rbd_image_options_set_string(rio.options, C.int(option), c_value)
	if ret != 0 {
		return fmt.Errorf("%v, could not set option %v to \"%v\"",
			getError(ret), option, value)
	}

	return nil
}

// GetString returns the string value of the RbdImageOption.
//
// Implements:
//  int rbd_image_options_get_string(rbd_image_options_t opts, int optname,
//          char* optval, size_t maxlen);
func (rio *RbdImageOptions) GetString(option RbdImageOption) (string, error) {
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
//  int rbd_image_options_set_uint64(rbd_image_options_t opts, int optname,
//          const uint64_t optval);
func (rio *RbdImageOptions) SetUint64(option RbdImageOption, value uint64) error {
	c_value := C.uint64_t(value)

	ret := C.rbd_image_options_set_uint64(rio.options, C.int(option), c_value)
	if ret != 0 {
		return fmt.Errorf("%v, could not set option %v to \"%v\"",
			getError(ret), option, value)
	}

	return nil
}

// GetUint64 returns the uint64 value of the RbdImageOption.
//
// Implements:
//  int rbd_image_options_get_uint64(rbd_image_options_t opts, int optname,
//          uint64_t* optval);
func (rio *RbdImageOptions) GetUint64(option RbdImageOption) (uint64, error) {
	var c_value C.uint64_t

	ret := C.rbd_image_options_get_uint64(rio.options, C.int(option), &c_value)
	if ret != 0 {
		return 0, fmt.Errorf("%v, could not get option %v", getError(ret), option)
	}

	return uint64(c_value), nil
}

// IsSet returns a true if the RbdImageOption is set, false otherwise.
//
// Implements:
//  int rbd_image_options_is_set(rbd_image_options_t opts, int optname,
//          bool* is_set);
func (rio *RbdImageOptions) IsSet(option RbdImageOption) (bool, error) {
	var c_set C.bool

	ret := C.rbd_image_options_is_set(rio.options, C.int(option), &c_set)
	if ret != 0 {
		return false, fmt.Errorf("%v, could not check option %v", getError(ret), option)
	}

	return bool(c_set), nil
}

// Unset a given RbdImageOption.
//
// Implements:
//  int rbd_image_options_unset(rbd_image_options_t opts, int optname)
func (rio *RbdImageOptions) Unset(option RbdImageOption) error {
	ret := C.rbd_image_options_unset(rio.options, C.int(option))
	if ret != 0 {
		return fmt.Errorf("%v, could not unset option %v", getError(ret), option)
	}

	return nil
}

// Clear all options in the RbdImageOptions.
//
// Implements:
//  void rbd_image_options_clear(rbd_image_options_t opts)
func (rio *RbdImageOptions) Clear() {
	C.rbd_image_options_clear(rio.options)
}

// IsEmpty returns true if there are no options set in the RbdImageOptions,
// false otherwise.
//
// Implements:
//  int rbd_image_options_is_empty(rbd_image_options_t opts)
func (rio *RbdImageOptions) IsEmpty() bool {
	ret := C.rbd_image_options_is_empty(rio.options)
	return ret != 0
}
