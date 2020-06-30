// +build !luminous

package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// ImageOptionFlatten is the representation of RBD_IMAGE_OPTION_FLATTEN
	// from librbd
	ImageOptionFlatten = C.RBD_IMAGE_OPTION_FLATTEN
)
