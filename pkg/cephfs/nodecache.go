/*
Copyright 2018 The Kubernetes Authors.

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

package cephfs

import (
	"fmt"
	"sync"
)

type nodeCacheEntry struct {
	volOptions  *volumeOptions
	cephAdminID string
}

type nodeCacheMap map[volumeID]*nodeCacheEntry

var (
	nodeCache    = make(nodeCacheMap)
	nodeCacheMtx sync.Mutex
)

func (m nodeCacheMap) insert(volId volumeID, ent *nodeCacheEntry) {
	nodeCacheMtx.Lock()
	defer nodeCacheMtx.Unlock()

	m[volId] = ent
}

func (m nodeCacheMap) pop(volId volumeID) (*nodeCacheEntry, error) {
	nodeCacheMtx.Lock()
	defer nodeCacheMtx.Unlock()

	ent, ok := m[volId]
	if !ok {
		return nil, fmt.Errorf("node cache entry for volume %s not found", volId)
	}

	delete(m, volId)

	return ent, nil
}
