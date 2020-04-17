/*
Copyright 2020 The Ceph-CSI Authors.

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
	"io/ioutil"
	"sync"
	"time"

	"github.com/ceph/go-ceph/rados"
	"github.com/pkg/errors"
)

type connEntry struct {
	conn     *rados.Conn
	lastUsed time.Time
	users    int
}

// ConnPool is the struct which contains details of connection entries in the pool and gc controlled params.
type ConnPool struct {
	// interval to run the garbage collector
	interval time.Duration
	// timeout for a connEntry to get garbage collected
	expiry time.Duration
	// Timer used to schedule calls to the garbage collector
	timer *time.Timer
	// Mutex for loading and touching connEntry's from the conns Map
	lock *sync.RWMutex
	// all connEntry's in this pool
	conns map[string]*connEntry
}

// NewConnPool creates a new connection pool instance and start the garbage collector running
// every @interval.
func NewConnPool(interval, expiry time.Duration) *ConnPool {
	cp := ConnPool{
		interval: interval,
		expiry:   expiry,
		lock:     &sync.RWMutex{},
		conns:    make(map[string]*connEntry),
	}
	cp.timer = time.AfterFunc(interval, cp.gc)

	return &cp
}

// loop through all cp.conns and destroy objects that have not been used for cp.expiry.
func (cp *ConnPool) gc() {
	cp.lock.Lock()
	defer cp.lock.Unlock()

	now := time.Now()
	for key, ce := range cp.conns {
		if ce.users == 0 && (now.Sub(ce.lastUsed)) > cp.expiry {
			ce.destroy()
			delete(cp.conns, key)
		}
	}

	// schedule the next gc() run
	cp.timer.Reset(cp.interval)
}

// Destroy stops the garbage collector and destroys all connections in the pool.
func (cp *ConnPool) Destroy() {
	cp.timer.Stop()
	// wait until gc() has finished, in case it is running
	cp.lock.Lock()
	defer cp.lock.Unlock()

	for key, ce := range cp.conns {
		if ce.users != 0 {
			panic("this connEntry still has users, operations" +
				"might still be in-flight")
		}

		ce.destroy()
		delete(cp.conns, key)
	}
}

func (cp *ConnPool) generateUniqueKey(pool, monitors, user, keyfile string) (string, error) {
	// the keyfile can be unique for operations, contents will be the same
	key, err := ioutil.ReadFile(keyfile) // nolint: gosec, #nosec
	if err != nil {
		return "", errors.Wrapf(err, "could not open keyfile %s", keyfile)
	}

	return fmt.Sprintf("%s|%s|%s|%s", pool, monitors, user, string(key)), nil
}

// getExisting returns the existing rados.Conn associated with the unique key.
//
// Requires: locked cp.lock because of ce.get()
func (cp *ConnPool) getConn(unique string) *rados.Conn {
	ce, exists := cp.conns[unique]
	if exists {
		ce.get()
		return ce.conn
	}
	return nil
}

// Get returns a rados.Conn for the given arguments. Creates a new rados.Conn in
// case there is none in the pool. Use the returned unique string to reduce the
// reference count with ConnPool.Put(unique).
func (cp *ConnPool) Get(pool, monitors, user, keyfile string) (*rados.Conn, error) {
	unique, err := cp.generateUniqueKey(pool, monitors, user, keyfile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate unique for connection")
	}

	cp.lock.RLock()
	conn := cp.getConn(unique)
	cp.lock.RUnlock()
	if conn != nil {
		return conn, nil
	}

	// construct and connect a new rados.Conn
	args := []string{"-m", monitors, "--keyfile=" + keyfile}
	conn, err = rados.NewConnWithUser(user)
	if err != nil {
		return nil, errors.Wrapf(err, "creating a new connection failed")
	}
	err = conn.ParseCmdLineArgs(args)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing cmdline args (%v) failed", args)
	}

	err = conn.Connect()
	if err != nil {
		return nil, errors.Wrapf(err, "connecting failed")
	}

	ce := &connEntry{
		conn:     conn,
		lastUsed: time.Now(),
		users:    1,
	}

	cp.lock.Lock()
	defer cp.lock.Unlock()
	oldConn := cp.getConn(unique)
	if oldConn != nil {
		// there was a race, oldConn already exists
		ce.destroy()
		return oldConn, nil
	}
	// this really is a new connection, add it to the map
	cp.conns[unique] = ce

	return conn, nil
}

// Put reduces the reference count of the rados.Conn object that was returned with
// ConnPool.Get().
func (cp *ConnPool) Put(conn *rados.Conn) {
	cp.lock.Lock()
	defer cp.lock.Unlock()

	for _, ce := range cp.conns {
		if ce.conn == conn {
			ce.put()
			return
		}
	}
}

// Add a reference to the connEntry.
// /!\ Only call this while holding the ConnPool.lock.
func (ce *connEntry) get() {
	ce.lastUsed = time.Now()
	ce.users++
}

// Reduce number of references. If this returns true, there are no more users.
// /!\ Only call this while holding the ConnPool.lock.
func (ce *connEntry) put() {
	ce.users--
	// do not call ce.destroy(), let ConnPool.gc() do that
}

// Destroy a connEntry object, close the connection to the Ceph cluster.
func (ce *connEntry) destroy() {
	if ce.conn != nil {
		ce.conn.Shutdown()
		ce.conn = nil
	}
}
