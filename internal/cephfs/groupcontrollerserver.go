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

package cephfs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/ceph/ceph-csi/internal/cephfs/core"
	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/ceph/go-ceph/cephfs/admin"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// validateCreateVolumeGroupSnapshotRequest validates the request for creating
// a group snapshot of volumes.
func (cs *ControllerServer) validateCreateVolumeGroupSnapshotRequest(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest,
) error {
	if err := cs.Driver.ValidateGroupControllerServiceRequest(
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT); err != nil {
		log.ErrorLog(ctx, "invalid create volume group snapshot req: %v", protosanitizer.StripSecrets(req))

		return err
	}

	// Check sanity of request volume group snapshot Name, Source Volume Id's
	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "volume group snapshot Name cannot be empty")
	}

	if len(req.GetSourceVolumeIds()) == 0 {
		return status.Error(codes.InvalidArgument, "source volume ids cannot be empty")
	}

	param := req.GetParameters()
	// check for ClusterID and fsName
	if value, ok := param["clusterID"]; !ok || value == "" {
		return status.Error(codes.InvalidArgument, "missing or empty clusterID")
	}

	if value, ok := param["fsName"]; !ok || value == "" {
		return status.Error(codes.InvalidArgument, "missing or empty fsName")
	}

	return nil
}

