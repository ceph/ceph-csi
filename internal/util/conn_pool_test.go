/*
Copyright 2020 ceph-csi authors.

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
	"os"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rados"
)

const (
	interval = 15 * time.Minute
	expiry   = 10 * time.Minute
)

// fakeGet is used as a replacement for ConnPool.Get and does not need a
// working Ceph cluster to connect to.
//
// This is mostly a copy of ConnPool.Get().
func (cp *ConnPool) fakeGet(monitors, user, keyfile string) (*rados.Conn, string, error) {
	unique, err := cp.generateUniqueKey(monitors, user, keyfile)
	if err != nil {
		return nil, "", err
	}

	// need a lock while calling ce.touch()
	cp.lock.RLock()
	conn := cp.getConn(unique)
	cp.lock.RUnlock()
	if conn != nil {
		return conn, unique, nil
	}

	// cp.Get() creates and connects a rados.Conn here
	conn, err = rados.NewConn()
	if err != nil {
		return nil, "", err
	}

	ce := &connEntry{
		conn:     conn,
		lastUsed: time.Now(),
		users:    1,
	}

	cp.lock.Lock()
	defer cp.lock.Unlock()
	if oldConn := cp.getConn(unique); oldConn != nil {
		// there was a race, oldConn already exists
		ce.destroy()

		return oldConn, unique, nil
	}
	// this really is a new connection, add it to the map
	cp.conns[unique] = ce

	return conn, unique, nil
}

//nolint:paralleltest // these tests cannot run in parallel
func TestConnPool(t *testing.T) {
	cp := NewConnPool(interval, expiry)
	defer cp.Destroy()

	// create a keyfile with some contents
	keyfile := "/tmp/conn_utils.keyfile"
	err := os.WriteFile(keyfile, []byte("the-key"), 0o600)
	if err != nil {
		t.Errorf("failed to create keyfile: %v", err)

		return
	}
	defer os.Remove(keyfile)

	var conn *rados.Conn
	var unique string

	t.Run("fakeGet", func(t *testing.T) {
		conn, unique, err = cp.fakeGet("monitors", "user", keyfile)
		if err != nil {
			t.Errorf("failed to get connection: %v", err)
		}
		// prevent goanalysis_metalinter from complaining about unused conn
		_ = conn

		// there should be a single item in cp.conns
		if len(cp.conns) != 1 {
			t.Errorf("there is more than a single conn in cp.conns: %v", len(cp.conns))
		}

		// the ce should have a single user
		ce, exists := cp.conns[unique]
		if !exists {
			t.Errorf("getting the conn from cp.conns failed")
		}
		if ce.users != 1 {
			t.Errorf("there should only be one user: %v", ce.users)
		}
	})

	t.Run("doubleFakeGet", func(t *testing.T) {
		// after a 2nd get, there should still be a single conn in cp.conns
		_, _, err = cp.fakeGet("monitors", "user", keyfile)
		if err != nil {
			t.Errorf("failed to get connection: %v", err)
		}
		if len(cp.conns) != 1 {
			t.Errorf("a second conn was added to cp.conns: %v", len(cp.conns))
		}

		// the ce should have a two users
		ce, exists := cp.conns[unique]
		if !exists {
			t.Errorf("getting the conn from cp.conns failed")
		}
		if ce.users != 2 {
			t.Errorf("there should be two users: %v", ce.users)
		}

		cp.Put(ce.conn)
		if len(cp.conns) != 1 {
			t.Errorf("a single put should not remove all cp.conns: %v", len(cp.conns))
		}
		// the ce should have a single user again
		ce, exists = cp.conns[unique]
		if !exists {
			t.Errorf("getting the conn from cp.conns failed")
		}
		if ce.users != 1 {
			t.Errorf("There should only be one user: %v", ce.users)
		}
	})

	// there is still one conn in cp.conns after "doubleFakeGet"
	t.Run("garbageCollection", func(t *testing.T) {
		// timeout has not occurred yet, so number of conns in the list should stay the same
		cp.gc()
		if len(cp.conns) != 1 {
			t.Errorf("gc() should not have removed any entry from cp.conns: %v", len(cp.conns))
		}

		// force expiring the ConnEntry by fetching it and adjusting .lastUsed
		ce, exists := cp.conns[unique]
		if !exists {
			t.Error("getting the conn from cp.conns failed")
		}
		ce.lastUsed = ce.lastUsed.Add(-2 * expiry)

		if ce.users != 1 {
			t.Errorf("There should only be one user: %v", ce.users)
		}
		cp.Put(ce.conn)
		if ce.users != 0 {
			t.Errorf("There should be no users anymore: %v", ce.users)
		}

		// timeout has occurred now, so number of conns in the list should be less
		cp.gc()
		if len(cp.conns) != 0 {
			t.Errorf("gc() should have removed an entry from cp.conns: %v", len(cp.conns))
		}
	})
}
