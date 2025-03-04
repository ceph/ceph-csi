/*
Copyright 2021 The Ceph-CSI Authors.
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
	"compress/gzip"
	"os"
	"strings"
)

// GzipLogFile convert and replace log file from text format to gzip
// compressed format.
func GzipLogFile(pathToFile string) error {
	// Get all the bytes from the file.
	content, err := os.ReadFile(pathToFile) // #nosec:G304, file inclusion via variable.
	if err != nil {
		return err
	}

	// Replace .log extension with .gz extension.
	newExt := strings.ReplaceAll(pathToFile, ".log", ".gz")

	// Open file for writing.
	gf, err := os.OpenFile(newExt, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644) // #nosec:G304,G302, file inclusion & perms
	if err != nil {
		return err
	}
	defer gf.Close() // #nosec:G307, error on close is not critical here

	// Write compressed data.
	w := gzip.NewWriter(gf)
	defer w.Close()
	if _, err = w.Write(content); err != nil {
		os.Remove(newExt) // #nosec:G104, not important error to handle

		return err
	}

	return os.Remove(pathToFile)
}
