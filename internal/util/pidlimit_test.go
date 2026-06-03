/*
Copyright 2019 ceph-csi authors.

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

package util

import (
	"os"
	"testing"
)

// minimal test to check if GetPIDLimit() returns an int
// changing the limit require root permissions, not tested.
func TestGetPIDLimit(t *testing.T) {
	t.Parallel()
	runTest := os.Getenv("CEPH_CSI_RUN_ALL_TESTS")
	if runTest == "" {
		t.Skip("not running test that requires root permissions and cgroup support")
	}

	limit, err := GetPIDLimit()
	if err != nil {
		t.Errorf("no error should be returned, got: %v", err)
	}
	if limit == 0 {
		t.Error("a PID limit of 0 is invalid")
	}

	// this is expected to fail when not run as root
	err = SetPIDLimit(4096)
	if err != nil {
		t.Log("failed to set PID limit, are you running as root?")
	} else {
		// in case it worked, reset to the previous value
		err = SetPIDLimit(limit)
		if err != nil {
			t.Logf("failed to reset PID to original limit %d", limit)
		}
	}
}
