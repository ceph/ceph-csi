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
// content and returns the reference to the file. The file is automatically
// closed; the caller is responsible for removing the file when done.
func CreateTempFile(prefix, contents string) (*os.File, error) {
	// Create a temp file
	file, err := os.CreateTemp("", prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	// Always close the file and remove it on error
	defer func() {
		//nolint:errcheck // close error is less important than sync error
		_ = file.Close()
		if err != nil {
			//nolint:errcheck // temporary file failed to remove, shrug
			_ = os.Remove(file.Name())
		}
	}()

	// Write the contents
	var c int
	c, err = file.WriteString(contents)
	if err != nil || c != len(contents) {
		return nil, fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Flush the contents to ensure they're written to disk
	if err = file.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync temporary file: %w", err)
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
