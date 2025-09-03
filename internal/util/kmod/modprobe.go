/*
Copyright 2025 The Ceph-CSI Authors.

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

package kmod

import (
	"context"
	"fmt"
	"os"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// Modprobe will check if the module is already available for use, and if that
// is not the case, it will try to load it.
func Modprobe(ctx context.Context, kmod string) error {
	_, err := os.Stat("/sys/module/" + kmod)
	if err == nil {
		// module already loaded (or compiled into the kernel)
		return nil
	}

	if !os.IsNotExist(err) {
		// something went really wrong
		err = fmt.Errorf("failed to check availability of kernel module %q: %w", kmod, err)
		log.WarningLog(ctx, err.Error())

		return err
	}

	// try to load the module
	_, stderr, err := util.ExecCommand(ctx, "modprobe", kmod)
	if err != nil {
		err = fmt.Errorf("modprobe of kernel module %q failed (%w): %s", kmod, err, stderr)
		log.WarningLog(ctx, err.Error())

		return err
	}

	return nil
}
