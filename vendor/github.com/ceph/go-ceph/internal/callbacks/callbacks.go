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
// by a pointer to C code and so instead integer IDs into an internal
// map are used.
// Typically the item being added will either be a callback function or
// a data structure containing a callback function. It is up to the caller
// to control and validate what "callbacks" get used.
type Callbacks struct {
	mutex  sync.RWMutex
	cmap   map[uintptr]interface{}
	lastID uintptr
}

// New returns a new callbacks tracker.
func New() *Callbacks {
	return &Callbacks{cmap: make(map[uintptr]interface{})}
}

// getID returns a unique ID.
// NOTE: cb.mutex must be locked already!
func (cb *Callbacks) getID() uintptr {
	for exists := true; exists; {
		cb.lastID++
		// Sanity check for the very unlikely case of an integer overflow in long
		// running processes.
		_, exists = cb.cmap[cb.lastID]
	}
	return cb.lastID
}

// Add a callback/object to the tracker and return a new ID
// for the object.
func (cb *Callbacks) Add(v interface{}) uintptr {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	id := cb.getID()
	cb.cmap[id] = v
	return id
}

// Remove a callback/object given it's ID.
func (cb *Callbacks) Remove(id uintptr) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	delete(cb.cmap, id)
}

// Lookup returns a mapped callback/object given an ID.
func (cb *Callbacks) Lookup(id uintptr) interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.cmap[id]
}
