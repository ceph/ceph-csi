package rados

/*
#include <errno.h>
*/
import "C"

import (
	"errors"
	"fmt"

	"github.com/ceph/go-ceph/internal/errutil"
)

// revive:disable:exported Temporarily live with stuttering

// RadosError represents an error condition returned from the Ceph RADOS APIs.
type RadosError int

// revive:enable:exported

// Error returns the error string for the RadosError type.
func (e RadosError) Error() string {
	errno, s := errutil.FormatErrno(int(e))
	if s == "" {
		return fmt.Sprintf("rados: ret=%d", errno)
	}
	return fmt.Sprintf("rados: ret=%d, %s", errno, s)
}

func getError(e C.int) error {
	if e == 0 {
		return nil
	}
	return RadosError(e)
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

// Public go errors:

var (
	// ErrNotConnected is returned when functions are called without a RADOS connection
	ErrNotConnected = errors.New("RADOS not connected")
)

// Public RadosErrors:

const (
	// ErrNotFound indicates a missing resource.
	ErrNotFound = RadosError(-C.ENOENT)
	// ErrPermissionDenied indicates a permissions issue.
	ErrPermissionDenied = RadosError(-C.EPERM)
	// ErrObjectExists indicates that an exclusive object creation failed.
	ErrObjectExists = RadosError(-C.EEXIST)

	// RadosErrorNotFound indicates a missing resource.
	//
	// Deprecated: use ErrNotFound instead
	RadosErrorNotFound = ErrNotFound
	// RadosErrorPermissionDenied indicates a permissions issue.
	//
	// Deprecated: use ErrPermissionDenied instead
	RadosErrorPermissionDenied = ErrPermissionDenied
)

// Private errors:

const (
	errNameTooLong = RadosError(-C.ENAMETOOLONG)

	errRange = RadosError(-C.ERANGE)
)