// CreateVolumeGroupSnapshot creates a group snapshot of volumes.
func (cs *ControllerServer) CreateVolumeGroupSnapshot(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest) (
	*csi.CreateVolumeGroupSnapshotResponse,
	error,
) {
	if err := cs.validateCreateVolumeGroupSnapshotRequest(ctx, req); err != nil {
		return nil, err
	}

	requestName := req.GetName()
	// Existence and conflict checks
	if acquired := cs.VolumeGroupLocks.TryAcquire(requestName); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, requestName)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, requestName)
	}
	defer cs.VolumeGroupLocks.Release(requestName)

	cr, err := util.NewAdminCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	vg, err := store.NewVolumeGroupOptions(ctx, req, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to get volume group options: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}
	defer vg.Destroy()

	vgs, err := store.CheckVolumeGroupSnapExists(ctx, vg, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to check volume group snapshot exists: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get the fs names and subvolume from the volume ids to execute quiesce commands.
	fsMap, err := getFsNamesAndSubVolumeFromVolumeIDs(ctx, req.GetSecrets(), req.GetSourceVolumeIds(), cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to get fs names and subvolume from volume ids: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}
	defer destroyFSConnections(fsMap)

	needRelease := checkIfFSNeedQuiesceRelease(vgs, req.GetSourceVolumeIds())
	if needRelease {
		return cs.releaseQuiesceAndGetVolumeGroupSnapshotResponse(ctx, req, vgs, fsMap, vg, cr)
	}

	// If the volume group snapshot does not exist, reserve the volume group
	if vgs == nil {
		vgs, err = store.ReserveVolumeGroup(ctx, vg, cr)
		if err != nil {
			log.ErrorLog(ctx, "failed to reserve volume group: %v", err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	inProgress, err := cs.queisceFileSystems(ctx, vgs, fsMap)
	if err != nil {
		log.ErrorLog(ctx, "failed to quiesce filesystems: %v", err)
		if !errors.Is(err, cerrors.ErrQuiesceInProgress) {
			uErr := cs.deleteSnapshotsAndUndoReservation(ctx, vgs, cr, fsMap, req.GetSecrets())
			if uErr != nil {
				log.ErrorLog(ctx, "failed to delete snapshot and undo reservation: %v", uErr)
			}
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	if inProgress {
		return nil, status.Error(codes.Internal, "Quiesce operation is in progress")
	}

	resp, err := cs.createSnapshotAddToVolumeGroupJournal(ctx, req, vg, vgs, cr, fsMap)
	if err != nil {
		log.ErrorLog(ctx, "failed to create snapshot and add to volume group journal: %v", err)

		if !errors.Is(err, cerrors.ErrQuiesceInProgress) {
			// Handle Undo reservation and timeout as well
			uErr := cs.deleteSnapshotsAndUndoReservation(ctx, vgs, cr, fsMap, req.GetSecrets())
			if uErr != nil {
				log.ErrorLog(ctx, "failed to delete snapshot and undo reservation: %v", uErr)
			}
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	response := &csi.CreateVolumeGroupSnapshotResponse{}
	response.GroupSnapshot = &csi.VolumeGroupSnapshot{
		GroupSnapshotId: vgs.VolumeGroupSnapshotID,
		ReadyToUse:      true,
		CreationTime:    timestamppb.New(time.Now()),
	}

	for _, r := range resp {
		r.Snapshot.GroupSnapshotId = vgs.VolumeGroupSnapshotID
		response.GroupSnapshot.Snapshots = append(response.GroupSnapshot.Snapshots, r.GetSnapshot())
	}

	return response, nil
}

// queisceFileSystems quiesces the subvolumes and subvolume groups present in
// the filesystems of the volumeID's present in the
// CreateVolumeGroupSnapshotRequest.
func (cs *ControllerServer) queisceFileSystems(ctx context.Context,
	vgs *store.VolumeGroupSnapshotIdentifier,
	fsMap map[string]core.FSQuiesceClient,
) (bool, error) {
	var inProgress bool
	for _, fm := range fsMap {
		// Quiesce the fs, subvolumes and subvolume groups
		data, err := fm.FSQuiesce(ctx, vgs.RequestName)
		if err != nil {
			log.ErrorLog(ctx, "failed to quiesce filesystem: %v", err)

			return inProgress, err
		}
		state := core.GetQuiesceState(data.State)
		if state == core.Quiescing {
			inProgress = true
		} else if state != core.Quiesced {
			return inProgress, fmt.Errorf("quiesce operation is in %s state", state)
		}
	}

	return inProgress, nil
}

// releaseQuiesceAndGetVolumeGroupSnapshotResponse releases the quiesce of the
// subvolumes and subvolume groups in the filesystems for the volumeID's
// present in the CreateVolumeGroupSnapshotRequest.
func (cs *ControllerServer) releaseQuiesceAndGetVolumeGroupSnapshotResponse(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest,
	vgs *store.VolumeGroupSnapshotIdentifier,
	fsMap map[string]core.FSQuiesceClient,
	vg *store.VolumeGroupOptions,
	cr *util.Credentials,
) (*csi.CreateVolumeGroupSnapshotResponse, error) {
	matchesSourceVolumeIDs := matchesSourceVolumeIDs(vgs.GetVolumeIDs(), req.GetSourceVolumeIds())
	if !matchesSourceVolumeIDs {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"source volume ids %v do not match in the existing volume group snapshot %v",
			req.GetSourceVolumeIds(),
			vgs.GetVolumeIDs())
	}
	// Release the quiesce of the subvolumes and subvolume groups in the
	// filesystems for the volumes.
	for _, fm := range fsMap {
		// UnFreeze the filesystems, subvolumes and subvolume groups
		data, err := fm.ReleaseFSQuiesce(ctx, vg.RequestName)
		if err != nil {
			log.ErrorLog(ctx, "failed to release filesystem quiesce: %v", err)
			uErr := cs.deleteSnapshotsAndUndoReservation(ctx, vgs, cr, fsMap, req.GetSecrets())
			if uErr != nil {
				log.ErrorLog(ctx, "failed to delete snapshot and undo reservation: %v", uErr)
			}

			return nil, status.Errorf(codes.Internal, "failed to release filesystem quiesce: %v", err)
		}
		state := core.GetQuiesceState(data.State)
		if state != core.Released {
			return nil, status.Errorf(codes.Internal, "quiesce operation is in %s state", state)
		}
	}
	var err error
	defer func() {
		if err != nil && !errors.Is(err, cerrors.ErrQuiesceInProgress) {
			uErr := cs.deleteSnapshotsAndUndoReservation(ctx, vgs, cr, fsMap, req.GetSecrets())
			if uErr != nil {
				log.ErrorLog(ctx, "failed to delete snapshot and undo reservation: %v", uErr)
			}
		}
	}()
	snapshotResponses := make([]*csi.CreateSnapshotResponse, 0)
	for _, volID := range req.GetSourceVolumeIds() {
		// Create the snapshot for the volumeID
		clusterID := getClusterIDForVolumeID(fsMap, volID)
		if clusterID == "" {
			return nil, status.Errorf(codes.Internal, "failed to get clusterID for volumeID %s", volID)
		}

		req := formatCreateSnapshotRequest(volID, vgs.FsVolumeGroupSnapshotName,
			clusterID,
			req.GetSecrets())
		var resp *csi.CreateSnapshotResponse
		resp, err = cs.createSnapshotAndAddMapping(ctx, req, vg, vgs, cr)
		if err != nil {
			// Handle cleanup
			log.ErrorLog(ctx, "failed to create snapshot: %v", err)

			return nil, status.Errorf(codes.Internal,
				"failed to create snapshot and add to volume group journal: %v",
				err)
		}
		snapshotResponses = append(snapshotResponses, resp)
	}

	response := &csi.CreateVolumeGroupSnapshotResponse{}
	response.GroupSnapshot = &csi.VolumeGroupSnapshot{
		GroupSnapshotId: vgs.VolumeGroupSnapshotID,
		ReadyToUse:      true,
		CreationTime:    timestamppb.New(time.Now()),
	}

	for _, r := range snapshotResponses {
		r.Snapshot.GroupSnapshotId = vgs.VolumeGroupSnapshotID
		response.GroupSnapshot.Snapshots = append(response.GroupSnapshot.Snapshots, r.GetSnapshot())
	}

	return response, nil
}

// createSnapshotAddToVolumeGroupJournal creates the snapshot and adds the
// snapshotID and volumeID to the volume group journal omap. If the freeze is
// true then it will freeze the subvolumes and subvolume groups before creating
// the snapshot and unfreeze them after creating the snapshot. If the freeze is
// false it will call createSnapshot and get the snapshot details for the
// volume and add the snapshotID and volumeID to the volume group journal omap.
// If any error occurs other than ErrInProgress it will delete the snapshots
// and undo the reservation and return the error.
func (cs *ControllerServer) createSnapshotAddToVolumeGroupJournal(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest,
	vgo *store.VolumeGroupOptions,
	vgs *store.VolumeGroupSnapshotIdentifier,
	cr *util.Credentials,
	fsMap map[string]core.FSQuiesceClient) (
	[]*csi.CreateSnapshotResponse,
	error,
) {
	var err error
	var resp *csi.CreateSnapshotResponse

	responses := make([]*csi.CreateSnapshotResponse, 0)
	for _, volID := range req.GetSourceVolumeIds() {
		err = fsQuiesceWithExpireTimeout(ctx, vgo.RequestName, fsMap)
		if err != nil {
			log.ErrorLog(ctx, "failed to quiesce filesystem with timeout: %v", err)

			return nil, err
		}

		// Create the snapshot for the volumeID
		clusterID := getClusterIDForVolumeID(fsMap, volID)
		if clusterID == "" {
			return nil, fmt.Errorf("failed to get clusterID for volumeID %s", volID)
		}

		req := formatCreateSnapshotRequest(volID, vgs.FsVolumeGroupSnapshotName,
			clusterID,
			req.GetSecrets())
		resp, err = cs.createSnapshotAndAddMapping(ctx, req, vgo, vgs, cr)
		if err != nil {
			// Handle cleanup
			log.ErrorLog(ctx, "failed to create snapshot: %v", err)

			return nil, err
		}
		responses = append(responses, resp)
	}

	err = releaseFSQuiesce(ctx, vgo.RequestName, fsMap)
	if err != nil {
		log.ErrorLog(ctx, "failed to release filesystem quiesce: %v", err)

		return nil, err
	}

	return responses, nil
}

func formatCreateSnapshotRequest(volID, groupSnapshotName,
	clusterID string,
	secret map[string]string,
) *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: volID,
		Name:           groupSnapshotName + "-" + volID,
		Secrets:        secret,
		Parameters: map[string]string{
			"clusterID": clusterID,
		},
	}
}

// releaseSubvolumeQuiesce releases the quiesce of the subvolumes and subvolume
// groups in the filesystems for the volumeID's present in the
// CreateVolumeGroupSnapshotRequest.
func releaseFSQuiesce(ctx context.Context,
	requestName string,
	fsMap map[string]core.FSQuiesceClient,
) error {
	inProgress := false
	var err error
	var data *admin.QuiesceInfo
	for _, fm := range fsMap {
		// UnFreeze the filesystems, subvolumes and subvolume groups
		data, err = fm.ReleaseFSQuiesce(ctx, requestName)
		if err != nil {
			log.ErrorLog(ctx, "failed to release filesystem quiesce: %v", err)

			return err
		}
		state := core.GetQuiesceState(data.State)
		if state != core.Released {
			inProgress = true
		}
	}

	if inProgress {
		return cerrors.ErrQuiesceInProgress
	}

	return nil
}

// fsQuiesceWithExpireTimeout quiesces the subvolumes and subvolume
// groups in the filesystems for the volumeID's present in the
// CreateVolumeGroupSnapshotRequest.
func fsQuiesceWithExpireTimeout(ctx context.Context,
	requestName string,
	fsMap map[string]core.FSQuiesceClient,
) error {
	var err error

	var data *admin.QuiesceInfo
	inProgress := false
	for _, fm := range fsMap {
		// reinitialize the expiry timer for the quiesce
		data, err = fm.FSQuiesceWithExpireTimeout(ctx, requestName)
		if err != nil {
			log.ErrorLog(ctx, "failed to quiesce filesystem with timeout: %v", err)

			return err
		}
		state := core.GetQuiesceState(data.State)
		if state == core.Quiescing {
			inProgress = true
		} else if state != core.Quiesced {
			return fmt.Errorf("quiesce operation is in %s state", state)
		}
	}

	if inProgress {
		return cerrors.ErrQuiesceInProgress
	}

	return nil
}

// createSnapshotAndAddMapping creates the snapshot and adds the snapshotID and
// volumeID to the volume group journal omap. If any error occurs it will
// delete the last created snapshot as its still not added to the journal.
func (cs *ControllerServer) createSnapshotAndAddMapping(
	ctx context.Context,
	req *csi.CreateSnapshotRequest,
	vgo *store.VolumeGroupOptions,
	vgs *store.VolumeGroupSnapshotIdentifier,
	cr *util.Credentials,
) (*csi.CreateSnapshotResponse, error) {
	// Create the snapshot
	resp, err := cs.CreateSnapshot(ctx, req)
	if err != nil {
		// Handle cleanup
		log.ErrorLog(ctx, "failed to create snapshot: %v", err)

		return nil, err
	}
	j, err := store.VolumeGroupJournal.Connect(vgo.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return nil, err
	}
	defer j.Destroy()
	// Add the snapshot to the volume group journal
	err = j.AddVolumesMapping(ctx,
		vgo.MetadataPool,
		vgs.ReservedID,
		map[string]string{
			req.GetSourceVolumeId(): resp.GetSnapshot().GetSnapshotId(),
		},
	)
	if err != nil {
		log.ErrorLog(ctx, "failed to add volume snapshot mapping: %v", err)
		// Delete the last created snapshot as its still not added to the
		// journal
		delReq := &csi.DeleteSnapshotRequest{
			SnapshotId: resp.GetSnapshot().GetSnapshotId(),
			Secrets:    req.GetSecrets(),
		}
		_, dErr := cs.DeleteSnapshot(ctx, delReq)
		if dErr != nil {
			log.ErrorLog(ctx, "failed to delete snapshot %s: %v", resp.GetSnapshot().GetSnapshotId(), dErr)
		}

		return nil, err
	}

	return resp, nil
}

// checkIfFSNeedQuiesceRelease checks that do we have snapshots for all the
// volumes stored in the omap so that we can release the quiesce.
func checkIfFSNeedQuiesceRelease(vgs *store.VolumeGroupSnapshotIdentifier, volIDs []string) bool {
	if vgs == nil {
		return false
	}
	// If the number of volumes in the snapshot is not equal to the number of volumes

	return len(vgs.GetVolumeIDs()) == len(volIDs)
}

// getClusterIDForVolumeID gets the clusterID for the volumeID from the fms map.
func getClusterIDForVolumeID(fms map[string]core.FSQuiesceClient, volumeID string) string {
	for _, fm := range fms {
		for _, vol := range fm.GetVolumes() {
			if vol.VolumeID == volumeID {
				return vol.ClusterID
			}
		}
	}

	return ""
}

// getFsNamesAndSubVolumeFromVolumeIDs gets the filesystem names and subvolumes
// from the volumeIDs present in the CreateVolumeGroupSnapshotRequest. It also
// returns the SubVolumeQuiesceClient for the filesystems present in the
// volumeIDs.
func getFsNamesAndSubVolumeFromVolumeIDs(ctx context.Context,
	secret map[string]string,
	volIDs []string,
	cr *util.Credentials) (
	map[string]core.FSQuiesceClient,
	error,
) {
	type fs struct {
		fsName                string
		volumes               []core.Volume
		subVolumeGroupMapping map[string][]string
		monitors              string
	}
	fm := make(map[string]fs, 0)
	for _, volID := range volIDs {
		// Find the volume using the provided VolumeID
		volOptions, _, err := store.NewVolumeOptionsFromVolID(ctx,
			volID, nil, secret, "", false)
		if err != nil {
			return nil, err
		}
		volOptions.Destroy()
		// choosing monitorIP's and fsName as the unique key
		// TODO: Need to use something else as the unique key as users can
		// still choose the different monitorIP's and fsName for subvolumes
		uniqueName := volOptions.Monitors + volOptions.FsName
		if _, ok := fm[uniqueName]; !ok {
			fm[uniqueName] = fs{
				fsName:                volOptions.FsName,
				volumes:               make([]core.Volume, 0),
				subVolumeGroupMapping: make(map[string][]string), // Initialize the map
				monitors:              volOptions.Monitors,
			}
		}
		a := core.Volume{
			VolumeID:  volID,
			ClusterID: volOptions.ClusterID,
		}
		// Retrieve the value, modify it, and assign it back
		val := fm[uniqueName]
		val.volumes = append(val.volumes, a)
		existingVolIDInMap := val.subVolumeGroupMapping[volOptions.SubVolume.SubvolumeGroup]
		val.subVolumeGroupMapping[volOptions.SubVolume.SubvolumeGroup] = append(
			existingVolIDInMap,
			volOptions.SubVolume.VolID)
		fm[uniqueName] = val
	}
	fsk := map[string]core.FSQuiesceClient{}
	var err error
	defer func() {
		if err != nil {
			destroyFSConnections(fsk)
		}
	}()
	for k, v := range fm {
		conn := &util.ClusterConnection{}
		if err = conn.Connect(v.monitors, cr); err != nil {
			return nil, err
		}
		fsk[k], err = core.NewFSQuiesce(v.fsName, v.volumes, v.subVolumeGroupMapping, conn)
		if err != nil {
			log.ErrorLog(ctx, "failed to get subvolume quiesce: %v", err)
			conn.Destroy()

			return nil, err
		}
	}

	return fsk, nil
}

// destroyFSConnections destroys connections of all FSQuiesceClient.
func destroyFSConnections(fsMap map[string]core.FSQuiesceClient) {
	for _, fm := range fsMap {
		if fm != nil {
			fm.Destroy()
		}
	}
}

// matchesSourceVolumeIDs checks if the sourceVolumeIDs and volumeIDsInOMap are
// equal.
func matchesSourceVolumeIDs(sourceVolumeIDs, volumeIDsInOMap []string) bool {
	// sort the array as its required for slices.Equal call.
	sort.Strings(sourceVolumeIDs)
	sort.Strings(volumeIDsInOMap)

	return slices.Equal(sourceVolumeIDs, volumeIDsInOMap)
}

// deleteSnapshotsAndUndoReservation deletes the snapshots and undoes the
// volume group reservation. It also resets the quiesce of the subvolumes and
// subvolume groups in the filesystems for the volumeID's present in the
// CreateVolumeGroupSnapshotRequest.
func (cs *ControllerServer) deleteSnapshotsAndUndoReservation(ctx context.Context,
	vgs *store.VolumeGroupSnapshotIdentifier,
	cr *util.Credentials,
	fsMap map[string]core.FSQuiesceClient,
	secrets map[string]string,
) error {
	// get the omap from the snapshot and volume mapping
	vgo, vgsi, err := store.NewVolumeGroupOptionsFromID(ctx, vgs.VolumeGroupSnapshotID, cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to get volume group options from id: %v", err)

		return err
	}
	defer vgo.Destroy()

	for volID, snapID := range vgsi.VolumeSnapshotMap {
		// delete the snapshots
		req := &csi.DeleteSnapshotRequest{
			SnapshotId: snapID,
			Secrets:    secrets,
		}
		_, err = cs.DeleteSnapshot(ctx, req)
		if err != nil {
			log.ErrorLog(ctx, "failed to delete snapshot: %v", err)

			return err
		}

		j, err := store.VolumeGroupJournal.Connect(vgo.Monitors, fsutil.RadosNamespace, cr)
		if err != nil {
			return err
		}
		// remove the entry from the omap
		err = j.RemoveVolumesMapping(
			ctx,
			vgo.MetadataPool,
			vgsi.ReservedID,
			[]string{volID})
		j.Destroy()
		if err != nil {
			log.ErrorLog(ctx, "failed to remove volume snapshot mapping: %v", err)

			return err
		}
		// undo the reservation
		err = store.UndoVolumeGroupReservation(ctx, vgo, vgsi, cr)
		if err != nil {
			log.ErrorLog(ctx, "failed to undo volume group reservation: %v", err)

			return err
		}
	}

	for _, fm := range fsMap {
		_, err := fm.ResetFSQuiesce(ctx, vgs.RequestName)
		if err != nil {
			log.ErrorLog(ctx, "failed to reset filesystem quiesce: %v", err)

			return err
		}
	}

	return nil
}

// validateVolumeGroupSnapshotDeleteRequest validates the request for creating a group
// snapshot of volumes.
func (cs *ControllerServer) validateVolumeGroupSnapshotDeleteRequest(
	ctx context.Context,
	req *csi.DeleteVolumeGroupSnapshotRequest,
) error {
	if err := cs.Driver.ValidateGroupControllerServiceRequest(
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT); err != nil {
		log.ErrorLog(ctx, "invalid create volume group snapshot req: %v", protosanitizer.StripSecrets(req))

		return err
	}

	// Check sanity of request volume group snapshot Name, Source Volume Id's
	if req.GetGroupSnapshotId() == "" {
		return status.Error(codes.InvalidArgument, "volume group snapshot id cannot be empty")
	}

	return nil
}

// DeleteVolumeGroupSnapshot deletes a group snapshot of volumes.
func (cs *ControllerServer) DeleteVolumeGroupSnapshot(ctx context.Context,
	req *csi.DeleteVolumeGroupSnapshotRequest) (
	*csi.DeleteVolumeGroupSnapshotResponse,
	error,
) {
	if err := cs.validateVolumeGroupSnapshotDeleteRequest(ctx, req); err != nil {
		return nil, err
	}

	groupSnapshotID := req.GetGroupSnapshotId()
	// Existence and conflict checks
	if acquired := cs.VolumeGroupLocks.TryAcquire(groupSnapshotID); !acquired {
		log.ErrorLog(ctx, util.SnapshotOperationAlreadyExistsFmt, groupSnapshotID)

		return nil, status.Errorf(codes.Aborted, util.SnapshotOperationAlreadyExistsFmt, groupSnapshotID)
	}
	defer cs.VolumeGroupLocks.Release(groupSnapshotID)

	cr, err := util.NewAdminCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	vgo, vgsi, err := store.NewVolumeGroupOptionsFromID(ctx, req.GetGroupSnapshotId(), cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to get volume group options: %v", err)
		err = extractDeleteVolumeGroupError(err)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
	}
	vgo.Destroy()

	volIds := vgsi.GetVolumeIDs()
	fsMap, err := getFsNamesAndSubVolumeFromVolumeIDs(ctx, req.GetSecrets(), volIds, cr)
	err = extractDeleteVolumeGroupError(err)
	if err != nil {
		log.ErrorLog(ctx, "failed to get volume group options: %v", err)
		err = extractDeleteVolumeGroupError(err)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
	}

	defer destroyFSConnections(fsMap)

	err = cs.deleteSnapshotsAndUndoReservation(ctx, vgsi, cr, fsMap, req.GetSecrets())
	if err != nil {
		log.ErrorLog(ctx, "failed to delete snapshot and undo reservation: %v", err)
		err = extractDeleteVolumeGroupError(err)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
	}

	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

// extractDeleteVolumeGroupError extracts the error from the delete volume
// group snapshot and returns the error if it is not a ErrKeyNotFound or
// ErrPoolNotFound error.
func extractDeleteVolumeGroupError(err error) error {
	switch {
	case errors.Is(err, util.ErrPoolNotFound):
		// if error is ErrPoolNotFound, the pool is already deleted we dont
		// need to worry about deleting snapshot or omap data, return success
		return nil
	case errors.Is(err, util.ErrKeyNotFound):
		// if error is ErrKeyNotFound, then a previous attempt at deletion was complete
		// or partially complete (snap and snapOMap are garbage collected already), hence return
		// success as deletion is complete
		return nil
	}

	return err
}
