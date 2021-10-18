//go:build !nautilus
// +build !nautilus

package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// ImageOptionMirrorImageMode is the representation of
	// RBD_IMAGE_OPTION_MIRROR_IMAGE_MODE from librbd
	ImageOptionMirrorImageMode = C.RBD_IMAGE_OPTION_MIRROR_IMAGE_MODE
)
