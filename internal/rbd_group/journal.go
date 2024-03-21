/*
Copyright 2024 The Ceph-CSI Authors.

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

package rbd_group

import (
	"context"
	"errors"

	"github.com/ceph/go-ceph/rados"

	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"
)

type groupObject struct {
	clusterID string

	credentials *util.Credentials
	secrets     map[string]string
	monitors    string
	pool        string

	// temporary connection attributes
	conn  *util.ClusterConnection
	ioctx *rados.IOContext

	// journalling related attributes
	journal     journal.VolumeGroupJournal
	journalPool string

	// id is a unique value for this volume group in the Ceph cluster, it
	// is used to find the group in the journal.
	id string

	// name is used in RBD API calls as the name of this object
	name string
}

func (obj *groupObject) Destroy(ctx context.Context) {
	if obj.journal != nil {
		obj.journal.Destroy()
		obj.journal = nil
	}

	if obj.credentials != nil {
		obj.credentials.DeleteCredentials()
		obj.credentials = nil
	}
}

func (obj *groupObject) resolveByID(ctx context.Context, id string, secrets map[string]string) error {
	csiID := util.CSIIdentifier{}

	err := csiID.DecomposeCSIID(id)
	if err != nil {
		return err
	}

	mons, _, err := util.GetMonsAndClusterID(ctx, csiID.ClusterID, false)
	if err != nil {
		return err
	}

	namespace, err := util.GetRadosNamespace(util.CsiConfigFile, csiID.ClusterID)
	if err != nil {
		return err
	}

	obj.clusterID = csiID.ClusterID
	obj.monitors = mons
	obj.secrets = secrets
	obj.id = id

	obj.credentials, err = util.NewUserCredentials(secrets)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			obj.Destroy(ctx)
		}
	}()

	pool, err := util.GetPoolName(mons, obj.credentials, csiID.LocationID)
	if err != nil {
		return err
	}

	err = obj.SetJournalNamespace(ctx, pool, namespace)
	if err != nil {
		return err
	}

	return nil
}

// SetMonitors connects to the Ceph cluster.
func (obj *groupObject) SetMonitors(ctx context.Context, monitors string) error {
	conn := &util.ClusterConnection{}
	err := conn.Connect(monitors, obj.credentials)
	if err != nil {
		return err
	}

	obj.conn = conn
	obj.monitors = monitors

	return nil
}

// SetPool uses the connection to the Ceph cluster to create an IOContext to
// the pool.
func (obj *groupObject) SetPool(ctx context.Context, pool string) error {
	if obj.conn == nil {
		return ErrRBDGroupNotConnected
	}

	ioctx, err := obj.conn.GetIoctx(pool)
	if err != nil {
		return err
	}

	obj.pool = pool
	obj.ioctx = ioctx

	return nil
}

func (obj *groupObject) SetJournalNamespace(ctx context.Context, pool, namespace string) error {
	if obj.conn == nil {
		return ErrRBDGroupNotConnected
	}

	vgj := journal.NewCSIVolumeGroupJournal(groupSuffix)
	vgj.SetNamespace(namespace)
	err := vgj.Connect(obj.monitors, namespace, obj.credentials)
	if err != nil {
		return err
	}

	obj.journal = vgj
	obj.journalPool = pool

	return nil
}

func (obj *groupObject) GetID(ctx context.Context) (string, error) {
	if obj.id == "" {
		return "", errors.New("BUG: ID is no set")
	}

	return obj.id, nil
}
