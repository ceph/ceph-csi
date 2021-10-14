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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
	"github.com/csi-addons/spec/lib/go/replication"
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
	// If the state is unknown means the rbd-mirror daemon is
	// running and the image is demoted on both the clusters.
	unknown imageMirroringState = "unknown"

	// If the state is error means image need resync.
	errorState imageMirroringState = "error"
)

const (
	// mirroringMode + key to get the imageMirroringMode from parameters.
	imageMirroringKey = "mirroringMode"
	// forceKey + key to get the force option from parameters.
	forceKey = "force"

	// schedulingIntervalKey to get the schedulingInterval from the
	// parameters.
	// Interval of time between scheduled snapshots. Typically in the form
	// <num><m,h,d>.
	schedulingIntervalKey = "schedulingInterval"

	// schedulingStartTimeKey to get the schedulingStartTime from the
	// parameters.
	// (optional) StartTime is the time the snapshot schedule
	// begins, can be specified using the ISO 8601 time format.
	schedulingStartTimeKey = "schedulingStartTime"
)

// ReplicationServer struct of rbd CSI driver with supported methods of Replication
// controller server spec.
type ReplicationServer struct {
	// added UnimplementedControllerServer as a member of
	// ControllerServer. if replication spec add more RPC services in the proto
	// file, then we don't need to add all RPC methods leading to forward
	// compatibility.
	*replication.UnimplementedControllerServer
	// Embed ControllerServer as it implements helper functions
	*ControllerServer
}

