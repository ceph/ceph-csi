/*
 * cache.go - Handles cache clearing and management.
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package security

import (
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// DropFilesystemCache instructs the kernel to free the reclaimable inodes and
// dentries. This has the effect of making encrypted directories whose keys are
// not present no longer accessible. Requires root privileges.
func DropFilesystemCache() error {
	// Dirty reclaimable inodes must be synced so that they will be freed.
	log.Print("syncing changes to filesystem")
	unix.Sync()

	// See: https://www.kernel.org/doc/Documentation/sysctl/vm.txt
	log.Print("freeing reclaimable inodes and dentries")
	file, err := os.OpenFile("/proc/sys/vm/drop_caches", os.O_WRONLY|os.O_SYNC, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	// "2" just frees the reclaimable inodes and dentries. The associated
	// pages to these inodes will be freed. We do not need to free the
	// entire pagecache (as this will severely impact performance).
	_, err = file.WriteString("2")
	return err
}
