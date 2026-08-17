//go:build !(nautilus || octopus || pacific || quincy || reef)

package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// LockModeExclusiveTransient is the representation of
	// RBD_LOCK_MODE_EXCLUSIVE_TRANSIENT from librbd.
	LockModeExclusiveTransient = LockMode(C.RBD_LOCK_MODE_EXCLUSIVE_TRANSIENT)
)
