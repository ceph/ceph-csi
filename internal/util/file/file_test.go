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

package file

import (
	"os"
	"testing"
)

func TestCreateTempFile_WithValidContent(t *testing.T) {
	t.Parallel()

	content := "Valid Content"

	file, err := CreateTempFile("test-", content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer func() {
		err = os.Remove(file.Name())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}()

	readContent, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if string(readContent) != content {
		t.Fatalf("Content mismatch: got %v, want %v", string(readContent), content)
	}
}

func TestCreateTempFile_WithEmptyContent(t *testing.T) {
	t.Parallel()

	content := ""

	file, err := CreateTempFile("test-", content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer func() {
		err = os.Remove(file.Name())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}()

	readContent, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if string(readContent) != content {
		t.Fatalf("Content mismatch: got %v, want %v", string(readContent), content)
	}
}

func TestCreateTempFile_WithLargeContent(t *testing.T) {
	t.Parallel()

	content := string(make([]byte, 1<<20))

	file, err := CreateTempFile("test-", content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer func() {
		err = os.Remove(file.Name())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}()

	readContent, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if string(readContent) != content {
		t.Fatalf("Content mismatch: got %v, want %v", string(readContent), content)
	}
}
