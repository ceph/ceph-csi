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
	"errors"
	"fmt"
	"time"

	ca "github.com/ceph/go-ceph/cephfs/admin"
	"github.com/ceph/go-ceph/common/admin/nfs"
	"github.com/ceph/go-ceph/rados"
	ra "github.com/ceph/go-ceph/rbd/admin"
)

type ClusterConnection struct {
	// connection
	conn *rados.Conn

	// FIXME: temporary reference for credentials. Remove this when go-ceph
	// is used for operations.
	Creds *Credentials

	discardOnZeroedWriteSameDisabled bool
}

var (
	// large interval and timeout, it should be longer than the maximum
	// time an operation can take (until refcounting of the connections is
	// available).
	cpInterval = 15 * time.Minute
	cpExpiry   = 10 * time.Minute
	connPool   = NewConnPool(cpInterval, cpExpiry)
)

// rbdVol.Connect() connects to the Ceph cluster and sets rbdVol.conn for further usage.
func (cc *ClusterConnection) Connect(monitors string, cr *Credentials) error {
	if cc.conn == nil {
		conn, err := connPool.Get(monitors, cr.ID, cr.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to get connection: %w", err)
		}

		cc.conn = conn

		// FIXME: remove .Creds from ClusterConnection
		cc.Creds = cr
	}

	return nil
}

func (cc *ClusterConnection) Destroy() {
	if cc.conn != nil {
		connPool.Put(cc.conn)
	}
}

// Copy creates a copy of the ClusterConnection. This is needed when an other
// object needs to use the existing connection.
// It is required to call Destroy() once the (copied) connection is not used
// anymore.
func (cc *ClusterConnection) Copy() *ClusterConnection {
	if cc.conn == nil {
		return nil
	}

	c := ClusterConnection{}
	c.discardOnZeroedWriteSameDisabled = cc.discardOnZeroedWriteSameDisabled
	c.conn = connPool.Copy(cc.conn)
	c.Creds = cc.Creds

	return &c
}

func (cc *ClusterConnection) GetIoctx(pool string) (*rados.IOContext, error) {
	if cc.conn == nil {
		return nil, errors.New("cluster is not connected yet")
	}

	ioctx, err := cc.conn.OpenIOContext(pool)
	if err != nil {
		// ErrNotFound indicates the Pool was not found
		if errors.Is(err, rados.ErrNotFound) {
			err = JoinErrors(ErrPoolNotFound, err)
		} else {
			err = fmt.Errorf("failed to open IOContext for pool %s: %w", pool, err)
		}

		return nil, err
	}

	return ioctx, nil
}

func (cc *ClusterConnection) GetFSAdmin() (*ca.FSAdmin, error) {
	if cc.conn == nil {
		return nil, errors.New("cluster is not connected yet")
	}

	return ca.NewFromConn(cc.conn), nil
}

func (cc *ClusterConnection) GetFSID() (string, error) {
	if cc.conn == nil {
		return "", errors.New("cluster is not connected yet")
	}

	return cc.conn.GetFSID()
}

// GetRBDAdmin get RBDAdmin to administrate rbd volumes.
func (cc *ClusterConnection) GetRBDAdmin() (*ra.RBDAdmin, error) {
	if cc.conn == nil {
		return nil, errors.New("cluster is not connected yet")
	}

	return ra.NewFromConn(cc.conn), nil
}

// GetTaskAdmin returns TaskAdmin to add tasks on rbd images.
func (cc *ClusterConnection) GetTaskAdmin() (*ra.TaskAdmin, error) {
	rbdAdmin, err := cc.GetRBDAdmin()
	if err != nil {
		return nil, err
	}

	return rbdAdmin.Task(), nil
}

// GetNFSAdmin returns an Admin type that can be used to interact with the
// NFS-cluster that is managed by Ceph.
func (cc *ClusterConnection) GetNFSAdmin() (*nfs.Admin, error) {
	if cc.conn == nil {
		return nil, errors.New("cluster is not connected yet")
	}

	return nfs.NewFromConn(cc.conn), nil
}
