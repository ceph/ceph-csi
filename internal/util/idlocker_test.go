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
