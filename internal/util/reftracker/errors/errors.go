/*
Copyright 2022 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package errors

import (
	goerrors "errors"
	"fmt"

	"github.com/ceph/go-ceph/rados"
	"golang.org/x/sys/unix"
)

// ErrObjectOutOfDate is an error returned by RADOS read/write ops whose
// rados_*_op_assert_version failed.
var ErrObjectOutOfDate = goerrors.New("object is out of date since the last time it was read, try again later")

// UnexpectedReadSize formats an error message for a failure due to bad read
// size.
func UnexpectedReadSize(expectedBytes, actualBytes int) error {
	return fmt.Errorf("unexpected size read: expected %d bytes, got %d",
		expectedBytes, actualBytes)
}

// UnknownObjectVersion formats an error message for a failure due to unknown
// reftracker object version.
func UnknownObjectVersion(unknownVersion uint32) error {
	return fmt.Errorf("unknown reftracker version %d", unknownVersion)
}

// FailedObjectRead formats an error message for a failed RADOS read op.
func FailedObjectRead(cause error) error {
	if cause != nil {
		return fmt.Errorf("failed to read object: %w", TryRADOSAborted(cause))
	}

	return nil
}

// FailedObjectRead formats an error message for a failed RADOS read op.
func FailedObjectWrite(cause error) error {
	if cause != nil {
		return fmt.Errorf("failed to write object: %w", TryRADOSAborted(cause))
	}

	return nil
}

// TryRADOSAborted tries to extract rados_*_op_assert_version from opErr.
func TryRADOSAborted(opErr error) error {
	if opErr == nil {
		return nil
	}

	var radosOpErr rados.OperationError
	if !goerrors.As(opErr, &radosOpErr) {
		return opErr
	}

	//nolint:errorlint // Can't use errors.As() because rados.radosError is private.
	errnoErr, ok := radosOpErr.OpError.(interface{ ErrorCode() int })
	if !ok {
		return opErr
	}

	errno := errnoErr.ErrorCode()
	if errno == -int(unix.EOVERFLOW) || errno == -int(unix.ERANGE) {
		return ErrObjectOutOfDate
	}

	return nil
}
