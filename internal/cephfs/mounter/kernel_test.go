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

	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/cephfs/store"
	"github.com/ceph/ceph-csi/internal/util"
)

func TestFilesystemSupported(t *testing.T) {
	t.Parallel()

	testErrorf = func(fmt string, args ...any) {
		t.Errorf(fmt, args...)
	}

	// "proc" is always a supported filesystem, we detect supported
	// filesystems by reading from it
	require.True(t, filesystemSupported("proc"))

	// "nonefs" is a made-up name, and does not exist
	require.False(t, filesystemSupported("nonefs"))
}

func TestIsKernelMounter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mnt  VolumeMounter
		want bool
	}{
		{"kernel", &kernelMounter{}, true},
		{"FUSE", &FuseMounter{}, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsKernelMounter(tc.mnt); got != tc.want {
				t.Fatalf("IsKernelMounter(%T) = %v, want %v", tc.mnt, got, tc.want)
			}
		})
	}
}

func TestIsRecoverable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mnt     VolumeMounter
		options *store.VolumeOptions
		want    bool
	}{
		{name: "kernel with unset option", mnt: &kernelMounter{}, options: &store.VolumeOptions{}, want: true},
		{name: "FUSE fallback with kernel option", mnt: &FuseMounter{}, options: &store.VolumeOptions{Mounter: "kernel"}},
		{
			name:    "encrypted kernel",
			mnt:     &kernelMounter{},
			options: &store.VolumeOptions{Encryption: &util.VolumeEncryption{}},
		},
		{
			name:    "snapshot-backed kernel",
			mnt:     &kernelMounter{},
			options: &store.VolumeOptions{BackingSnapshot: true},
		},
		{name: "nil mounter", options: &store.VolumeOptions{}},
		{name: "nil options", mnt: &kernelMounter{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRecoverable(tc.mnt, tc.options); got != tc.want {
				t.Fatalf("IsRecoverable(%T, %+v) = %t, want %t", tc.mnt, tc.options, got, tc.want)
			}
		})
	}
}
