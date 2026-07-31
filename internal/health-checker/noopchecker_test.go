/*
Copyright 2026 ceph-csi authors.

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

package healthchecker

import (
	"testing"
	"time"
)

func TestNoOpChecker(t *testing.T) {
	t.Parallel()

	nc := newNoOpChecker()
	checker, ok := nc.(*noOpChecker)
	if !ok {
		t.Errorf("failed to convert nc to *noOpChecker: %v", nc)
	}

	// start the checker
	checker.start()

	// wait a second to get the go routine running
	time.Sleep(time.Second)
	if !checker.isRunning {
		t.Error("checker failed to start")
	}

	// check health, should always be healthy with the expected message
	healthy, msg := checker.isHealthy()
	if !healthy {
		t.Error("NoOpChecker should always return healthy=true")
	}
	if msg != nil {
		t.Error("NoOpChecker should never return an error message")
	}

	// verify it stays healthy over time
	for range 5 {
		healthy, msg = checker.isHealthy()
		if !healthy {
			t.Error("NoOpChecker should always return healthy=true")
		}
		if msg != nil {
			t.Errorf("unexpected message: %v", msg)
		}
		time.Sleep(time.Second)
	}

	if !checker.isRunning {
		t.Error("runChecker() exited already")
	}

	// stop the checker
	checker.stop()
}
