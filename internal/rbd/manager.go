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
	"fmt"

	"github.com/ceph/ceph-csi/internal/journal"
	rbd_group "github.com/ceph/ceph-csi/internal/rbd/group"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

var _ types.Manager = &rbdManager{}

type rbdManager struct {
	// csiID is the instance id of the CSI-driver (driver name).
	csiID string
	// parameters can contain the parameters of a create request.
	parameters map[string]string
	// secrets contain the credentials to connect to the Ceph cluster.
	secrets map[string]string

	// creds are the cached credentials, will be freed on Destroy()
	creds *util.Credentials
	// vgJournal is the journal that is used during opetations, it will be freed on Destroy().
	vgJournal journal.VolumeGroupJournal
}

// NewManager returns a new manager for handling Volume and Volume Group
// operations, combining the requests for RBD and the journalling in RADOS.
func NewManager(csiID string, parameters, secrets map[string]string) types.Manager {
	return &rbdManager{
		csiID:      csiID,
		parameters: parameters,
		secrets:    secrets,
	}
}

func (mgr *rbdManager) Destroy(ctx context.Context) {
	if mgr.creds != nil {
		mgr.creds.DeleteCredentials()
		mgr.creds = nil
	}

	if mgr.vgJournal != nil {
		mgr.vgJournal.Destroy()
		mgr.vgJournal = nil
	}
}

// getCredentials sets up credentials and connects to the journal.
func (mgr *rbdManager) getCredentials() (*util.Credentials, error) {
	if mgr.creds != nil {
		return mgr.creds, nil
	}

	creds, err := util.NewUserCredentials(mgr.secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	mgr.creds = creds

	return creds, nil
}

func (mgr *rbdManager) getVolumeGroupJournal(clusterID string) (journal.VolumeGroupJournal, error) {
	if mgr.vgJournal != nil {
		return mgr.vgJournal, nil
	}

	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	monitors, err := util.Mons(util.CsiConfigFile, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to find MONs for cluster %q: %w", clusterID, err)
	}

	ns, err := util.GetRBDRadosNamespace(util.CsiConfigFile, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to find the RADOS namespace for cluster %q: %w", clusterID, err)
	}

	vgJournalConfig := journal.NewCSIVolumeGroupJournalWithNamespace(mgr.csiID, ns)

	vgJournal, err := vgJournalConfig.Connect(monitors, ns, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to journal: %w", err)
	}

	mgr.vgJournal = vgJournal

	return vgJournal, nil
}

// getGroupUUID checks if a UUID in the volume group journal is already
// reserved. If none is reserved, a new reservation is made. Upon exit of
// getGroupUUID, the function returns:
// 1. the UUID that was reserved
// 2. an undo() function that reverts the reservation (if that succeeded), should be called in a defer
// 3. an error or nil.
func (mgr *rbdManager) getGroupUUID(
	ctx context.Context,
	clusterID, journalPool, name, prefix string,
) (string, func(), error) {
	nothingToUndo := func() {
		// the reservation was not done, no need to undo the reservation
	}

	vgJournal, err := mgr.getVolumeGroupJournal(clusterID)
	if err != nil {
		return "", nothingToUndo, err
	}

	vgsData, err := vgJournal.CheckReservation(ctx, journalPool, name, prefix)
	if err != nil {
		return "", nothingToUndo, fmt.Errorf("failed to check reservation for group %q: %w", name, err)
	}

	var uuid string
	if vgsData != nil && vgsData.GroupUUID != "" {
		uuid = vgsData.GroupUUID
	} else {
		log.DebugLog(ctx, "the journal does not contain a reservation for group %q yet", name)

		uuid, _ /*vgsName*/, err = vgJournal.ReserveName(ctx, journalPool, name, prefix)
		if err != nil {
			return "", nothingToUndo, fmt.Errorf("failed to reserve a UUID for group %q: %w", name, err)
		}
	}

	log.DebugLog(ctx, "got UUID %q for group %q", uuid, name)

	// undo contains the cleanup that should be done by the caller when the
	// reservation was made, and further actions fulfilling the final
	// request failed
	undo := func() {
		err = vgJournal.UndoReservation(ctx, journalPool, uuid, name)
		if err != nil {
			log.ErrorLog(ctx, "failed to undo the reservation for group %q: %w", name, err)
		}
	}

	return uuid, undo, nil
}

func (mgr *rbdManager) GetVolumeByID(ctx context.Context, id string) (types.Volume, error) {
	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	volume, err := GenVolFromVolID(ctx, id, creds, mgr.secrets)
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = fmt.Errorf("volume %s not found: %w", id, err)

			return nil, err
		case errors.Is(err, util.ErrPoolNotFound):
			err = fmt.Errorf("pool %s not found for %s: %w", volume.Pool, id, err)

			return nil, err
		default:
			return nil, fmt.Errorf("failed to get volume from id %q: %w", id, err)
		}
	}

	return volume, nil
}

