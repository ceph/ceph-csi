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
	"fmt"
	"strings"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/csi-addons/spec/lib/go/volumegroup"

	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

var ErrRBDGroupNotConnected = errors.New("RBD group is not connected")

// volumeGroup handles all requests for 'rbd group' operations.
type volumeGroup struct {
	// id is a unique value for this volume group in the Ceph cluster, it
	// is used to find the group in the journal.
	id string

	// name is used in RBD API calls as the name of this object
	name string

	clusterID string

	credentials *util.Credentials

	// temporary connection attributes
	conn  *util.ClusterConnection
	ioctx *rados.IOContext

	// required details to perform operations on the group
	monitors  string
	pool      string
	namespace string

	journal journal.VolumeGroupJournal

	// volumes is a list of rbd-images that are part of the group. The ID
	// of each volume is stored in the journal.
	volumes []types.Volume

	// volumeToFree contains Volumes that were resolved during
	// GetVolumeGroup. The volumes slice can be updated independently of
	// this by calling AddVolume (Volumes are allocated elsewhere), and
	// RemoveVolume (need to keep track of the allocated Volume).
	volumesToFree []types.Volume
}

// verify that volumeGroup implements the VolumeGroup and Stringer interfaces.
var (
	_ types.VolumeGroup = &volumeGroup{}
	_ fmt.Stringer      = &volumeGroup{}
)

// GetVolumeGroup initializes a new VolumeGroup object that can be used
// to manage an `rbd group`.
// If the .GetName() function returns an error, the VolumeGroup does not exist
// yet. It is needed to call .Create() in that case first.
func GetVolumeGroup(
	ctx context.Context,
	id string,
	j journal.VolumeGroupJournal,
	creds *util.Credentials,
	volumeResolver types.VolumeResolver,
) (types.VolumeGroup, error) {
	csiID := util.CSIIdentifier{}
	err := csiID.DecomposeCSIID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose volume group id %q: %w", id, err)
	}

	mons, err := util.Mons(util.CsiConfigFile, csiID.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MONs for cluster id %q: %w", csiID.ClusterID, err)
	}

	namespace, err := util.GetRadosNamespace(util.CsiConfigFile, csiID.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get RADOS namespace for cluster id %q: %w", csiID.ClusterID, err)
	}

	pool, err := util.GetPoolName(mons, creds, csiID.LocationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool for volume group id %q: %w", id, err)
	}

	attrs, err := j.GetVolumeGroupAttributes(ctx, pool, csiID.ObjectUUID)
	if err != nil {
		if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) {
			return nil, fmt.Errorf("failed to get attributes for volume group id %q: %w", id, err)
		}

		attrs = &journal.VolumeGroupAttributes{}
	}

	var volumes []types.Volume
	for volID := range attrs.VolumeMap {
		vol, err := volumeResolver.GetVolumeByID(ctx, volID)
		if err != nil {
			// free the previously allocated volumes
			for _, v := range volumes {
				v.Destroy(ctx)
			}

			return nil, fmt.Errorf("failed to get attributes for volume group id %q: %w", id, err)
		}

		volumes = append(volumes, vol)
	}

	vg := &volumeGroup{
		journal:     j,
		credentials: creds,
		id:          id,
		name:        attrs.GroupName,
		clusterID:   csiID.ClusterID,
		monitors:    mons,
		pool:        pool,
		namespace:   namespace,
		volumes:     volumes,
		// all allocated volumes need to be free'd at Destroy() time
		volumesToFree: volumes,
	}

	log.DebugLog(ctx, "GetVolumeGroup(%s) returns %+v", id, *vg)

	return vg, nil
}

// String returns the image-spec (pool/{namespace}/{name}) format of the group.
func (vg *volumeGroup) String() string {
	if vg.namespace != "" && vg.pool != "" && vg.name != "" {
		return fmt.Sprintf("%s/%s/%s", vg.pool, vg.namespace, vg.name)
	}

	if vg.name != "" && vg.pool != "" {
		return fmt.Sprintf("%s/%s", vg.pool, vg.name)
	}

	return fmt.Sprintf("<unidentified group %v>", *vg)
}

