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

package lock

import "sync"

// GroupLock implements a simplified version of Group Mutual Exclusion
// for exactly 2 groups. For the general N-group case see:
//
// - Keane, P. and Moir, M. - "A simple local-spin group mutual exclusion algorithm"
// - Link: https://dl.acm.org/doi/epdf/10.1145/301308.301319
//
// GroupLock implements mutual exclusion between two groups of operations.
// Operations within the same group can run concurrently, but operations
// from different groups cannot run simultaneously.
//
// Example use cases:
// - Allowing multiple Stage operations OR multiple Unstage operations, but not both
// - Allowing multiple Read operations OR multiple Write operations, but not both
// - Allowing multiple Connect operations OR multiple Disconnect operations, but not both
//
// This is sometimes called "symmetric reader-writer lock" or "group mutual exclusion".
//
// NOTE:
// GroupLock does not guarantee fairness. Under heavy load from one group,
// the other group may experience temporary delays!!
type GroupLock struct {
	mutex       sync.Mutex
	groupACount int        // Number of active Group A operations
	groupBCount int        // Number of active Group B operations
	groupACond  *sync.Cond // Signals when groupBCount becomes 0
	groupBCond  *sync.Cond // Signals when groupACount becomes 0
}

// NewGroupLock creates a new GroupLock.
func NewGroupLock() *GroupLock {
	gl := &GroupLock{}
	gl.groupACond = sync.NewCond(&gl.mutex)
	gl.groupBCond = sync.NewCond(&gl.mutex)

	return gl
}

// AcquireGroupA acquires the lock for a Group A operation.
// Multiple Group A operations can proceed concurrently, but they will block
// if any Group B operations are active.
func (gl *GroupLock) AcquireGroupA() {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	// Wait while any Group B operations are active
	for gl.groupBCount > 0 {
		// Wait releases the mutex and blocks until Broadcast is called, then re-acquires the mutex before returning
		gl.groupACond.Wait()
	}

	gl.groupACount++
}

// ReleaseGroupA releases the lock for a Group A operation.
func (gl *GroupLock) ReleaseGroupA() {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	gl.groupACount--

	// If this was the last Group A operation, wake up waiting Group B operations
	if gl.groupACount == 0 {
		gl.groupBCond.Broadcast()
	}
}

// AcquireGroupB acquires the lock for a Group B operation.
// Multiple Group B operations can proceed concurrently, but they will block
// if any Group A operations are active.
func (gl *GroupLock) AcquireGroupB() {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	// Wait while any Group A operations are active
	for gl.groupACount > 0 {
		// Wait releases the mutex and blocks until Broadcast is called, then re-acquires the mutex before returning
		gl.groupBCond.Wait()
	}

	gl.groupBCount++
}

// ReleaseGroupB releases the lock for a Group B operation.
func (gl *GroupLock) ReleaseGroupB() {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	gl.groupBCount--

	// If this was the last Group B operation, wake up waiting Group A operations
	if gl.groupBCount == 0 {
		gl.groupACond.Broadcast()
	}
}
