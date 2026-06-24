/*
Copyright 2026 The Ceph-CSI Authors.

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

package rbd

/*
#include <linux/fs.h>
#include <sys/ioctl.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// errTrimNotSupported indicates the filesystem does not support FITRIM.
var errTrimNotSupported = errors.New("trim operation not supported by filesystem")

// fsTrim performs filesystem trim operation on the given path using FITRIM ioctl.
// This is equivalent to running 'fstrim' command but uses direct ioctl call.
// Returns ErrTrimNotSupported if the filesystem does not support trim operations.
func fsTrim(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open path %q: %w", path, err)
	}
	defer func() {
		if file.Close() == nil {
			// nothing to do
			return
		}

		log.ErrorLogMsg("failed to close open filedescriptor for %q, unmounting may fail", path)
	}()

	trimRange := C.struct_fstrim_range{
		start:  0,           // 0 (trim from beginning)
		len:    ^C.__u64(0), // ~0ULL (trim entire filesystem)
		minlen: 0,           // 0 (no minimum extent length)
	}

	_, _, err = syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		C.FITRIM,
		uintptr(unsafe.Pointer(&trimRange)),
	)
	if err != nil {
		// Check for fatal errors that indicate the operation is not supported
		if errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, syscall.ENOTTY) {
			return fmt.Errorf("%w: %w", errTrimNotSupported, err)
		}

		return fmt.Errorf("FITRIM ioctl failed: %w", err)
	}

	return nil
}
