//go:build ceph_preview

package osd

/*
#include <errno.h>
*/
import "C"

import (
	"errors"

	"github.com/ceph/go-ceph/internal/errutil"
)

var (
	// ErrEmptyArgument may be returned if argument is empty.
	ErrEmptyArgument = errors.New("Argument must contain at least one item")
	// ErrInvalidArgument may be returned if argument is invalid.
	ErrInvalidArgument = getError(-C.EINVAL)
)

func getError(errno C.int) error {
	return errutil.GetError("osd", int(errno))
}

// getErrorIfNegative converts a ceph return code to error if negative.
// This is useful for functions that return a usable positive value on
// success but a negative error number on error.
func getErrorIfNegative(ret C.int) error {
	if ret >= 0 {
		return nil
	}
	return getError(ret)
}
