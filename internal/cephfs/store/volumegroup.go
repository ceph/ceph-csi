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

package store

import (
	"context"
	"fmt"

	"github.com/ceph/ceph-csi/internal/cephfs/core"
	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

type VolumeGroupOptions struct {
	*VolumeOptions
}

// NewVolumeGroupOptions generates a new instance of volumeGroupOptions from the provided
// CSI request parameters.
func NewVolumeGroupOptions(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest,
	cr *util.Credentials,
) (*VolumeGroupOptions, error) {
	var (
		opts = &VolumeGroupOptions{}
		err  error
	)

	volOptions := req.GetParameters()
	opts.VolumeOptions, err = getVolumeOptions(volOptions)
	if err != nil {
		return nil, err
	}

	if err = extractOptionalOption(&opts.NamePrefix, "volumeGroupNamePrefix", volOptions); err != nil {
		return nil, err
	}

	opts.RequestName = req.GetName()

	err = opts.Connect(cr)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			opts.Destroy()
		}
	}()

	fs := core.NewFileSystem(opts.conn)
	opts.FscID, err = fs.GetFscID(ctx, opts.FsName)
	if err != nil {
		return nil, err
	}

	opts.MetadataPool, err = fs.GetMetadataPool(ctx, opts.FsName)
	if err != nil {
		return nil, err
	}

	return opts, nil
}

type VolumeGroupSnapshotIdentifier struct {
	ReservedID                string
	FsVolumeGroupSnapshotName string
	VolumeGroupSnapshotID     string
	RequestName               string
	VolumeSnapshotMap         map[string]string
}

// GetVolumeIDs returns the list of volumeIDs in the VolumeSnaphotMap.
func (vgsi *VolumeGroupSnapshotIdentifier) GetVolumeIDs() []string {
	keys := make([]string, 0, len(vgsi.VolumeSnapshotMap))
	for k := range vgsi.VolumeSnapshotMap {
		keys = append(keys, k)
	}

	return keys
}

// NewVolumeGroupOptionsFromID generates a new instance of volumeGroupOptions and GroupIdentifier
// from the provided CSI volumeGroupSnapshotID.
func NewVolumeGroupOptionsFromID(
	ctx context.Context,
	volumeGroupSnapshotID string,
	cr *util.Credentials,
) (*VolumeGroupOptions, *VolumeGroupSnapshotIdentifier, error) {
	var (
		vi         util.CSIIdentifier
		volOptions = &VolumeGroupOptions{}
		vgs        VolumeGroupSnapshotIdentifier
	)
	// Decode the snapID first, to detect pre-provisioned snapshot before other errors
	err := vi.DecomposeCSIID(volumeGroupSnapshotID)
	if err != nil {
		return nil, nil, cerrors.ErrInvalidVolID
	}
	volOptions.VolumeOptions = &VolumeOptions{}
	volOptions.ClusterID = vi.ClusterID
	vgs.VolumeGroupSnapshotID = volumeGroupSnapshotID
	volOptions.FscID = vi.LocationID
	vgs.ReservedID = vi.ObjectUUID

	if volOptions.Monitors, err = util.Mons(util.CsiConfigFile, vi.ClusterID); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to fetch monitor list using clusterID (%s): %w",
			vi.ClusterID,
			err)
	}

	if volOptions.RadosNamespace, err = util.GetCephFSRadosNamespace(util.CsiConfigFile, vi.ClusterID); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to fetch rados namespace using clusterID (%s): %w",
			vi.ClusterID,
			err)
	}

	err = volOptions.Connect(cr)
	if err != nil {
		return nil, nil, err
	}
	// in case of an error, volOptions is returned, but callers may not
	// expect to need to call Destroy() on it. So, make sure to release any
	// resources that may have been allocated
	defer func() {
		if err != nil {
			volOptions.Destroy()
		}
	}()

	fs := core.NewFileSystem(volOptions.conn)
	volOptions.FsName, err = fs.GetFsName(ctx, volOptions.FscID)
	if err != nil {
		return nil, nil, err
	}

	volOptions.MetadataPool, err = fs.GetMetadataPool(ctx, volOptions.FsName)
	if err != nil {
		return nil, nil, err
	}

	j, err := VolumeGroupJournal.Connect(volOptions.Monitors, volOptions.RadosNamespace, cr)
	if err != nil {
		return nil, nil, err
	}
	defer j.Destroy()

	groupAttributes, err := j.GetVolumeGroupAttributes(
		ctx, volOptions.MetadataPool, vi.ObjectUUID)
	if err != nil {
		return nil, nil, err
	}

	vgs.RequestName = groupAttributes.RequestName
	vgs.FsVolumeGroupSnapshotName = groupAttributes.GroupName
	vgs.VolumeGroupSnapshotID = volumeGroupSnapshotID
	vgs.VolumeSnapshotMap = groupAttributes.VolumeMap

	return volOptions, &vgs, nil
}