// getForceOption extracts the force option from the GRPC request parameters.
// If not set, the default will be set to false.
func getForceOption(ctx context.Context, parameters map[string]string) (bool, error) {
	val, ok := parameters[forceKey]
	if !ok {
		log.WarningLog(ctx, "%s is not set in parameters, setting to default (%v)", forceKey, false)

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
		log.WarningLog(
			ctx,
			"%s is not set in parameters, setting to mirroringMode to default (%s)",
			imageMirroringKey,
			imageMirrorModeSnapshot)

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

// getSchedulingDetails gets the mirroring mode and scheduling details from the
// input GRPC request parameters and validates the scheduling is only supported
// for snapshot mirroring mode.
func getSchedulingDetails(parameters map[string]string) (admin.Interval, admin.StartTime, error) {
	admInt := admin.NoInterval
	adminStartTime := admin.NoStartTime
	var err error

	val := parameters[imageMirroringKey]

	switch imageMirroringMode(val) {
	case imageMirrorModeSnapshot:
	// If mirroring mode is not set in parameters, we are defaulting mirroring
	// mode to snapshot. Discard empty mirroring mode from validation as it is
	// an optional parameter.
	case "":
	default:
		return admInt, adminStartTime, status.Error(codes.InvalidArgument, "scheduling is only supported for snapshot mode")
	}

	// validate mandatory interval field
	interval, ok := parameters[schedulingIntervalKey]
	if ok && interval == "" {
		return admInt, adminStartTime, status.Error(codes.InvalidArgument, "scheduling interval cannot be empty")
	}
	adminStartTime = admin.StartTime(parameters[schedulingStartTimeKey])
	if !ok {
		// startTime is alone not supported it has to be present with interval
		if adminStartTime != "" {
			return admInt, admin.NoStartTime, status.Errorf(codes.InvalidArgument,
				"%q parameter is supported only with %q",
				schedulingStartTimeKey,
				schedulingIntervalKey)
		}
	}
	if interval != "" {
		admInt, err = validateSchedulingInterval(interval)
		if err != nil {
			return admInt, admin.NoStartTime, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	return admInt, adminStartTime, nil
}

// validateSchedulingInterval return the interval as it is if its ending with
// `m|h|d` or else it will return error.
func validateSchedulingInterval(interval string) (admin.Interval, error) {
	re := regexp.MustCompile(`^\d+[mhd]$`)
	if re.MatchString(interval) {
		return admin.Interval(interval), nil
	}

	return "", errors.New("interval specified without d, h, m suffix")
}

// EnableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present it will enable the mirroring based on the
// user provided information.
func (rs *ReplicationServer) EnableVolumeReplication(ctx context.Context,
	req *replication.EnableVolumeReplicationRequest,
) (*replication.EnableVolumeReplicationResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	interval, startTime, err := getSchedulingDetails(req.GetParameters())
	if err != nil {
		return nil, err
	}

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)

	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume %s not found", volumeID)
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Errorf(codes.NotFound, "pool %s not found for %s", rbdVol.Pool, volumeID)
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}
	// extract the mirroring mode
	mirroringMode, err := getMirroringMode(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		err = rbdVol.enableImageMirroring(mirroringMode)
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if interval != "" {
		err = rbdVol.addSnapshotScheduling(interval, startTime)
		if err != nil {
			return nil, err
		}
		log.DebugLog(
			ctx,
			"Added scheduling at interval %s, start time %s for volume %s",
			interval,
			startTime,
			rbdVol)
	}

	return &replication.EnableVolumeReplicationResponse{}, nil
}

// DisableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present and the mirroring is enabled on the RBD
// image it will disable the mirroring.
func (rs *ReplicationServer) DisableVolumeReplication(ctx context.Context,
	req *replication.DisableVolumeReplicationRequest,
) (*replication.DisableVolumeReplicationResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)

	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume %s not found", volumeID)
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Errorf(codes.NotFound, "pool %s not found for %s", rbdVol.Pool, volumeID)
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}
	// extract the force option
	force, err := getForceOption(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	switch mirroringInfo.State {
	// image is already in disabled state
	case librbd.MirrorImageDisabled:
	// image mirroring is still disabling
	case librbd.MirrorImageDisabling:
		return nil, status.Errorf(codes.Aborted, "%s is in disabling state", volumeID)
	case librbd.MirrorImageEnabled:
		return disableVolumeReplication(rbdVol, mirroringInfo, force)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "image is in %s Mode", mirroringInfo.State)
	}

	return &replication.DisableVolumeReplicationResponse{}, nil
}

func disableVolumeReplication(rbdVol *rbdVolume,
	mirroringInfo *librbd.MirrorImageInfo,
	force bool) (*replication.DisableVolumeReplicationResponse, error) {
	if !mirroringInfo.Primary {
		// Return success if the below condition is met
		// Local image is secondary
		// Local image is in up+replaying state

		// If the image is in a secondary and its state is  up+replaying means
		// its an healthy secondary and the image is primary somewhere in the
		// remote cluster and the local image is getting replayed. Return
		// success for the Disabling mirroring as we cannot disable mirroring
		// on the secondary image, when the image on the primary site gets
		// disabled the image on all the remote (secondary) clusters will get
		// auto-deleted. This helps in garbage collecting the volume
		// replication Kubernetes artifacts after failback operation.
		localStatus, rErr := rbdVol.getLocalState()
		if rErr != nil {
			return nil, status.Error(codes.Internal, rErr.Error())
		}
		if localStatus.Up && localStatus.State == librbd.MirrorImageStatusStateReplaying {
			return &replication.DisableVolumeReplicationResponse{}, nil
		}

		return nil, status.Errorf(codes.InvalidArgument,
			"secondary image status is up=%t and state=%s",
			localStatus.Up,
			localStatus.State)
	}
	err := rbdVol.disableImageMirroring(force)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// the image state can be still disabling once we disable the mirroring
	// check the mirroring is disabled or not
	mirroringInfo, err = rbdVol.getImageMirroringInfo()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if mirroringInfo.State == librbd.MirrorImageDisabling {
		return nil, status.Errorf(codes.Aborted, "%s is in disabling state", rbdVol.VolID)
	}

	return &replication.DisableVolumeReplicationResponse{}, nil
}

// PromoteVolume extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in demoted state it
// will promote the volume as primary.
// If the image is already primary it will return success.
func (rs *ReplicationServer) PromoteVolume(ctx context.Context,
	req *replication.PromoteVolumeRequest,
) (*replication.PromoteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)

	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume %s not found", volumeID)
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Errorf(codes.NotFound, "pool %s not found for %s", rbdVol.Pool, volumeID)
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"mirroring is not enabled on %s, image is in %d Mode",
			rbdVol.VolID,
			mirroringInfo.State)
	}

	// promote secondary to primary
	if !mirroringInfo.Primary {
		err = rbdVol.promoteImage(req.Force)
		if err != nil {
			log.ErrorLog(ctx, err.Error())
			// In case of the DR the image on the primary site cannot be
			// demoted as the cluster is down, during failover the image need
			// to be force promoted. RBD returns `Device or resource busy`
			// error message if the image cannot be promoted for above reason.
			// Return FailedPrecondition so that replication operator can send
			// request to force promote the image.
			if strings.Contains(err.Error(), "Device or resource busy") {
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			}

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &replication.PromoteVolumeResponse{}, nil
}

// DemoteVolume extracts the RBD volume information from the
// volumeID, If the image is present, mirroring is enabled and the
// image is in promoted state it will demote the volume as secondary.
// If the image is already secondary it will return success.
func (rs *ReplicationServer) DemoteVolume(ctx context.Context,
	req *replication.DemoteVolumeRequest,
) (*replication.DemoteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)

	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume %s not found", volumeID)
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Errorf(codes.NotFound, "pool %s not found for %s", rbdVol.Pool, volumeID)
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}
	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"mirroring is not enabled on %s, image is in %d Mode",
			rbdVol.VolID,
			mirroringInfo.State)
	}

	// demote image to secondary
	if mirroringInfo.Primary {
		err = rbdVol.demoteImage()
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &replication.DemoteVolumeResponse{}, nil
}

