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
	"sync"
)

/*
IDLock is a per identifier lock with a use counter that retains a number of users of the lock.
IDLocker is a map of IDLocks holding the IDLocks based on a passed in identifier.
Typical usage (post creating an IDLocker) is to Lock/Unlock based on identifiers as per the API.
*/
type (
	IDLock struct {
		mtx      sync.Mutex
		useCount int
	}

	IDLocker struct {
		lMutex sync.Mutex
		lMap   map[string]*IDLock
	}
)

func NewIDLocker() *IDLocker {
	return &IDLocker{
		lMap: make(map[string]*IDLock),
	}
}

func (lkr *IDLocker) Lock(identifier string) *IDLock {
	var (
		lk *IDLock
		ok bool
	)

	newlk := new(IDLock)

	lkr.lMutex.Lock()

	if lk, ok = lkr.lMap[identifier]; !ok {
		lk = newlk
		lkr.lMap[identifier] = lk
	}
	lk.useCount++
	lkr.lMutex.Unlock()

	lk.mtx.Lock()

	return lk
}

func (lkr *IDLocker) Unlock(lk *IDLock, identifier string) {
	lk.mtx.Unlock()

	lkr.lMutex.Lock()
	lk.useCount--
	if lk.useCount == 0 {
		delete(lkr.lMap, identifier)
	}
	lkr.lMutex.Unlock()
}
