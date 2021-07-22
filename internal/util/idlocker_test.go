/*
Copyright 2019 ceph-csi authors.

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
	"testing"
)

// very basic tests for the moment.
func TestIDLocker(t *testing.T) {
	t.Parallel()
	fakeID := "fake-id"
	locks := NewVolumeLocks()
	// acquire lock for fake-id
	ok := locks.TryAcquire(fakeID)

	if !ok {
		t.Errorf("TryAcquire failed: want (%v), got (%v)",
			true, ok)
	}

	// try to acquire lock  again for fake-id, as lock is already present
	// it should fail
	ok = locks.TryAcquire(fakeID)

	if ok {
		t.Errorf("TryAcquire failed: want (%v), got (%v)",
			false, ok)
	}

	// release the lock for fake-id and try to get lock again, it should pass
	locks.Release(fakeID)
	ok = locks.TryAcquire(fakeID)

	if !ok {
		t.Errorf("TryAcquire failed: want (%v), got (%v)",
			true, ok)
	}
}

func TestOperationLocks(t *testing.T) {
	t.Parallel()
	volumeID := "test-vol"
	lock := NewOperationLock()
	err := lock.GetCloneLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire clone lock for %s %s", volumeID, err)
	}

	err = lock.GetExpandLock(volumeID)
	if err == nil {
		t.Errorf("expected to fail for GetExpandLock for %s", volumeID)
	}
	lock.ReleaseCloneLock(volumeID)

	// Get multiple clone operation
	err = lock.GetCloneLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire clone lock for %s %s", volumeID, err)
	}
	err = lock.GetCloneLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire clone lock for %s %s", volumeID, err)
	}
	err = lock.GetCloneLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire clone lock for %s %s", volumeID, err)
	}
	// release all clone locks
	lock.ReleaseCloneLock(volumeID)
	lock.ReleaseCloneLock(volumeID)
	lock.ReleaseCloneLock(volumeID)

	// release extra lock it should not cause any issue as the key is already
	// deleted from the map
	lock.ReleaseCloneLock(volumeID)

	// get multiple restore lock
	err = lock.GetRestoreLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire restore lock for %s %s", volumeID, err)
	}
	err = lock.GetRestoreLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire restore lock for %s %s", volumeID, err)
	}
	err = lock.GetRestoreLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire restore lock for %s %s", volumeID, err)
	}
	// release all restore locks
	lock.ReleaseRestoreLock(volumeID)
	lock.ReleaseRestoreLock(volumeID)
	lock.ReleaseRestoreLock(volumeID)

	err = lock.GetSnapshotCreateLock(volumeID)
	if err != nil {
		t.Errorf("failed to acquire createSnapshot lock for %s %s", volumeID, err)
	}
	lock.ReleaseSnapshotCreateLock(volumeID)

	err = lock.GetDeleteLock(volumeID)
	if err != nil {
		t.Errorf("failed to get GetDeleteLock for %s %v", volumeID, err)
	}
	lock.ReleaseDeleteLock(volumeID)
}
