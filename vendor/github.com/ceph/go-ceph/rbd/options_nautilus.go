// +build !luminous,!mimic

package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// ImageOptionCloneFormat is the representation of
	// RBD_IMAGE_OPTION_CLONE_FORMAT from librbd
	ImageOptionCloneFormat = C.RBD_IMAGE_OPTION_CLONE_FORMAT
)
