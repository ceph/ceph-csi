package callbacks

import (
	"sync"
)

// The logic of this file is largely adapted from:
// https://github.com/golang/go/wiki/cgo#function-variables
//
// Also helpful:
// https://eli.thegreenplace.net/2019/passing-callbacks-and-pointers-to-cgo/

// Callbacks provides a tracker for data that is to be passed between Go
// and C callback functions. The Go callback/object may not be passed
// by a pointer to C code and so instead integer indexes into an internal
// map are used.
// Typically the item being added will either be a callback function or
// a data structure containing a callback function. It is up to the caller
// to control and validate what "callbacks" get used.
type Callbacks struct {
	mutex sync.RWMutex
	cmap  map[int]interface{}
}

// New returns a new callbacks tracker.
func New() *Callbacks {
	return &Callbacks{cmap: make(map[int]interface{})}
}

// Add a callback/object to the tracker and return a new index
// for the object.
func (cb *Callbacks) Add(v interface{}) int {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	// this approach assumes that there are typically very few callbacks
	// in play at once and can just use the length of the map as our
	// index. But in case of collisions we fall back to simply incrementing
	// until we find a free key like in the cgo wiki page.
	// If this code ever becomes a hot path there's surely plenty of room
	// for optimization in the future :-)
	index := len(cb.cmap) + 1
	for {
		if _, found := cb.cmap[index]; !found {
			break
		}
		index++
	}
	cb.cmap[index] = v
	return index
}

// Remove a callback/object given it's index.
func (cb *Callbacks) Remove(index int) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	delete(cb.cmap, index)
}

// Lookup returns a mapped callback/object given an index.
func (cb *Callbacks) Lookup(index int) interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.cmap[index]
}
