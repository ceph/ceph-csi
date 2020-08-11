// +build !luminous

package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// ImageOptionFlatten is the representation of RBD_IMAGE_OPTION_FLATTEN
	// from librbd
	ImageOptionFlatten = C.RBD_IMAGE_OPTION_FLATTEN

	// ImageOptionCloneFormat is the representation of
	// RBD_IMAGE_OPTION_CLONE_FORMAT from librbd
	ImageOptionCloneFormat = C.RBD_IMAGE_OPTION_CLONE_FORMAT
)