func (mgr *rbdManager) GetSnapshotByID(ctx context.Context, id string) (types.Snapshot, error) {
	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	snapshot, err := genSnapFromSnapID(ctx, id, creds, mgr.secrets)
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = fmt.Errorf("volume %s not found: %w", id, err)

			return nil, err
		case errors.Is(err, util.ErrPoolNotFound):
			err = fmt.Errorf("pool %s not found for %s: %w", snapshot.Pool, id, err)

			return nil, err
		default:
			return nil, fmt.Errorf("failed to get volume from id %q: %w", id, err)
		}
	}

	return snapshot, nil
}

func (mgr *rbdManager) GetVolumeGroupByID(ctx context.Context, id string) (types.VolumeGroup, error) {
	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	vg, err := rbd_group.GetVolumeGroup(ctx, id, mgr.csiID, creds, mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume group with id %q: %w", id, err)
	}

	return vg, nil
}

func (mgr *rbdManager) CreateVolumeGroup(ctx context.Context, name string) (types.VolumeGroup, error) {
	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	clusterID, err := util.GetClusterID(mgr.parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster-id: %w", err)
	}

	vgJournal, err := mgr.getVolumeGroupJournal(clusterID)
	if err != nil {
		return nil, err
	}

	// pool is a required parameter
	pool, ok := mgr.parameters["pool"]
	if !ok || pool == "" {
		return nil, errors.New("required 'pool' option missing in volume group parameters")
	}

	// journalPool is an optional parameter, use pool if it is not set
	journalPool, ok := mgr.parameters["journalPool"]
	if !ok || journalPool == "" {
		journalPool = pool
	}

	// volumeNamePrefix is an optional parameter, can be an empty string
	prefix := mgr.parameters["volumeNamePrefix"]

	// check if the journal contains a generated name for the group already
	vgData, err := vgJournal.CheckReservation(ctx, journalPool, name, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve volume group for name %q: %w", name, err)
	}

	var uuid string
	if vgData != nil && vgData.GroupUUID != "" {
		uuid = vgData.GroupUUID
	} else {
		log.DebugLog(ctx, "the journal does not contain a reservation for a volume group with name %q yet", name)

		var vgName string
		uuid, vgName, err = vgJournal.ReserveName(ctx, journalPool, name, prefix)
		if err != nil {
			return nil, fmt.Errorf("failed to reserve volume group for name %q: %w", name, err)
		}
		defer func() {
			if err != nil {
				err = vgJournal.UndoReservation(ctx, pool, vgName, name)
				if err != nil {
					log.ErrorLog(ctx, "failed to undo the reservation for volume group %q: %w", name, err)
				}
			}
		}()
	}

	monitors, err := util.Mons(util.CsiConfigFile, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to find MONs for cluster %q: %w", clusterID, err)
	}

	_ /*journalPoolID*/, poolID, err := util.GetPoolIDs(ctx, monitors, journalPool, pool, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a unique CSI volume group with uuid for %q: %w", uuid, err)
	}

	csiID, err := util.GenerateVolID(ctx, monitors, creds, poolID, pool, clusterID, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a unique CSI volume group with uuid for %q: %w", uuid, err)
	}

	vg, err := rbd_group.GetVolumeGroup(ctx, csiID, mgr.csiID, creds, mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume group %q at cluster %q: %w", name, clusterID, err)
	}
	defer func() {
		if err != nil {
			vg.Destroy(ctx)
		}
	}()

	err = vg.Create(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume group %q: %w", name, err)
	}

	return vg, nil
}

func (mgr *rbdManager) GetVolumeGroupSnapshotByID(
	ctx context.Context,
	id string,
) (types.VolumeGroupSnapshot, error) {
	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	vgs, err := rbd_group.GetVolumeGroupSnapshot(ctx, id, mgr.csiID, creds, mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume group with id %q: %w", id, err)
	}

	return vgs, nil
}

