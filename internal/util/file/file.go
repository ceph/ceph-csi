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
	"fmt"
	"os"
)

// CreateTempFile create a temporary file with the given string
// content and returns the reference to the file.
// The caller is responsible for disposing the file.
func CreateTempFile(prefix, contents string) (*os.File, error) {
	// Create a temp file
	file, err := os.CreateTemp("", prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	// In case of error, remove the file if it was created
	defer func() {
		if err != nil {
			_ = os.Remove(file.Name())
		}
	}()

	// Write the contents
	var c int
	c, err = file.WriteString(contents)
	if err != nil || c != len(contents) {
		return nil, fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Close the handle
	if err = file.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temporary file: %w", err)
	}

	return file, nil
}

// CreateSpareFile makes `file` a sparse file of size `sizeMB`.
func CreateSparseFile(file *os.File, sizeMB int64) error {
	sizeBytes := sizeMB * 1024 * 1024

	// seek to the end of the file.
	if _, err := file.Seek(sizeBytes-1, 0); err != nil {
		return fmt.Errorf("failed to seek to the end of the file: %w", err)
	}

	// write a single byte, effectively making it a sparse file.
	if _, err := file.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to write to the end of the file: %w", err)
	}

	return nil
}
