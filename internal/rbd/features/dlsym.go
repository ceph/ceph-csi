/*
Copyright 2024 The Ceph-CSI Authors.

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

package features

/*
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include <dlfcn.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// dlsym checks if the given symbol is provided by the currently loaded
// libraries. If the symbol is available, no error is returned.
func dlsym(symbol string) error {
	c_symbol := C.CString(symbol)
	//nolint:nlreturn // linter complains about missing empty line!?
	defer C.free(unsafe.Pointer(c_symbol))

	// clear dlerror before looking up the symbol
	C.dlerror()
	_ = C.dlsym(nil, c_symbol)
	e := C.dlerror()
	err := C.GoString(e)
	if err != "" {
		return fmt.Errorf("dlsym: %s", err)
	}

	return nil
}