/*
CheckVolumeGroupSnapExists checks to determine if passed in RequestName in
volGroupOptions exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information
regarding volume group snapshot names as requested by the CSI drivers. Hence,
these need to be invoked only when the respective CSI driver generated volume
group snapshot name based locks are held, as otherwise racy access to these
omaps may end up leaving them in an inconsistent state.
*/
func CheckVolumeGroupSnapExists(
	ctx context.Context,
	volOptions *VolumeGroupOptions,
	cr *util.Credentials,
) (*VolumeGroupSnapshotIdentifier, error) {
	j, err := VolumeGroupJournal.Connect(volOptions.Monitors, volOptions.RadosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	volGroupData, err := j.CheckReservation(
		ctx, volOptions.MetadataPool, volOptions.RequestName, volOptions.NamePrefix)
	if err != nil {
		return nil, err
	}
	if volGroupData == nil {
		return nil, nil
	}
	vgs := &VolumeGroupSnapshotIdentifier{}
	vgs.RequestName = volOptions.RequestName
	vgs.ReservedID = volGroupData.GroupUUID
	vgs.FsVolumeGroupSnapshotName = volGroupData.GroupName
	vgs.VolumeSnapshotMap = volGroupData.VolumeGroupAttributes.VolumeMap

	// found a snapshot already available, process and return it!
	vgs.VolumeGroupSnapshotID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, volGroupData.GroupUUID)
	if err != nil {
		return nil, err
	}
	log.DebugLog(ctx, "Found existing volume group snapshot (%s) with UUID (%s) for request (%s) and mapping %v",
		vgs.RequestName, volGroupData.GroupUUID, vgs.RequestName, vgs.VolumeSnapshotMap)

	return vgs, nil
}

// ReserveVolumeGroup is a helper routine to request a UUID reservation for the
// CSI request name and,
// to generate the volumegroup snapshot identifier for the reserved UUID.
func ReserveVolumeGroup(
	ctx context.Context,
	volOptions *VolumeGroupOptions,
	cr *util.Credentials,
) (*VolumeGroupSnapshotIdentifier, error) {
	var (
		vgsi      VolumeGroupSnapshotIdentifier
		groupUUID string
		err       error
	)

	vgsi.RequestName = volOptions.RequestName
	j, err := VolumeGroupJournal.Connect(volOptions.Monitors, volOptions.RadosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()

	groupUUID, vgsi.FsVolumeGroupSnapshotName, err = j.ReserveName(
		ctx, volOptions.MetadataPool, volOptions.RequestName, volOptions.NamePrefix)
	if err != nil {
		return nil, err
	}

	// generate the snapshot ID to return to the CO system
	vgsi.VolumeGroupSnapshotID, err = util.GenerateVolID(ctx, volOptions.Monitors, cr, volOptions.FscID,
		"", volOptions.ClusterID, groupUUID)
	if err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "Generated volume group snapshot ID (%s) for request name (%s)",
		vgsi.VolumeGroupSnapshotID, volOptions.RequestName)

	return &vgsi, nil
}

// UndoVolumeGroupReservation is a helper routine to undo a name reservation
// for a CSI volumeGroupSnapshot name.
func UndoVolumeGroupReservation(
	ctx context.Context,
	volOptions *VolumeGroupOptions,
	vgsi *VolumeGroupSnapshotIdentifier,
	cr *util.Credentials,
) error {
	j, err := VolumeGroupJournal.Connect(volOptions.Monitors, volOptions.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(ctx, volOptions.MetadataPool,
		vgsi.FsVolumeGroupSnapshotName, vgsi.RequestName)

	return err
}