// GetID returns the CSI-Addons VolumeGroupId of the VolumeGroup.
func (vg *volumeGroup) GetID(ctx context.Context) (string, error) {
	if vg.id == "" {
		return "", errors.New("BUG: ID is not set")
	}

	return vg.id, nil
}

// GetName returns the name in the backend storage for the VolumeGroup.
func (vg *volumeGroup) GetName(ctx context.Context) (string, error) {
	if vg.name == "" {
		return "", errors.New("BUG: name is not set")
	}

	return vg.name, nil
}

// GetPool returns the name of the pool that holds the VolumeGroup.
func (vg *volumeGroup) GetPool(ctx context.Context) (string, error) {
	if vg.pool == "" {
		return "", errors.New("BUG: pool is not set")
	}

	return vg.pool, nil
}

// GetClusterID returns the name of the pool that holds the VolumeGroup.
func (vg *volumeGroup) GetClusterID(ctx context.Context) (string, error) {
	if vg.clusterID == "" {
		return "", errors.New("BUG: clusterID is not set")
	}

	return vg.clusterID, nil
}

// ToCSI creates a CSI-Addons type for the VolumeGroup.
func (vg *volumeGroup) ToCSI(ctx context.Context) (*volumegroup.VolumeGroup, error) {
	volumes, err := vg.ListVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes for volume group %q: %w", vg, err)
	}

	csiVolumes := make([]*csi.Volume, len(volumes))
	for i, vol := range volumes {
		csiVolumes[i], err = vol.ToCSI(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to convert volume %q to CSI type: %w", vol, err)
		}
	}

	id, err := vg.GetID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get id for volume group %q: %w", vg, err)
	}

	// TODO: maybe store the VolumeContext in the journal?
	vgContext := map[string]string{}

	return &volumegroup.VolumeGroup{
		VolumeGroupId:      id,
		VolumeGroupContext: vgContext,
		Volumes:            csiVolumes,
	}, nil
}

// getConnection returns the ClusterConnection for the volume group if it
// exists, otherwise it will open a new one.
// Destroy should be used to close the ClusterConnection.
func (vg *volumeGroup) getConnection(ctx context.Context) (*util.ClusterConnection, error) {
	if vg.conn != nil {
		return vg.conn, nil
	}

	conn := &util.ClusterConnection{}
	err := conn.Connect(vg.monitors, vg.credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MONs %q: %w", vg.monitors, err)
	}

	vg.conn = conn
	log.DebugLog(ctx, "connection established for volume group %q", vg.id)

	return conn, nil
}

// GetIOContext returns the IOContext for the volume group if it exists,
// otherwise it will allocate a new one.
// Destroy should be used to free the IOContext.
func (vg *volumeGroup) GetIOContext(ctx context.Context) (*rados.IOContext, error) {
	if vg.ioctx != nil {
		return vg.ioctx, nil
	}

	conn, err := vg.getConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to connect: %w", ErrRBDGroupNotConnected, err)
	}

	ioctx, err := conn.GetIoctx(vg.pool)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get IOContext: %w", ErrRBDGroupNotConnected, err)
	}

	if vg.namespace != "" {
		ioctx.SetNamespace(vg.namespace)
	}

	vg.ioctx = ioctx
	log.DebugLog(ctx, "iocontext created for volume group %q in pool %q", vg.id, vg.pool)

	return ioctx, nil
}

// Destroy frees the resources used by the volumeGroup.
func (vg *volumeGroup) Destroy(ctx context.Context) {
	// free the volumes that were allocated in GetVolumeGroup()
	if len(vg.volumesToFree) > 0 {
		for _, volume := range vg.volumesToFree {
			volume.Destroy(ctx)
		}
		vg.volumesToFree = make([]types.Volume, 0)
	}

	if vg.ioctx != nil {
		vg.ioctx.Destroy()
		vg.ioctx = nil
	}

	if vg.conn != nil {
		vg.conn.Destroy()
		vg.conn = nil
	}

	if vg.credentials != nil {
		vg.credentials.DeleteCredentials()
		vg.credentials = nil
	}

	log.DebugLog(ctx, "destroyed volume group instance with id %q", vg.id)
}

