/*
Copyright 2023 ceph-csi authors.

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
	"path"
	"testing"
	"time"
)

func TestFileChecker(t *testing.T) {
	t.Parallel()

	volumePath := path.Join(t.TempDir(), "csi-health-check.d")
	fc, err := newFileChecker(volumePath)
	if err != nil {
		t.Fatalf("failed to create FileChecker: %v", err)
	}

	checker := fc.(*fileChecker)
	checker.interval = time.Second * 5

	// start the checker
	checker.start()

	// wait a second to get the go routine running
	time.Sleep(time.Second)
	if !checker.isRunning {
		t.Error("checker failed to start")
	}

	for i := 0; i < 10; i++ {
		// check health, should be healthy
		healthy, msg := checker.isHealthy()
		if !healthy || msg != nil {
			t.Error("volume is unhealthy")
		}

		time.Sleep(time.Second)
	}

	// stop the checker
	checker.stop()
}

func TestWriteReadTimestamp(t *testing.T) {
	t.Parallel()

	volumePath := path.Join(t.TempDir(), "csi-health-check.d")
	fc, err := newFileChecker(volumePath)
	if err != nil {
		t.Fatalf("failed to create FileChecker: %v", err)
	}

	checker := fc.(*fileChecker)
	ts := time.Now()

	err = checker.writeTimestamp(ts)
	if err != nil {
		t.Fatalf("failed to write timestamp: %v", err)
	}

	_, err = checker.readTimestamp()
	if err != nil {
		t.Fatalf("failed to read timestamp: %v", err)
	}
}