func (mgr *rbdManager) GetVolumeGroupSnapshotByName(
	ctx context.Context,
	name string,
) (types.VolumeGroupSnapshot, error) {
	pool, ok := mgr.parameters["pool"]
	if !ok || pool == "" {
		return nil, errors.New("required 'pool' option missing in volume group parameters")
	}

	// groupNamePrefix is an optional parameter, can be an empty string
	prefix := mgr.parameters["groupNamePrefix"]

	clusterID, err := util.GetClusterID(mgr.parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster-id: %w", err)
	}

	uuid, freeUUID, err := mgr.getGroupUUID(ctx, clusterID, pool, name, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get a UUID for volume group snapshot %q: %w", name, err)
	}
	defer func() {
		// no error, no need to undo the reservation
		if err == nil {
			return
		}

		freeUUID()
	}()

	monitors, err := util.Mons(util.CsiConfigFile, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to find MONs for cluster %q: %w", clusterID, err)
	}

	_ /*journalPoolID*/, poolID, err := util.GetPoolIDs(ctx, monitors, pool, pool, mgr.creds)
	if err != nil {
		return nil, fmt.Errorf("failed to get the pool for volume group snapshot with uuid for %q: %w", uuid, err)
	}

	csiID, err := util.GenerateVolID(ctx, monitors, mgr.creds, poolID, pool, clusterID, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a unique CSI volume group with uuid %q: %w", uuid, err)
	}

	vgs, err := rbd_group.GetVolumeGroupSnapshot(ctx, csiID, mgr.csiID, mgr.creds, mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing volume group snapshot with uuid %q: %w", uuid, err)
	}

	snapshots, err := vgs.ListSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots for volume group snapshot %q: %w", vgs, err)
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("volume group snapshot %q is incomplete, it has no snapshots", vgs)
	}

	return vgs, nil
}

func (mgr *rbdManager) CreateVolumeGroupSnapshot(
	ctx context.Context,
	vg types.VolumeGroup,
	name string,
) (types.VolumeGroupSnapshot, error) {
	pool, err := vg.GetPool(ctx)
	if err != nil {
		return nil, err
	}

	// groupNamePrefix is an optional parameter, can be an empty string
	prefix := mgr.parameters["groupNamePrefix"]

	clusterID, err := vg.GetClusterID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster id for volume group snapshot %q: %w", vg, err)
	}

	uuid, freeUUID, err := mgr.getGroupUUID(ctx, clusterID, pool, name, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get a UUID for volume group snapshot %q: %w", vg, err)
	}
	defer func() {
		// no error, no need to undo the reservation
		if err == nil {
			return
		}

		freeUUID()
	}()

	monitors, err := util.Mons(util.CsiConfigFile, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to find MONs for cluster %q: %w", clusterID, err)
	}

	_ /*journalPoolID*/, poolID, err := util.GetPoolIDs(ctx, monitors, pool, pool, mgr.creds)
	if err != nil {
		return nil, fmt.Errorf("failed to get PoolID for %q: %w", pool, err)
	}

	groupID, err := util.GenerateVolID(ctx, monitors, mgr.creds, poolID, pool, clusterID, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a unique CSI volume group with uuid for %q: %w", uuid, err)
	}

	vgs, err := rbd_group.GetVolumeGroupSnapshot(ctx, groupID, mgr.csiID, mgr.creds, mgr)
	if vgs != nil {
		log.DebugLog(ctx, "found existing volume group snapshot %q for id %q", vgs, groupID)

		// validate the contents of the vgs
		snapshots, vgsErr := vgs.ListSnapshots(ctx)
		if vgsErr != nil {
			return nil, fmt.Errorf("failed to list snapshots of existing volume group snapshot %q: %w", vgs, vgsErr)
		}

		volumes, vgErr := vg.ListVolumes(ctx)
		if vgErr != nil {
			return nil, fmt.Errorf("failed to list volumes of volume group %q: %w", vg, vgErr)
		}

		// return the existing vgs if the contents matches
		// TODO: improve contents verification, length is a very minimal check
		if len(snapshots) == len(volumes) {
			log.DebugLog(ctx, "existing volume group snapshot %q contains %d snapshots", vgs, len(snapshots))

			return vgs, nil
		}
	} else if err != nil && !errors.Is(ErrImageNotFound, err) {
		// ErrImageNotFound can be returned if the VolumeGroupSnapshot
		// could not be found. It is expected that it does not exist
		// yet, in which case it will be created below.
		return nil, fmt.Errorf("failed to check for existing volume group snapshot with id %q: %w", groupID, err)
	}

	snapshots, err := vg.CreateSnapshots(ctx, mgr.creds, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume group snapshot %q: %w", name, err)
	}
	defer func() {
		// cleanup created snapshots in case there was a failure
		if err == nil {
			return
		}

		for _, snap := range snapshots {
			delErr := snap.Delete(ctx)
			if delErr != nil {
				log.ErrorLog(ctx, "failed to delete snapshot %q: %v", snap, delErr)
			}
		}
	}()

	log.DebugLog(ctx, "volume group snapshot %q contains %d snapshots: %v", name, len(snapshots), snapshots)

	vgs, err = rbd_group.NewVolumeGroupSnapshot(ctx, groupID, mgr.csiID, mgr.creds, snapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to create new volume group snapshot %q: %w", name, err)
	}

	log.DebugLog(ctx, "volume group snapshot %q has been created", vgs)

	return vgs, nil
}
