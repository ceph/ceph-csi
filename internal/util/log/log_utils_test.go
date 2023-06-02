/*
Copyright 2021 ceph-csi authors.

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

package log

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestGzipLogFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	logFile, err := os.CreateTemp(tmpDir, "rbd-*.log")
	if err != nil {
		fmt.Println(err)
	}
	defer os.Remove(logFile.Name())

	if err = GzipLogFile(logFile.Name()); err != nil {
		t.Errorf("GzipLogFile failed: %v", err)
	}

	newExt := strings.ReplaceAll(logFile.Name(), ".log", ".gz")
	if _, err = os.Stat(newExt); errors.Is(err, os.ErrNotExist) {
		t.Errorf("compressed logFile (%s) not found: %v", newExt, err)
	}

	os.Remove(newExt)
}
