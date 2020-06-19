package rbd

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

// RBDError represents an error condition returned from the librbd APIs.
type RBDError int

// revive:enable:exported

func (e RBDError) Error() string {
	errno, s := errutil.FormatErrno(int(e))
	if s == "" {
		return fmt.Sprintf("rbd: ret=%d", errno)
	}
	return fmt.Sprintf("rbd: ret=%d, %s", errno, s)
}

func getError(err C.int) error {
	if err != 0 {
		if err == -C.ENOENT {
			return ErrNotFound
		}
		return RBDError(err)
	}
	return nil
}

// Public go errors:

var (
	// ErrNoIOContext may be returned if an api call requires an IOContext and
	// it is not provided.
	ErrNoIOContext = errors.New("RBD image does not have an IOContext")
	// ErrNoName may be returned if an api call requires a name and it is
	// not provided.
	ErrNoName = errors.New("RBD image does not have a name")
	// ErrSnapshotNoName may be returned if an aip call requires a snapshot
	// name and it is not provided.
	ErrSnapshotNoName = errors.New("RBD snapshot does not have a name")
	// ErrImageNotOpen may be returnened if an api call requires an open image handle and one is not provided.
	ErrImageNotOpen = errors.New("RBD image not open")
	// ErrNotFound may be returned from an api call when the requested item is
	// missing.
	ErrNotFound = errors.New("RBD image not found")

	// revive:disable:exported for compatibility with old versions
	RbdErrorImageNotOpen = ErrImageNotOpen
	RbdErrorNotFound     = ErrNotFound
	// revive:enable:exported
)
