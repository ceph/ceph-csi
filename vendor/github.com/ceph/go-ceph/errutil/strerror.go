/*
Package errutil provides common functions for dealing with error conditions for
all ceph api wrappers.
*/
package errutil

/* force XSI-complaint strerror_r() */

// #define _POSIX_C_SOURCE 200112L
// #undef _GNU_SOURCE
// #include <stdlib.h>
// #include <errno.h>
// #include <string.h>
import "C"

import (
	"unsafe"
)

// FormatErrno returns the absolute value of the errno as well as a string
// describing the errno. The string will be empty is the errno is not known.
func FormatErrno(errno int) (int, string) {
	buf := make([]byte, 1024)
	// strerror expects errno >= 0
	if errno < 0 {
		errno = -errno
	}

	ret := C.strerror_r(
		C.int(errno),
		(*C.char)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)))
	if ret != 0 {
		return errno, ""
	}

	return errno, C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
}

// StrError returns a string describing the errno. The string will be empty if
// the errno is not known.
func StrError(errno int) string {
	_, s := FormatErrno(errno)
	return s
}