// checkRemoteSiteStatus checks the state of the remote cluster.
// It returns true if the state of the remote cluster is up and unknown.
func checkRemoteSiteStatus(ctx context.Context, mirrorStatus *librbd.GlobalMirrorImageStatus) bool {
	ready := true
	for _, s := range mirrorStatus.SiteStatuses {
		if s.MirrorUUID != "" {
			if imageMirroringState(s.State.String()) != unknown && !s.Up {
				log.UsefulLog(
					ctx,
					"peer site mirrorUUID=%s, mirroring state=%s, description=%s and lastUpdate=%s",
					s.MirrorUUID,
					s.State.String(),
					s.Description,
					s.LastUpdate)

				ready = false
			}
		}
	}

	return ready
}

// ResyncVolume extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in demoted state.
// If yes it will resync the image to correct the split-brain.
// FIXME: reduce complexity.
func (rs *ReplicationServer) ResyncVolume(ctx context.Context,
	req *replication.ResyncVolumeRequest,
) (*replication.ResyncVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)
	rbdVol, err := genVolFromVolID(ctx, volumeID, cr, req.GetSecrets())
	defer rbdVol.Destroy()
	if err != nil {
		switch {
		case errors.Is(err, ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume %s not found", volumeID)
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Errorf(codes.NotFound, "pool %s not found for %s", rbdVol.Pool, volumeID)
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}

	mirroringInfo, err := rbdVol.getImageMirroringInfo()
	if err != nil {
		// in case of Resync the image will get deleted and gets recreated and
		// it takes time for this operation.
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}

	if mirroringInfo.State != librbd.MirrorImageEnabled {
		return nil, status.Error(codes.InvalidArgument, "image mirroring is not enabled")
	}

	// return error if the image is still primary
	if mirroringInfo.Primary {
		return nil, status.Error(codes.InvalidArgument, "image is in primary state")
	}

	mirrorStatus, err := rbdVol.getImageMirroringStatus()
	if err != nil {
		// the image gets recreated after issuing resync
		if errors.Is(err, ErrImageNotFound) {
			// caller retries till RBD syncs an initial version of the image to
			// report its status in the resync call. Ideally, this line will not
			// be executed as the error would get returned due to getImageMirroringInfo
			// failing to find an image above.
			return nil, status.Error(codes.Aborted, err.Error())
		}
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}
	ready := false

	localStatus, err := mirrorStatus.LocalStatus()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, fmt.Errorf("failed to get local status: %w", err)
	}
	state := imageMirroringState(localStatus.State.String())
	//  To recover from split brain (up+error) state the image need to be
	//  demoted and requested for resync on site-a and then the image on site-b
	//  should be demoted. The volume should be marked to ready=true when the
	//  image state on both the clusters are up+unknown because during the last
	//  snapshot syncing the data gets copied first and then image state on the
	//  site-a changes to up+unknown.

	// If the image state on both the sites are up+unknown consider that
	// complete data is synced as the last snapshot
	// gets exchanged between the clusters.
	if state == unknown && localStatus.Up {
		ready = checkRemoteSiteStatus(ctx, mirrorStatus)
	}

	if resyncRequired(localStatus) {
		err = rbdVol.resyncImage()
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, status.Error(codes.Internal, err.Error())
		}

		// If we issued a resync, return a non-final error as image needs to be recreated
		// locally. Caller retries till RBD syncs an initial version of the image to
		// report its status in the resync request.
		return nil, status.Error(codes.Unavailable, "awaiting initial resync due to split brain")
	}

	// convert the last update time to UTC
	lastUpdateTime := time.Unix(localStatus.LastUpdate, 0).UTC()
	log.UsefulLog(
		ctx,
		"image mirroring state=%s, description=%s and lastUpdate=%s",
		localStatus.State.String(),
		localStatus.Description,
		lastUpdateTime)

	resp := &replication.ResyncVolumeResponse{
		Ready: ready,
	}

	return resp, nil
}

// resyncRequired returns true if local image is in split-brain state and image
// needs resync.
func resyncRequired(localStatus librbd.SiteMirrorImageStatus) bool {
	// resync is required if the image is in error state or the description
	// contains split-brain message.
	// In some corner cases like `re-player shutdown` the local image will not
	// be in an error state. It would be also worth considering the `description`
	// field to make sure about split-brain.
	splitBrain := "split-brain"
	if strings.Contains(localStatus.State.String(), string(errorState)) ||
		strings.Contains(localStatus.Description, splitBrain) {
		return true
	}

	return false
}
