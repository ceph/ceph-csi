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
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// CheckerType describes the type of health-check that needs to be done.
type CheckerType uint64

const (
	// StatCheckerType uses the stat() syscall to validate volume health.
	StatCheckerType = iota
	// FileCheckerType writes and reads a timestamp to a file for checking the
	// volume health.
	FileCheckerType
)

// Manager provides the API for getting the health status of a volume. The main
// usage is requesting the health status by volumeID.
//
// When the Manager detects that a new volumeID is used for checking, a new
// instance of a ConditionChecker is created for the volumeID on the given
// path, and started.
//
// Once the volumeID is not active anymore (when NodeUnstageVolume is called),
// the ConditionChecker needs to be stopped, which can be done by
// Manager.StopChecker().
type Manager interface {
	// StartChecker starts a health-checker of the requested type for the
	// volumeID using the path. The path usually is the publishTargetPath, and
	// a unique path for this checker. If the path can be used by multiple
	// containers, use the StartSharedChecker function instead.
	StartChecker(volumeID, path string, ct CheckerType) error

	// StartSharedChecker starts a health-checker of the requested type for the
	// volumeID using the path. The path usually is the stagingTargetPath, and
	// can be used for multiple containers.
	StartSharedChecker(volumeID, path string, ct CheckerType) error

	StopChecker(volumeID, path string)
	StopSharedChecker(volumeID string)

	// IsHealthy locates the checker for the volumeID and path. If no checker
	// is found, `true` is returned together with an error message.
	// When IsHealthy runs into an internal error, it is assumed that the
	// volume is healthy. Only when it is confirmed that the volume is
	// unhealthy, `false` is returned together with an error message.
	IsHealthy(volumeID, path string) (bool, error)
}

// ConditionChecker describes the interface that a health status reporter needs
// to implement. It is used internally by the Manager only.
type ConditionChecker interface {
	// start runs a the health checking function in a new go routine.
	start()

	// stop terminates a the health checking function that runs in a go
	// routine.
	stop()

	// isHealthy returns the status of the volume, without blocking.
	isHealthy() (bool, error)
}

type healthCheckManager struct {
	checkers sync.Map // map[volumeID]ConditionChecker
}

func NewHealthCheckManager() Manager {
	return &healthCheckManager{
		checkers: sync.Map{},
	}
}

func (hcm *healthCheckManager) StartSharedChecker(volumeID, path string, ct CheckerType) error {
	return hcm.createChecker(volumeID, path, ct, true)
}

func (hcm *healthCheckManager) StartChecker(volumeID, path string, ct CheckerType) error {
	return hcm.createChecker(volumeID, path, ct, false)
}

// createChecker decides based on the CheckerType what checker to start for
// the volume.
func (hcm *healthCheckManager) createChecker(volumeID, path string, ct CheckerType, shared bool) error {
	switch ct {
	case FileCheckerType:
		return hcm.startFileChecker(volumeID, path, shared)
	case StatCheckerType:
		return hcm.startStatChecker(volumeID, path, shared)
	}

	return nil
}

// startFileChecker initializes the fileChecker and starts it.
func (hcm *healthCheckManager) startFileChecker(volumeID, path string, shared bool) error {
	workdir := filepath.Join(path, ".csi")
	err := os.Mkdir(workdir, 0o755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to created workdir %q for health-checker: %w", workdir, err)
	}

	cc := newFileChecker(workdir)

	return hcm.startChecker(cc, volumeID, path, shared)
}

// startStatChecker initializes the statChecker and starts it.
func (hcm *healthCheckManager) startStatChecker(volumeID, path string, shared bool) error {
	cc := newStatChecker(path)

	return hcm.startChecker(cc, volumeID, path, shared)
}

// startChecker adds the checker to its map and starts it.
// Shared checkers are key'd by their volumeID, whereas non-shared checkers
// are key'd by theit volumeID+path.
func (hcm *healthCheckManager) startChecker(cc ConditionChecker, volumeID, path string, shared bool) error {
	key := volumeID
	if shared {
		key = fallbackKey(volumeID, path)
	}

	// load the 'old' ConditionChecker if it exists, otherwise store 'cc'
	old, ok := hcm.checkers.LoadOrStore(key, cc)
	if ok {
		// 'old' was loaded, cast it to ConditionChecker
		_, ok = old.(ConditionChecker)
		if !ok {
			return fmt.Errorf("failed to cast cc to ConditionChecker for volume-id %q", volumeID)
		}
	} else {
		// 'cc' was stored, start it only once
		cc.start()
	}

	return nil
}

func (hcm *healthCheckManager) StopSharedChecker(volumeID string) {
	hcm.StopChecker(volumeID, "")
}

func (hcm *healthCheckManager) StopChecker(volumeID, path string) {
	old, ok := hcm.checkers.LoadAndDelete(fallbackKey(volumeID, path))
	if !ok {
		// nothing was loaded, nothing to do
		return
	}

	// 'old' was loaded, cast it to ConditionChecker
	cc, ok := old.(ConditionChecker)
	if !ok {
		// failed to cast, should not be possible
		return
	}
	cc.stop()
}

func (hcm *healthCheckManager) IsHealthy(volumeID, path string) (bool, error) {
	// load the 'old' ConditionChecker if it exists
	old, ok := hcm.checkers.Load(volumeID)
	if !ok {
		// try fallback which include an optional (unique) path (usually publishTargetPath)
		old, ok = hcm.checkers.Load(fallbackKey(volumeID, path))
		if !ok {
			return true, fmt.Errorf("no ConditionChecker for volume-id: %s", volumeID)
		}
	}

	// 'old' was loaded, cast it to ConditionChecker
	cc, ok := old.(ConditionChecker)
	if !ok {
		return true, fmt.Errorf("failed to cast cc to ConditionChecker for volume-id %q", volumeID)
	}

	return cc.isHealthy()
}

// fallbackKey returns the key for a checker in the map. If the path is empty,
// it is assumed that the key'd checked is shared.
func fallbackKey(volumeID, path string) string {
	if path == "" {
		return volumeID
	}

	return fmt.Sprintf("%s:%s", volumeID, path)
}
