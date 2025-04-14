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
	"testing"

	"github.com/stretchr/testify/require"
)

const volumeID = "fake-volume-id"

func TestManager(t *testing.T) {
	t.Parallel()

	volumePath := t.TempDir()
	mgr := NewHealthCheckManager()

	// expected to have an error in msg
	healthy, msg := mgr.IsHealthy(volumeID, volumePath)
	if !(healthy && msg != nil) {
		t.Error("ConditionChecker was not started yet, did not get an error")
	}

	t.Log("start the checker")
	err := mgr.StartChecker(volumeID, volumePath, StatCheckerType)
	if err != nil {
		t.Fatalf("ConditionChecker could not get started: %v", err)
	}

	t.Log("check health, should be healthy")
	healthy, msg = mgr.IsHealthy(volumeID, volumePath)
	if !healthy || err != nil {
		t.Errorf("volume is unhealthy: %s", msg)
	}

	t.Log("stop the checker")
	mgr.StopChecker(volumeID, volumePath)
}

func TestSharedChecker(t *testing.T) {
	t.Parallel()

	volumePath := t.TempDir()
	mgr := NewHealthCheckManager()

	// expected to have an error in msg
	healthy, msg := mgr.IsHealthy(volumeID, volumePath)
	if !(healthy && msg != nil) {
		t.Error("ConditionChecker was not started yet, did not get an error")
	}

	t.Log("start the checker")
	err := mgr.StartSharedChecker(volumeID, volumePath, StatCheckerType)
	if err != nil {
		t.Fatalf("ConditionChecker could not get started: %v", err)
	}

	t.Log("check health, should be healthy")
	healthy, msg = mgr.IsHealthy(volumeID, volumePath)
	if !healthy || err != nil {
		t.Errorf("volume is unhealthy: %s", msg)
	}

	t.Log("check health, should be healthy, path is ignored")
	healthy, msg = mgr.IsHealthy(volumeID, "different-path")
	if !healthy || err != nil {
		t.Errorf("volume is unhealthy: %s", msg)
	}

	t.Log("stop the checker")
	mgr.StopSharedChecker(volumeID)
}

func TestTwoNonSharedChecker(t *testing.T) {
	t.Parallel()

	// create two different paths for same volumeID
	// to test if the checkers are independent
	firstVolumePath := t.TempDir()
	secondVolumePath := t.TempDir()
	mgr := NewHealthCheckManager()

	t.Log("start the first checker")
	err := mgr.StartChecker(volumeID, firstVolumePath, StatCheckerType)
	if err != nil {
		t.Fatalf("ConditionChecker could not get started: %v", err)
	}

	t.Log("check health for first path, should be healthy")
	healthy, msg := mgr.IsHealthy(volumeID, firstVolumePath)
	if !healthy || err != nil {
		t.Errorf("volume is unhealthy: %s", msg)
	}

	t.Log("check health for second path, should error out since checker is not started")
	_, msg = mgr.IsHealthy(volumeID, secondVolumePath)
	require.ErrorContains(t, msg, "no ConditionChecker for volume-id")

	t.Log("start the second checker")
	err = mgr.StartChecker(volumeID, secondVolumePath, StatCheckerType)
	if err != nil {
		t.Fatalf("ConditionChecker could not get started: %v", err)
	}

	t.Log("check health, should be healthy")
	healthy, msg = mgr.IsHealthy(volumeID, secondVolumePath)
	if !healthy || err != nil {
		t.Errorf("volume is unhealthy: %s", msg)
	}

	t.Log("stop the first checker")
	mgr.StopChecker(volumeID, firstVolumePath)

	t.Log("check health of second path, should still be healthy")
	healthy, msg = mgr.IsHealthy(volumeID, secondVolumePath)
	if !healthy || err != nil {
		t.Errorf("volume is unhealthy: %s", msg)
	}

	t.Log("stop the second checker")
	mgr.StopChecker(volumeID, secondVolumePath)
}