func (vg *volumeGroup) Create(ctx context.Context) error {
	name, err := vg.GetName(ctx)
	if err != nil {
		return fmt.Errorf("missing name to create volume group: %w", err)
	}

	ioctx, err := vg.GetIOContext(ctx)
	if err != nil {
		return err
	}

	err = librbd.GroupCreate(ioctx, name)
	if err != nil {
		if !errors.Is(rados.ErrObjectExists, err) && !strings.Contains(err.Error(), "rbd: ret=-17, File exists") {
			return fmt.Errorf("failed to create volume group %q: %w", name, err)
		}

		log.DebugLog(ctx, "ignoring error while creating volume group %q: %v", vg, err)
	}

	log.DebugLog(ctx, "volume group %q has been created", vg)

	return nil
}

func (vg *volumeGroup) Delete(ctx context.Context) error {
	name, err := vg.GetName(ctx)
	if err != nil {
		return err
	}

	ioctx, err := vg.GetIOContext(ctx)
	if err != nil {
		return err
	}

	err = librbd.GroupRemove(ioctx, name)
	if err != nil && !errors.Is(rados.ErrNotFound, err) {
		return fmt.Errorf("failed to remove volume group %q: %w", vg, err)
	}

	log.DebugLog(ctx, "volume group %q has been removed", vg)

	return nil
}

func (vg *volumeGroup) AddVolume(ctx context.Context, vol types.Volume) error {
	err := vol.AddToGroup(ctx, vg)
	if err != nil {
		return fmt.Errorf("failed to add volume %q to volume group %q: %w", vol, vg, err)
	}

	vg.volumes = append(vg.volumes, vol)

	volID, err := vol.GetID(ctx)
	if err != nil {
		return err
	}

	pool, err := vg.GetPool(ctx)
	if err != nil {
		return err
	}

	id, err := vg.GetID(ctx)
	if err != nil {
		return err
	}

	csiID := util.CSIIdentifier{}
	err = csiID.DecomposeCSIID(id)
	if err != nil {
		return fmt.Errorf("failed to decompose volume group id %q: %w", id, err)
	}

	toAdd := map[string]string{
		volID: "",
	}

	err = vg.journal.AddVolumesMapping(ctx, pool, csiID.ObjectUUID, toAdd)
	if err != nil {
		return fmt.Errorf("failed to add mapping for volume %q to volume group id %q: %w", volID, id, err)
	}

	return nil
}

func (vg *volumeGroup) RemoveVolume(ctx context.Context, vol types.Volume) error {
	// volume was already removed from the group
	if len(vg.volumes) == 0 {
		return nil
	}

	err := vol.RemoveFromGroup(ctx, vg)
	if err != nil {
		if errors.Is(librbd.ErrNotExist, err) {
			return nil
		}

		return fmt.Errorf("failed to remove volume %q from volume group %q: %w", vol, vg, err)
	}

	// toRemove contain the ID of the volume that is removed from the group
	toRemove, err := vol.GetID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get volume id for %q: %w", vol, err)
	}

	// volumes is the updated list, without the volume that was removed
	volumes := make([]types.Volume, 0)
	var id string
	for _, v := range vg.volumes {
		id, err = v.GetID(ctx)
		if err != nil {
			return err
		}

		if id == toRemove {
			// do not add the volume to the list
			continue
		}

		volumes = append(volumes, v)
	}

	// update the list of volumes
	vg.volumes = volumes

	pool, err := vg.GetPool(ctx)
	if err != nil {
		return err
	}

	id, err = vg.GetID(ctx)
	if err != nil {
		return err
	}

	csiID := util.CSIIdentifier{}
	err = csiID.DecomposeCSIID(id)
	if err != nil {
		return fmt.Errorf("failed to decompose volume group id %q: %w", id, err)
	}

	mapping := []string{
		toRemove,
	}

	err = vg.journal.RemoveVolumesMapping(ctx, pool, csiID.ObjectUUID, mapping)
	if err != nil {
		return fmt.Errorf("failed to remove mapping for volume %q to volume group id %q: %w", toRemove, id, err)
	}

	return nil
}

func (vg *volumeGroup) ListVolumes(ctx context.Context) ([]types.Volume, error) {
	return vg.volumes, nil
}
