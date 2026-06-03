/*
Copyright 2019 The Kubernetes Authors.
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
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// VolumeOperationAlreadyExistsFmt string format to return for concurrent operation.
	VolumeOperationAlreadyExistsFmt = "an operation with the given Volume ID %s already exists"

	// SnapshotOperationAlreadyExistsFmt string format to return for concurrent operation.
	SnapshotOperationAlreadyExistsFmt = "an operation with the given Snapshot ID %s already exists"

	// TargetPathOperationAlreadyExistsFmt string format to return for concurrent operation on target path.
	TargetPathOperationAlreadyExistsFmt = "an operation with the given target path %s already exists"

	// HostOperationAlreadyExistsFmt is used for reporting an in-progress operation that modifies the
	// NVMe-oF gateway configuration for a prticular host.
	HostOperationAlreadyExistsFmt = "an operation that modifies the gateway for Host ID %s already exists"
)

// IDLocker implements a map with atomic operations. It stores an ID/key in a set while the lock is held
// during an operation that needs exclusive access to resources.
type IDLocker struct {
	locks sets.Set[string]
	mux   sync.Mutex
}

// NewIDLocker creates a new IDLocker.
func NewIDLocker() *IDLocker {
	return &IDLocker{
		locks: sets.New[string](),
	}
}

// TryAcquire tries to acquire the lock on the ID and returns true if successful.
// If another operation is already using the ID, returns false.
func (vl *IDLocker) TryAcquire(id string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	if vl.locks.Has(id) {
		return false
	}
	vl.locks.Insert(id)

	return true
}

// Release deletes the lock on an ID.
func (vl *IDLocker) Release(id string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	vl.locks.Delete(id)
}

type operation string

const (
	createOp  operation = "create"
	deleteOp  operation = "delete"
	cloneOpt  operation = "clone"
	restoreOp operation = "restore"
	expandOp  operation = "expand"
	modifyOp  operation = "modify"
)

// OperationLock implements a map with atomic operations.
type OperationLock struct {
	// lock is a map of map, internal key is the list of id and its counters
	// and the outer map key is the operation type it will be one of the above
	// const
	//
	// example map[restore][xxx-xxx-xxx-xxx]1
	// map[restore][xxx-xxx-xxx-xxx]2
	// the counter value will be increased for allowed parallel operations and
	// it will be decreased when the operation is completed, when the counter
	// value goes to zero the `xxx-xxx-xxx` key will be removed from the
	// operation map.
	locks map[operation]map[string]int
	// lock to avoid concurrent operation on map
	mux sync.Mutex
}

// NewOperationLock returns new OperationLock.
func NewOperationLock() *OperationLock {
	lock := make(map[operation]map[string]int)
	lock[createOp] = make(map[string]int)
	lock[deleteOp] = make(map[string]int)
	lock[cloneOpt] = make(map[string]int)
	lock[restoreOp] = make(map[string]int)
	lock[expandOp] = make(map[string]int)
	lock[modifyOp] = make(map[string]int)

	return &OperationLock{
		locks: lock,
	}
}

// conflictMatrix defines which operations conflict with each other.
// The key is the operation being attempted, and the value is a list of
// operations that cannot be in progress.
var conflictMatrix = map[operation][]operation{
	cloneOpt:  {expandOp, modifyOp},
	deleteOp:  {expandOp, restoreOp, modifyOp},
	restoreOp: {deleteOp},
	expandOp:  {deleteOp, cloneOpt, createOp, modifyOp},
	modifyOp:  {deleteOp, cloneOpt, createOp, expandOp},
	// createOp has no conflicts according to the original logic.
}

// GetSnapshotCreateLock gets the snapshot lock on given volumeID.
func (ol *OperationLock) GetSnapshotCreateLock(volumeID string) error {
	return ol.tryAcquire(createOp, volumeID)
}

// GetCloneLock gets the clone lock on given volumeID.
func (ol *OperationLock) GetCloneLock(volumeID string) error {
	return ol.tryAcquire(cloneOpt, volumeID)
}

// GetDeleteLock gets the delete lock on given volumeID,ensures that there is
// no clone,restore and expand operation on given volumeID.
func (ol *OperationLock) GetDeleteLock(volumeID string) error {
	return ol.tryAcquire(deleteOp, volumeID)
}

// GetRestoreLock gets the restore lock on given volumeID,ensures that there is
// no delete operation on given volumeID.
func (ol *OperationLock) GetRestoreLock(volumeID string) error {
	return ol.tryAcquire(restoreOp, volumeID)
}

// GetExpandLock gets the expand lock on given volumeID,ensures that there is
// no delete and clone operation on given volumeID.
func (ol *OperationLock) GetExpandLock(volumeID string) error {
	return ol.tryAcquire(expandOp, volumeID)
}

func (ol *OperationLock) GetModifyLock(volumeID string) error {
	return ol.tryAcquire(modifyOp, volumeID)
}

// ReleaseSnapshotCreateLock releases the create lock on given volumeID.
func (ol *OperationLock) ReleaseSnapshotCreateLock(volumeID string) {
	ol.release(createOp, volumeID)
}

// ReleaseCloneLock releases the clone lock on given volumeID.
func (ol *OperationLock) ReleaseCloneLock(volumeID string) {
	ol.release(cloneOpt, volumeID)
}

// ReleaseDeleteLock releases the delete lock on given volumeID.
func (ol *OperationLock) ReleaseDeleteLock(volumeID string) {
	ol.release(deleteOp, volumeID)
}

// ReleaseRestoreLock releases the restore lock on given volumeID.
func (ol *OperationLock) ReleaseRestoreLock(volumeID string) {
	ol.release(restoreOp, volumeID)
}

// ReleaseExpandLock releases the expand lock on given volumeID.
func (ol *OperationLock) ReleaseExpandLock(volumeID string) {
	ol.release(expandOp, volumeID)
}

func (ol *OperationLock) ReleaseModifyLock(volumeID string) {
	ol.release(modifyOp, volumeID)
}

// tryAcquire tries to acquire the lock for operating on volumeID and returns true if successful.
// If another operation is already using volumeID, returns false.
func (ol *OperationLock) tryAcquire(op operation, volumeID string) error {
	ol.mux.Lock()
	defer ol.mux.Unlock()
	if conflictingOps, ok := conflictMatrix[op]; ok {
		for _, conflictingOp := range conflictingOps {
			if _, exists := ol.locks[conflictingOp][volumeID]; exists {
				return fmt.Errorf("cannot acquire lock for %q, "+
					"an %q operation with given id %s already exists",
					op, conflictingOp, volumeID)
			}
		}
	}
	switch op {
	case createOp, cloneOpt, restoreOp:
		// These operations are counters.
		ol.locks[op][volumeID]++
	case deleteOp, expandOp, modifyOp:
		// These operations are flags (presence check).
		ol.locks[op][volumeID] = 1
	default:
		return fmt.Errorf("%v operation not supported", op)
	}

	return nil
}

// release deletes the lock on volumeID.
func (ol *OperationLock) release(op operation, volumeID string) {
	ol.mux.Lock()
	defer ol.mux.Unlock()
	switch op {
	case cloneOpt, createOp, expandOp, restoreOp, deleteOp, modifyOp:
		if val, ok := ol.locks[op][volumeID]; ok {
			// decrement the counter for operation
			ol.locks[op][volumeID] = val - 1
			if ol.locks[op][volumeID] == 0 {
				delete(ol.locks[op], volumeID)
			}
		}
	default:
		log.ErrorLogMsg("%v operation not supported", op)
	}
}
