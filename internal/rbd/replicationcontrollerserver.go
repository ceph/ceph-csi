/*
Copyright 2021 The Ceph-CSI Authors.

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
	"strconv"

	"github.com/ceph/ceph-csi/internal/util"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/kube-storage/spec/lib/go/replication"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//  imageMirroringMode is used to indicate the mirroring mode for an RBD image.
type imageMirroringMode string

const (
	// imageMirrorModeSnapshot uses snapshots to propagate RBD images between
	// ceph clusters.
	imageMirrorModeSnapshot imageMirroringMode = "snapshot"
)

// imageMirroringState represents the image mirroring state.
type imageMirroringState string

const (
	// If the state is up+replaying, then mirroring is functioning properly.
	// up means the rbd-mirror daemon is running, and replaying means
	// this image is the target for replication from another storage cluster.
	upAndReplaying imageMirroringState = "up+replaying"
	// If the state is up+stopped means the rbd-mirror daemon is
	// running and stopped means the image is not a target for replication from
	// another cluster
	upAndStopped imageMirroringState = "up+stopped"
)

const (
	// mirroringMode + key to get the imageMirroringMode from parameters.
	imageMirroringKey = "mirroringMode"
	// forceKey + key to get the force option from parameters.
	forceKey = "force"
)

// getVolumeFromID gets the rbd image details from the volumeID.
// TODO: move this to controllerserver.go and reuse it wherever its applicable.
func (cs *ControllerServer) getVolumeFromID(ctx context.Context, volumeID string, secrets map[string]string) (*rbdVolume, *util.Credentials, error) {
	// validate the volume ID
	cr, err := util.NewUserCredentials(secrets)
	if err != nil {
		return nil, nil, status.Error(codes.Internal, err.Error())
	}

	if volumeID == "" {
		return nil, cr, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	if acquired := cs.VolumeLocks.TryAcquire(volumeID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)
		return nil, cr, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}

	var rbdVol = &rbdVolume{}
	rbdVol, err = genVolFromVolID(ctx, volumeID, cr, secrets)
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume %s not found", volumeID)
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Errorf(codes.NotFound, "pool %s not found for %s", rbdVol.Pool, volumeID)
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}
	}
	return rbdVol, cr, err
}

// getForceOption extracts the force option from the GRPC request parameters.
// If not set, the default will be set to false.
func getForceOption(ctx context.Context, parameters map[string]string) (bool, error) {
	val, ok := parameters[forceKey]
	if !ok {
		util.WarningLog(ctx, "%s is not set in parameters, setting to default (%v)", forceKey, false)
		return false, nil
	}
	force, err := strconv.ParseBool(val)
	if err != nil {
		return false, status.Errorf(codes.Internal, err.Error())
	}
	return force, nil
}

// getMirroringMode gets the mirroring mode from the input GRPC request parameters.
// mirroringMode is the key to check the mode in the parameters.
func getMirroringMode(ctx context.Context, parameters map[string]string) (librbd.ImageMirrorMode, error) {
	val, ok := parameters[imageMirroringKey]
	if !ok {
		util.WarningLog(ctx, "%s is not set in parameters, setting to mirroringMode to default (%s)", imageMirroringKey, imageMirrorModeSnapshot)
		return librbd.ImageMirrorModeSnapshot, nil
	}

	var mirroringMode librbd.ImageMirrorMode
	switch imageMirroringMode(val) {
	case imageMirrorModeSnapshot:
		mirroringMode = librbd.ImageMirrorModeSnapshot
	default:
		return mirroringMode, status.Errorf(codes.InvalidArgument, "%s %s not supported", imageMirroringKey, val)
	}
	return mirroringMode, nil
}

// cleanup performs below resource cleanup operations.
func (cs *ControllerServer) cleanup(rbdVol *rbdVolume, cr *util.Credentials) {
	if cr != nil {
		// destroy the credential file
		cr.DeleteCredentials()
	}
	if rbdVol != nil {
		// release the volume lock
		cs.VolumeLocks.Release(rbdVol.VolID)
		// destroy the cluster connection
		rbdVol.Destroy()
	}
}

// EnableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present it will enable the mirroring based on the
// user provided information.
// TODO: create new Replication controller struct for the replication operations.
func (cs *ControllerServer) EnableVolumeReplication(ctx context.Context,
	req *replication.EnableVolumeReplicationRequest,
) (*replication.EnableVolumeReplicationResponse, error) {
	rbdVol, cr, err := cs.getVolumeFromID(ctx, req.GetVolumeId(), req.GetSecrets())
	defer cs.cleanup(rbdVol, cr)
	if err != nil {
		return nil, err
	}
	// extract the mirroring mode
	mirroringMode, err := getMirroringMode(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		err = rbdVol.enableImageMirroring(mirroringMode)
		if err != nil {
			util.ErrorLog(ctx, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &replication.EnableVolumeReplicationResponse{}, nil
}

// DisableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present and the mirroring is enabled on the RBD
// image it will disable the mirroring.
func (cs *ControllerServer) DisableVolumeReplication(ctx context.Context,
	req *replication.DisableVolumeReplicationRequest,
) (*replication.DisableVolumeReplicationResponse, error) {
	rbdVol, cr, err := cs.getVolumeFromID(ctx, req.GetVolumeId(), req.GetSecrets())
	defer cs.cleanup(rbdVol, cr)
	if err != nil {
		return nil, err
	}

	// extract the force option
	force, err := getForceOption(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	switch mirroringInfo.State {
	case librbd.MirrorImageEnabled:
		if !force && !mirroringInfo.Primary {
			return nil, status.Error(codes.InvalidArgument, "image is in non-primary state")
		}
		err = rbdVol.disableImageMirroring(force)
		if err != nil {
			util.ErrorLog(ctx, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
		// TODO: check do we need to return abort after disabling the mirroring
		return &replication.DisableVolumeReplicationResponse{}, nil
	case librbd.MirrorImageDisabled:
		// image is already in disabled state
	default:
		// TODO: use string instead of int for returning valid error message
		return nil, status.Errorf(codes.InvalidArgument, "image is in %d Mode", mirroringInfo.State)
	}

	return &replication.DisableVolumeReplicationResponse{}, nil
}

// PromoteVolume extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in demoted state it
// will promote the volume as primary.
// If the image is already primary it will return success.
func (cs *ControllerServer) PromoteVolume(ctx context.Context,
	req *replication.PromoteVolumeRequest,
) (*replication.PromoteVolumeResponse, error) {
	rbdVol, cr, err := cs.getVolumeFromID(ctx, req.GetVolumeId(), req.GetSecrets())
	defer cs.cleanup(rbdVol, cr)
	if err != nil {
		return nil, err
	}

	// extract the force option
	force, err := getForceOption(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		return nil, status.Errorf(codes.InvalidArgument, "mirroring is not enabled on %s, image is in %d Mode", rbdVol.VolID, mirroringInfo.State)
	}

	// promote secondary to primary
	if !mirroringInfo.Primary {
		err = rbdVol.promoteImage(force)
		if err != nil {
			util.ErrorLog(ctx, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &replication.PromoteVolumeResponse{}, nil
}

// DemoteVolume extracts the RBD volume information from the
// volumeID, If the image is present, mirroring is enabled and the
// image is in promoted state it will demote the volume as secondary.
// If the image is already secondary it will return success.
func (cs *ControllerServer) DemoteVolume(ctx context.Context,
	req *replication.DemoteVolumeRequest,
) (*replication.DemoteVolumeResponse, error) {
	rbdVol, cr, err := cs.getVolumeFromID(ctx, req.GetVolumeId(), req.GetSecrets())
	defer cs.cleanup(rbdVol, cr)
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		return nil, status.Errorf(codes.InvalidArgument, "mirroring is not enabled on %s, image is in %d Mode", rbdVol.VolID, mirroringInfo.State)
	}

	// demote image to secondary
	if mirroringInfo.Primary {
		err = rbdVol.demoteImage()
		if err != nil {
			util.ErrorLog(ctx, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &replication.DemoteVolumeResponse{}, nil
}

// ResyncVolume extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in demoted state.
// If yes it will resync the image to correct the split-brain.
func (cs *ControllerServer) ResyncVolume(ctx context.Context,
	req *replication.ResyncVolumeRequest,
) (*replication.ResyncVolumeResponse, error) {
	rbdVol, cr, err := cs.getVolumeFromID(ctx, req.GetVolumeId(), req.GetSecrets())
	defer cs.cleanup(rbdVol, cr)
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		// TODO: check if the image is not found return valid error message
		// in case of Resync the image will get deleted and gets recreated and
		// it takes time for this operation
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		return nil, status.Error(codes.InvalidArgument, "image mirroring is not enabled")
	}

	// return error if the image is still primary
	if mirroringInfo.Primary {
		return nil, status.Error(codes.InvalidArgument, "image is in primary state")
	}

	err = rbdVol.resyncImage()
	if err != nil {
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	// TODO: check the image state and return its ready to use or not
	mirrorStatus, err := rbdVol.getImageMirroingStatus()
	if err != nil {
		// the image gets recreated after issuing resync in that case return
		// volume as not ready.
		if errors.Is(err, ErrImageNotFound) {
			resp := &replication.ResyncVolumeResponse{
				Ready: false,
			}
			return resp, nil
		}
		util.ErrorLog(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	ready := false
	state := imageMirroringState(mirrorStatus.State)
	if state == upAndStopped || state == upAndReplaying {
		ready = true
	}
	util.UsefulLog(ctx, "image mirroring state=%s, description=%s and lastUpdate=%s", mirrorStatus.State, mirrorStatus.Description, mirrorStatus.LastUpdate)
	resp := &replication.ResyncVolumeResponse{
		Ready: ready,
	}
	return resp, nil
}
