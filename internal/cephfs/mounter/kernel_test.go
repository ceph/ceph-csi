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

package mounter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilesystemSupported(t *testing.T) {
	t.Parallel()

	testErrorf = func(fmt string, args ...any) {
		t.Errorf(fmt, args...)
	}

	// "proc" is always a supported filesystem, we detect supported
	// filesystems by reading from it
	assert.True(t, filesystemSupported("proc"))

	// "nonefs" is a made-up name, and does not exist
	assert.False(t, filesystemSupported("nonefs"))
}
