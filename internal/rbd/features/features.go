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
#cgo LDFLAGS: -lrbd
#include <rbd/librbd.h>
*/
import "C"

import (
	"strings"
	"sync"
)

var (
	groupGetSnapInfoOnce      sync.Once
	errGroupGetSnapInfo       error
	groupGetSnapInfoSupported = false
)

// SupportsGroupSnapGetInfo detects if librbd has the rbd_group_snap_get_info
// function.
func SupportsGroupSnapGetInfo() (bool, error) {
	groupGetSnapInfoOnce.Do(func() {
		// make sure librbd.so.x is loaded, might not (yet) be the case
		// if no rbd functions are called
		var opts C.rbd_image_options_t
		//nolint:gocritic // ignore result of rbd_image_options functions
		C.rbd_image_options_create(&opts)
		C.rbd_image_options_destroy(opts)

		// check for rbd_group_snap_get_info() in loaded libs/symbols
		errGroupGetSnapInfo = dlsym("rbd_group_snap_get_info")

		if errGroupGetSnapInfo == nil {
			groupGetSnapInfoSupported = true
		} else if strings.Contains(errGroupGetSnapInfo.Error(), "undefined symbol") {
			errGroupGetSnapInfo = nil
		}
	})

	return groupGetSnapInfoSupported, errGroupGetSnapInfo
}
