/*
Copyright 2026 The Ceph-CSI Authors.

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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMountCache(t *testing.T) {
	t.Parallel()
	cache := NewMountCache()

	// Test adding and retrieving mappings
	cache.Add("/dev/nvme0n1", "/mnt/staging1")
	cache.Add("/dev/nvme1n1", "/mnt/staging2")

	device, exists := cache.GetDevice("/mnt/staging1")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme0n1", device)

	device, exists = cache.GetDevice("/mnt/staging2")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme1n1", device)

	// Test retrieving a non-existent mapping
	_, exists = cache.GetDevice("/mnt/nonexistent")
	require.False(t, exists)

	// Test removing a mapping by device
	cache.RemoveByDevice("/dev/nvme0n1")

	_, exists = cache.GetDevice("/mnt/staging1")
	require.False(t, exists)

	// Ensure the other mapping still exists
	device, exists = cache.GetDevice("/mnt/staging2")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme1n1", device)
}

func TestMountCacheConcurrency(t *testing.T) {
	t.Parallel()
	cache := NewMountCache()
	done := make(chan bool)

	// Start multiple goroutines to add mappings concurrently
	for i := range [10]int{} {
		go func(i int) {
			cache.Add(fmt.Sprintf("/dev/nvme%dn1", i),
				fmt.Sprintf("/mnt/staging%d", i))
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for range [10]int{} {
		<-done
	}

	// Verify that all mappings were added correctly
	for i := range [10]int{} {
		device, exists := cache.GetDevice(fmt.Sprintf("/mnt/staging%d", i))
		require.True(t, exists)
		require.Equal(t, fmt.Sprintf("/dev/nvme%dn1", i), device)
	}
}

func TestMountCacheRemoveByMountPoint(t *testing.T) {
	t.Parallel()
	cache := NewMountCache()

	cache.Add("/dev/nvme0n1", "/mnt/staging1")
	cache.Add("/dev/nvme1n1", "/mnt/staging2")

	// Remove by mount point
	cache.RemoveByMountPoint("/mnt/staging1")

	_, exists := cache.GetDevice("/mnt/staging1")
	require.False(t, exists)

	// Ensure the other mapping still exists
	device, exists := cache.GetDevice("/mnt/staging2")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme1n1", device)
}

func TestMountCacheRemoveNonExistent(t *testing.T) {
	t.Parallel()
	cache := NewMountCache()

	cache.Add("/dev/nvme0n1", "/mnt/staging1")

	// Attempt to remove a non-existent device
	cache.RemoveByDevice("/dev/nvme2n1")

	// Ensure the existing mapping still exists
	device, exists := cache.GetDevice("/mnt/staging1")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme0n1", device)

	// Attempt to remove a non-existent staging path
	cache.RemoveByMountPoint("/mnt/nonexistent")

	// Ensure the existing mapping still exists
	device, exists = cache.GetDevice("/mnt/staging1")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme0n1", device)
}

func TestMountCacheEmpty(t *testing.T) {
	t.Parallel()
	cache := NewMountCache()

	// Attempt to retrieve from an empty cache
	_, exists := cache.GetDevice("/mnt/staging1")
	require.False(t, exists)

	// Attempt to remove from an empty cache
	cache.RemoveByDevice("/dev/nvme0n1")
	cache.RemoveByMountPoint("/mnt/staging1")

	// Ensure the cache is still empty
	_, exists = cache.GetDevice("/mnt/staging1")
	require.False(t, exists)
}

func TestMountCacheOverwriteFailed(t *testing.T) {
	t.Parallel()
	cache := NewMountCache()

	cache.Add("/dev/nvme0n1", "/mnt/staging1")
	cache.Add("/dev/nvme0n1", "/mnt/staging2") // Overwrite existing mapping

	// Ensure the new mapping is not in place
	_, exists := cache.GetDevice("/mnt/staging2")
	require.False(t, exists)

	// Ensure the old mapping is still in place
	device, exists := cache.GetDevice("/mnt/staging1")
	require.True(t, exists)
	require.Equal(t, "/dev/nvme0n1", device)
}
