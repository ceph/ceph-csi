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
	"maps"
	"sync"
)

// MountCache defines the contract for managing mount cache operations.
type MountCache interface {
	Add(device, stagingPath string)
	GetDevice(stagingPath string) (string, bool)
	RemoveByDevice(device string)
	RemoveByMountPoint(stagingPath string)
	GetCopyAllDevices() map[string]string
}

// mountCache is the concrete implementation of MountCache interface.
// it is a thread-safe cache that maintains the mapping between staging paths and devices.
// Note: This map is 1:1, meaning each staging path corresponds to exactly one device.
// device can only be mounted to one staging path at a time, and each staging path can only have one device.
type mountCache struct {
	pathToDevice map[string]string // stagingPath -> device
	deviceToPath map[string]string // device -> stagingPath
	mu           sync.Mutex        // protects concurrent access
}

// NewMountCache initializes and returns a new empty instance of MountCache.
func NewMountCache() MountCache {
	return &mountCache{
		pathToDevice: make(map[string]string),
		deviceToPath: make(map[string]string),
	}
}

// Add adds a new mapping between the given device and staging path to the cache.
func (mc *mountCache) Add(device, stagingPath string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Check if already exists - extra safety to prevent overwriting existing mappings
	// even though the caller should ensure this, we want to avoid accidentally overwriting
	// existing mappings in the cache.
	if _, exists := mc.pathToDevice[stagingPath]; exists {
		// Already in cache, do nothing
		return
	}

	if _, exists := mc.deviceToPath[device]; exists {
		// Already in cache, do nothing
		return
	}

	mc.deviceToPath[device] = stagingPath
	mc.pathToDevice[stagingPath] = device
}

// GetDevice returns the device associated with the given staging path,
// along with a boolean indicating if the mapping exists.
func (mc *mountCache) GetDevice(stagingPath string) (string, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	device, exists := mc.pathToDevice[stagingPath]

	return device, exists
}

// RemoveByDevice removes the mapping for the given device and
// its associated staging path from the cache.
func (mc *mountCache) RemoveByDevice(device string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	stagingPath, exists := mc.deviceToPath[device]
	if !exists {
		return // Device not in cache, nothing to do
	}
	delete(mc.deviceToPath, device)
	delete(mc.pathToDevice, stagingPath)
}

// RemoveByMountPoint removes the mapping for the given staging path
// and its associated device from the cache.
func (mc *mountCache) RemoveByMountPoint(stagingPath string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	device, exists := mc.pathToDevice[stagingPath]
	if !exists {
		return // Staging path not in cache, nothing to do
	}
	delete(mc.pathToDevice, stagingPath)
	delete(mc.deviceToPath, device)
}

// GetCopyAllDevices returns a copy of all devices currently in the cache.
// The returned map has device paths as keys and staging paths as values.
func (mc *mountCache) GetCopyAllDevices() map[string]string {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	return maps.Clone(mc.deviceToPath)
}
