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

	"github.com/ceph/ceph-csi/internal/util/log"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// VolumeOperationAlreadyExistsFmt string format to return for concurrent operation.
	VolumeOperationAlreadyExistsFmt = "an operation with the given Volume ID %s already exists"

	// SnapshotOperationAlreadyExistsFmt string format to return for concurrent operation.
	SnapshotOperationAlreadyExistsFmt = "an operation with the given Snapshot ID %s already exists"
)

// VolumeLocks implements a map with atomic operations. It stores a set of all volume IDs
// with an ongoing operation.
type VolumeLocks struct {
	locks sets.Set[string]
	mux   sync.Mutex
}

// NewVolumeLocks returns new VolumeLocks.
func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{
		locks: sets.New[string](),
	}
}

// TryAcquire tries to acquire the lock for operating on volumeID and returns true if successful.
// If another operation is already using volumeID, returns false.
func (vl *VolumeLocks) TryAcquire(volumeID string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	if vl.locks.Has(volumeID) {
		return false
	}
	vl.locks.Insert(volumeID)

	return true
}

// Release deletes the lock on volumeID.
func (vl *VolumeLocks) Release(volumeID string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	vl.locks.Delete(volumeID)
}

type operation string

const (
	createOp  operation = "create"
	deleteOp  operation = "delete"
	cloneOpt  operation = "clone"
	restoreOp operation = "restore"
	expandOp  operation = "expand"
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

	return &OperationLock{
		locks: lock,
	}
}

// tryAcquire tries to acquire the lock for operating on volumeID and returns true if successful.
// If another operation is already using volumeID, returns false.
func (ol *OperationLock) tryAcquire(op operation, volumeID string) error {
	ol.mux.Lock()
	defer ol.mux.Unlock()
	switch op {
	case createOp:
		// snapshot controller make sure the pvc which is the source for the
		// snapshot request won't get deleted while snapshot is getting created,
		// so we dont need to check for any ongoing delete operation here on the
		// volume.
		// increment the counter for snapshot create operation
		val := ol.locks[createOp][volumeID]
		ol.locks[createOp][volumeID] = val + 1
	case cloneOpt:
		// During clone operation, controller make sure no pvc deletion happens on the
		// referred PVC datasource, so we are safe from source PVC delete.

		// Check any expand operation is going on for given volume ID.
		// if yes we need to return an error to avoid issues.
		if _, ok := ol.locks[expandOp][volumeID]; ok {
			return fmt.Errorf("an Expand operation with given id %s already exists", volumeID)
		}
		// increment the counter for clone operation
		val := ol.locks[cloneOpt][volumeID]
		ol.locks[cloneOpt][volumeID] = val + 1
	case deleteOp:
		// During delete operation the volume should not be under expand,
		// check any expand operation is going on for given volume ID
		if _, ok := ol.locks[expandOp][volumeID]; ok {
			return fmt.Errorf("an Expand operation with given id %s already exists", volumeID)
		}
		// check any restore operation is going on for given volume ID
		if _, ok := ol.locks[restoreOp][volumeID]; ok {
			return fmt.Errorf("a Restore operation with given id %s already exists", volumeID)
		}
		ol.locks[deleteOp][volumeID] = 1
	case restoreOp:
		// During restore operation the volume should not be deleted
		// check any delete operation is going on for given volume ID
		if _, ok := ol.locks[deleteOp][volumeID]; ok {
			return fmt.Errorf("a Delete operation with given id %s already exists", volumeID)
		}
		// increment the counter for restore operation
		val := ol.locks[restoreOp][volumeID]
		ol.locks[restoreOp][volumeID] = val + 1
	case expandOp:
		// During expand operation the volume should not be deleted or cloned
		// and there should not be a create operation also.
		// check any delete operation is going on for given volume ID
		if _, ok := ol.locks[deleteOp][volumeID]; ok {
			return fmt.Errorf("a Delete operation with given id %s already exists", volumeID)
		}
		// check any clone operation is going on for given volume ID
		if _, ok := ol.locks[cloneOpt][volumeID]; ok {
			return fmt.Errorf("a Clone operation with given id %s already exists", volumeID)
		}
		// check any delete operation is going on for given volume ID
		if _, ok := ol.locks[createOp][volumeID]; ok {
			return fmt.Errorf("a Create operation with given id %s already exists", volumeID)
		}

		ol.locks[expandOp][volumeID] = 1
	default:
		return fmt.Errorf("%v operation not supported", op)
	}

	return nil
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

// release deletes the lock on volumeID.
func (ol *OperationLock) release(op operation, volumeID string) {
	ol.mux.Lock()
	defer ol.mux.Unlock()
	switch op {
	case cloneOpt, createOp, expandOp, restoreOp, deleteOp:
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
