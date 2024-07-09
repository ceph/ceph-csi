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

package rbd

import (
	"context"
	"errors"

	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
)

var _ types.Manager = &rbdManager{}

type rbdManager struct {
	parameters map[string]string
	secrets    map[string]string

	creds *util.Credentials
}

// NewManager returns a new manager for handling Volume and Volume Group
// operations, combining the requests for RBD and the journalling in RADOS.
func NewManager(parameters, secrets map[string]string) types.Manager {
	return &rbdManager{
		parameters: parameters,
		secrets:    secrets,
	}
}

func (mgr *rbdManager) Destroy(ctx context.Context) {
	if mgr.creds != nil {
		mgr.creds.DeleteCredentials()
		mgr.creds = nil
	}
}

// connect sets up credentials and connects to the journal.
func (mgr *rbdManager) connect() error {
	if mgr.creds == nil {
		creds, err := util.NewUserCredentials(mgr.secrets)
		if err != nil {
			return err
		}

		mgr.creds = creds
	}

	return nil
}

func (mgr *rbdManager) GetVolumeByID(ctx context.Context, id string) (types.Volume, error) {
	if err := mgr.connect(); err != nil {
		return nil, err
	}

	volume, err := GenVolFromVolID(ctx, id, mgr.creds, mgr.secrets)
	if err != nil {
		return nil, err
	}

	return volume, nil
}

func (mgr *rbdManager) GetVolumeGroupByID(ctx context.Context, id string) (types.VolumeGroup, error) {
	return nil, errors.New("rbdManager.GetVolumeGroupByID() is not implemented yet")
}

func (mgr *rbdManager) CreateVolumeGroup(ctx context.Context, name string) (types.VolumeGroup, error) {
	return nil, errors.New("rbdManager.CreateVolumeGroup() is not implemented yet")
}

func (mgr *rbdManager) DeleteVolumeGroup(ctx context.Context, vg types.VolumeGroup) error {
	return errors.New("rbdManager.CreateVolumeGroup() is not implemented yet")
}
