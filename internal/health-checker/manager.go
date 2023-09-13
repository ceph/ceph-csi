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
	"sync"
)

// Manager provides the API for getting the health status of a volume. The main
// usage is requesting the health status by path.
//
// When the Manager detects that a new path is used for checking, a new
// instance of a ConditionChecker is created  for the path, and started.
//
// Once the path is not active anymore (when NodeUnstageVolume is called), the
// ConditionChecker needs to be stopped, which can be done by
// Manager.StopChecker().
type Manager interface {
	StartChecker(path string) error
	StopChecker(path string)
	IsHealthy(path string) (bool, error)
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
	checkers sync.Map // map[path]ConditionChecker
}

func NewHealthCheckManager() Manager {
	return &healthCheckManager{
		checkers: sync.Map{},
	}
}

func (hcm *healthCheckManager) StartChecker(path string) error {
	cc := newFileChecker(path)

	// load the 'old' ConditionChecker if it exists, otherwuse store 'cc'
	old, ok := hcm.checkers.LoadOrStore(path, cc)
	if ok {
		// 'old' was loaded, cast it to ConditionChecker
		cc = old.(ConditionChecker)
	} else {
		// 'cc' was stored, start it only once
		cc.start()
	}

	return nil
}

func (hcm *healthCheckManager) StopChecker(path string) {
	old, ok := hcm.checkers.LoadAndDelete(path)
	if !ok {
		// nothing was loaded, nothing to do
		return
	}

	// 'old' was loaded, cast it to ConditionChecker
	cc := old.(ConditionChecker)
	cc.stop()
}

func (hcm *healthCheckManager) IsHealthy(path string) (bool, error) {
	// load the 'old' ConditionChecker if it exists
	old, ok := hcm.checkers.Load(path)
	if !ok {
		return true, fmt.Errorf("no ConditionChecker for path: %s", path)
	}

	// 'old' was loaded, cast it to ConditionChecker
	cc := old.(ConditionChecker)

	return cc.isHealthy()
}
