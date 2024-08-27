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

	"github.com/ceph/go-ceph/rados"

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

	ns, err := util.GetRadosNamespace(util.CsiConfigFile, clusterID)
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

func (mgr *rbdManager) GetVolumeGroupByID(ctx context.Context, id string) (types.VolumeGroup, error) {
	vi := &util.CSIIdentifier{}
	if err := vi.DecomposeCSIID(id); err != nil {
		return nil, fmt.Errorf("failed to parse volume group id %q: %w", id, err)
	}

	vgJournal, err := mgr.getVolumeGroupJournal(vi.ClusterID)
	if err != nil {
		return nil, err
	}

	creds, err := mgr.getCredentials()
	if err != nil {
		return nil, err
	}

	vg, err := rbd_group.GetVolumeGroup(ctx, id, vgJournal, creds, mgr)
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
		uuid, vgName, err = vgJournal.ReserveName(ctx, journalPool, name, vgData.GroupUUID, prefix)
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

	vg, err := rbd_group.GetVolumeGroup(ctx, csiID, vgJournal, creds, mgr)
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

func (mgr *rbdManager) DeleteVolumeGroup(ctx context.Context, vg types.VolumeGroup) error {
	err := vg.Delete(ctx)
	if err != nil && !errors.Is(rados.ErrNotFound, err) {
		return fmt.Errorf("failed to delete volume group %q: %w", vg, err)
	}

	clusterID, err := vg.GetClusterID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster id for volume group %q: %w", vg, err)
	}

	vgJournal, err := mgr.getVolumeGroupJournal(clusterID)
	if err != nil {
		return err
	}

	name, err := vg.GetName(ctx)
	if err != nil {
		return fmt.Errorf("failed to get name for volume group %q: %w", vg, err)
	}

	csiID, err := vg.GetID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get id for volume group %q: %w", vg, err)
	}

	pool, err := vg.GetPool(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pool for volume group %q: %w", vg, err)
	}

	err = vgJournal.UndoReservation(ctx, pool, name, csiID)
	if err != nil /* TODO? !errors.Is(..., err) */ {
		return fmt.Errorf("failed to undo the reservation for volume group %q: %w", vg, err)
	}

	return nil
}

// RegenerateVolumeGroupJournal regenerate the omap data for the volume group.
// This performs the following operations:
//   - extracts clusterID and Mons from the cluster mapping
//   - Retrieves pool and journalPool parameters from the VolumeGroupReplicationClass
//   - Reserves omap data
//   - Add volumeIDs mapping to the reserved volume group omap object
//   - Generate new volume group handler
//
// Returns the generated volume group handle.
//
// Note: The new volume group handler will differ from the original as it includes
// poolID and clusterID, which vary between clusters.
func (mgr *rbdManager) RegenerateVolumeGroupJournal(
	ctx context.Context,
	groupID, requestName string,
	volumeIds []string,
) (string, error) {
	var (
		clusterID   string
		monitors    string
		pool        string
		journalPool string
		namePrefix  string
		groupUUID   string
		vgName      string

		gi  util.CSIIdentifier
		ok  bool
		err error
	)

	err = gi.DecomposeCSIID(groupID)
	if err != nil {
		return "", fmt.Errorf("%w: error decoding volume group ID (%w) (%s)", ErrInvalidVolID, err, groupID)
	}

	monitors, clusterID, err = util.FetchMappedClusterIDAndMons(ctx, gi.ClusterID)
	if err != nil {
		return "", err
	}

	pool, ok = mgr.parameters["pool"]
	if !ok {
		return "", errors.New("required 'pool' parameter missing in parameters")
	}

	journalPool = mgr.parameters["journalPool"]
	if journalPool == "" {
		journalPool = pool
	}

	vgJournal, err := mgr.getVolumeGroupJournal(clusterID)
	if err != nil {
		return "", err
	}
	defer vgJournal.Destroy()

	namePrefix = mgr.parameters["volumeNamePrefix"]
	vgData, err := vgJournal.CheckReservation(ctx, journalPool, requestName, namePrefix)
	if err != nil {
		return "", err
	}

	if vgData != nil {
		groupUUID = vgData.GroupUUID
		vgName = vgData.GroupName
	} else {
		log.DebugLog(ctx, "the journal does not contain a reservation for a volume group with name %q yet", requestName)
		groupUUID, vgName, err = vgJournal.ReserveName(ctx, journalPool, requestName, gi.ObjectUUID, namePrefix)
		if err != nil {
			return "", fmt.Errorf("failed to reserve volume group for name %q: %w", requestName, err)
		}
		defer func() {
			if err != nil {
				err = vgJournal.UndoReservation(ctx, journalPool, vgName, requestName)
				if err != nil {
					log.ErrorLog(ctx, "failed to undo the reservation for volume group %q: %w", requestName, err)
				}
			}
		}()
	}

	volumes := make([]types.Volume, len(volumeIds))
	defer func() {
		for _, v := range volumes {
			v.Destroy(ctx)
		}
	}()
	var volume types.Volume
	for i, id := range volumeIds {
		volume, err = mgr.GetVolumeByID(ctx, id)
		if err != nil {
			return "", fmt.Errorf("failed to find required volume %q for volume group id %q: %w", id, vgName, err)
		}

		volumes[i] = volume
	}

	var volID string
	for _, vol := range volumes {
		volID, err = vol.GetID(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get VolumeID for %q: %w", vol, err)
		}

		toAdd := map[string]string{
			volID: "",
		}
		log.DebugLog(ctx, "adding volume mapping for volume %q to volume group %q", volID, vgName)
		err = mgr.vgJournal.AddVolumesMapping(ctx, pool, gi.ObjectUUID, toAdd)
		if err != nil {
			return "", fmt.Errorf("failed to add mapping for volume %q to volume group %q: %w", volID, vgName, err)
		}
	}

	_, poolID, err := util.GetPoolIDs(ctx, monitors, journalPool, pool, mgr.creds)
	if err != nil {
		return "", fmt.Errorf("failed to get poolID for %q: %w", groupUUID, err)
	}

	groupHandle, err := util.GenerateVolID(ctx, monitors, mgr.creds, poolID, pool, clusterID, groupUUID)
	if err != nil {
		return "", fmt.Errorf("failed to generate a unique CSI volume group with uuid for %q: %w", groupUUID, err)
	}

	log.DebugLog(ctx, "re-generated Group ID (%s) and Group Name (%s) for request name (%s)",
		groupHandle, vgName, requestName)

	return groupHandle, nil
}
