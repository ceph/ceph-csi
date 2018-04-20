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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/golang/glog"
)

const (
	volumeCacheRoot = PluginFolder + "/controller/volume-cache"
)

type volumeCacheEntry struct {
	VolOptions volumeOptions
	Identifier volumeIdentifier
}

type volumeCache struct {
	entries map[string]*volumeCacheEntry
}

var (
	volCache    volumeCache
	volCacheMtx sync.RWMutex
)

// Loads all .json files from volumeCacheRoot into volCache
// Called from driver.go's Run()
func loadVolumeCache() error {
	cacheDir, err := ioutil.ReadDir(volumeCacheRoot)
	if err != nil {
		return fmt.Errorf("cannot read volume cache: %v", err)
	}

	volCacheMtx.Lock()
	defer volCacheMtx.Unlock()

	volCache.entries = make(map[string]*volumeCacheEntry)

	for _, fi := range cacheDir {
		if !strings.HasSuffix(fi.Name(), ".json") || !fi.Mode().IsRegular() {
			continue
		}

		f, err := os.Open(path.Join(volumeCacheRoot, fi.Name()))
		if err != nil {
			glog.Errorf("cephfs: couldn't read '%s' from volume cache: %v", fi.Name(), err)
			continue
		}

		d := json.NewDecoder(f)
		ent := &volumeCacheEntry{}

		if err = d.Decode(ent); err != nil {
			glog.Errorf("cephfs: failed to parse '%s': %v", fi.Name(), err)
		} else {
			volCache.entries[ent.Identifier.uuid] = ent
		}

		f.Close()
	}

	return nil
}

func getVolumeCacheEntryPath(volUuid string) string {
	return path.Join(volumeCacheRoot, fmt.Sprintf("vol-%s.json", volUuid))
}

func (vc *volumeCache) insert(ent *volumeCacheEntry) error {
	filePath := getVolumeCacheEntryPath(ent.Identifier.uuid)

	volCacheMtx.Lock()
	defer volCacheMtx.Unlock()

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("couldn't create cache entry file %s: %v", filePath, err)
	}
	defer f.Close()

	e := json.NewEncoder(f)
	if err = e.Encode(ent); err != nil {
		return fmt.Errorf("failed to encode cache entry for volume %s: %v", ent.Identifier.id, err)
	}

	vc.entries[ent.Identifier.uuid] = ent

	return nil
}

func (vc *volumeCache) erase(volUuid string) error {
	volCacheMtx.Lock()
	delete(vc.entries, volUuid)
	volCacheMtx.Unlock()

	return os.Remove(getVolumeCacheEntryPath(volUuid))
}

func (vc *volumeCache) get(volUuid string) (volumeCacheEntry, bool) {
	volCacheMtx.RLock()
	defer volCacheMtx.RUnlock()

	if ent, ok := vc.entries[volUuid]; ok {
		return *ent, true
	} else {
		return volumeCacheEntry{}, false
	}
}
