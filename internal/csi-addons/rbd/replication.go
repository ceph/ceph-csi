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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/rbd"
	corerbd "github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
	"github.com/csi-addons/spec/lib/go/replication"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// imageMirroringMode is used to indicate the mirroring mode for an RBD image.
type imageMirroringMode string

const (
	// imageMirrorModeSnapshot uses snapshots to propagate RBD images between
	// ceph clusters.
	imageMirrorModeSnapshot imageMirroringMode = "snapshot"
	// imageMirrorModeJournal uses journaling to propagate RBD images between
	// ceph clusters.
	imageMirrorModeJournal imageMirroringMode = "journal"

	// imageCreationTimeKey is the key to get/set the image creation timestamp
	// on the image metadata. The key is starting with `.rbd` so that it will
	// not get replicated to remote cluster.
	imageCreationTimeKey = ".rbd.image.creation_time"
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

	// flattenModeKey to get the flattenMode from the parameters.
	// (optional) flattenMode decides how to handle images with parent.
	// (default) If set to "never", the image with parent will not be flattened.
	// If set to "force", the image with parent will be flattened.
	flattenModeKey = "flattenMode"
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
	*corerbd.ControllerServer
	// csiID is the unique ID for this CSI-driver deployment.
	csiID string
}

// NewReplicationServer creates a new ReplicationServer which handles
// the Replication Service requests from the CSI-Addons specification.
func NewReplicationServer(instanceID string, c *corerbd.ControllerServer) *ReplicationServer {
	return &ReplicationServer{
		ControllerServer: c,
		csiID:            instanceID,
	}
}

func (rs *ReplicationServer) RegisterService(server grpc.ServiceRegistrar) {
	replication.RegisterControllerServer(server, rs)
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
		return false, status.Error(codes.Internal, err.Error())
	}

	return force, nil
}

// getFlattenMode gets flatten mode from the input GRPC request parameters.
// flattenMode is the key to check the mode in the parameters.
func getFlattenMode(ctx context.Context, parameters map[string]string) (types.FlattenMode, error) {
	val, ok := parameters[flattenModeKey]
	if !ok {
		log.DebugLog(ctx, "%q is not set in parameters, setting to default (%v)",
			flattenModeKey, types.FlattenModeNever)

		return types.FlattenModeNever, nil
	}

	mode := types.FlattenMode(val)
	switch mode {
	case types.FlattenModeForce, types.FlattenModeNever:
		return mode, nil
	}
	log.ErrorLog(ctx, "%q=%q is not supported", flattenModeKey, val)

	return mode, status.Errorf(codes.InvalidArgument, "%q=%q is not supported", flattenModeKey, val)
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
	case imageMirrorModeJournal:
		mirroringMode = librbd.ImageMirrorModeJournal
	default:
		return mirroringMode, status.Errorf(codes.InvalidArgument, "%s %s not supported", imageMirroringKey, val)
	}

	return mirroringMode, nil
}

// validateSchedulingDetails gets the mirroring mode and scheduling details from the
// input GRPC request parameters and validates the scheduling is only supported
// for snapshot mirroring mode.
func validateSchedulingDetails(ctx context.Context, parameters map[string]string) error {
	var err error

	val := parameters[imageMirroringKey]

	switch imageMirroringMode(val) {
	case imageMirrorModeJournal:
		// journal mirror mode does not require scheduling parameters
		if _, ok := parameters[schedulingIntervalKey]; ok {
			log.WarningLog(ctx, "%s parameter cannot be used with %s mirror mode, ignoring it",
				schedulingIntervalKey, string(imageMirrorModeJournal))
		}
		if _, ok := parameters[schedulingStartTimeKey]; ok {
			log.WarningLog(ctx, "%s parameter cannot be used with %s mirror mode, ignoring it",
				schedulingStartTimeKey, string(imageMirrorModeJournal))
		}

		return nil
	case imageMirrorModeSnapshot:
	// If mirroring mode is not set in parameters, we are defaulting mirroring
	// mode to snapshot. Discard empty mirroring mode from validation as it is
	// an optional parameter.
	case "":
	default:
		return status.Error(codes.InvalidArgument, "scheduling is only supported for snapshot mode")
	}

	// validate mandatory interval field
	interval, ok := parameters[schedulingIntervalKey]
	if ok && interval == "" {
		return status.Error(codes.InvalidArgument, "scheduling interval cannot be empty")
	}
	adminStartTime := admin.StartTime(parameters[schedulingStartTimeKey])
	if !ok {
		// startTime is alone not supported it has to be present with interval
		if adminStartTime != "" {
			return status.Errorf(codes.InvalidArgument,
				"%q parameter is supported only with %q",
				schedulingStartTimeKey,
				schedulingIntervalKey)
		}
	}
	if interval != "" {
		err = validateSchedulingInterval(interval)
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	return nil
}

// getSchedulingDetails returns scheduling interval and scheduling startTime.
func getSchedulingDetails(parameters map[string]string) (admin.Interval, admin.StartTime) {
	return admin.Interval(parameters[schedulingIntervalKey]),
		admin.StartTime(parameters[schedulingStartTimeKey])
}

// validateSchedulingInterval return the interval as it is if its ending with
// `m|h|d` or else it will return error.
func validateSchedulingInterval(interval string) error {
	re := regexp.MustCompile(`^\d+[mhd]$`)
	if re.MatchString(interval) {
		return nil
	}

	return errors.New("interval specified without d, h, m suffix")
}

// EnableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present it will enable the mirroring based on the
// user provided information.
func (rs *ReplicationServer) EnableVolumeReplication(ctx context.Context,
	req *replication.EnableVolumeReplicationRequest,
) (*replication.EnableVolumeReplicationResponse, error) {
	volumeID := csicommon.GetIDFromReplication(req)
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	err = validateSchedulingDetails(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)

	mgr := rbd.NewManager(rs.csiID, req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, getGRPCError(err)
	}
	mirror, err := rbdVol.ToMirror()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// extract the mirroring mode
	mirroringMode, err := getMirroringMode(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}
	// extract the flatten mode
	flattenMode, err := getFlattenMode(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}
	if info.GetState() != librbd.MirrorImageEnabled.String() {
		err = rbdVol.HandleParentImageExistence(ctx, flattenMode)
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, getGRPCError(err)
		}
		err = mirror.EnableMirroring(ctx, mirroringMode)
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &replication.EnableVolumeReplicationResponse{}, nil
}

// DisableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present and the mirroring is enabled on the RBD
// image it will disable the mirroring.
func (rs *ReplicationServer) DisableVolumeReplication(ctx context.Context,
	req *replication.DisableVolumeReplicationRequest,
) (*replication.DisableVolumeReplicationResponse, error) {
	volumeID := csicommon.GetIDFromReplication(req)
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

	mgr := rbd.NewManager(rs.csiID, req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, getGRPCError(err)
	}
	mirror, err := rbdVol.ToMirror()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// extract the force option
	force, err := getForceOption(ctx, req.GetParameters())
	if err != nil {
		return nil, err
	}

	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}
	switch info.GetState() {
	// image is already in disabled state
	case librbd.MirrorImageDisabled.String():
	// image mirroring is still disabling
	case librbd.MirrorImageDisabling.String():
		return nil, status.Errorf(codes.Aborted, "%s is in disabling state", volumeID)
	case librbd.MirrorImageEnabled.String():
		err = corerbd.DisableVolumeReplication(mirror, ctx, info.IsPrimary(), force)
		if err != nil {
			return nil, getGRPCError(err)
		}

		return &replication.DisableVolumeReplicationResponse{}, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "image is in %s Mode", info.GetState())
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
	volumeID := csicommon.GetIDFromReplication(req)
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

	mgr := rbd.NewManager(rs.csiID, req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, getGRPCError(err)
	}
	mirror, err := rbdVol.ToMirror()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	if info.GetState() != librbd.MirrorImageEnabled.String() {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"mirroring is not enabled on %s, image is in %s Mode",
			volumeID,
			info.GetState())
	}

	// promote secondary to primary
	if !info.IsPrimary() {
		if req.GetForce() {
			// workaround for https://github.com/ceph/ceph-csi/issues/2736
			// TODO: remove this workaround when the issue is fixed
			err = mirror.ForcePromote(ctx, cr)
		} else {
			err = mirror.Promote(ctx, req.GetForce())
		}
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

	interval, startTime := getSchedulingDetails(req.GetParameters())
	if interval != admin.NoInterval {
		err = mirror.AddSnapshotScheduling(interval, startTime)
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

	return &replication.PromoteVolumeResponse{}, nil
}

// DemoteVolume extracts the RBD volume information from the
// volumeID, If the image is present, mirroring is enabled and the
// image is in promoted state it will demote the volume as secondary.
// If the image is already secondary it will return success.
func (rs *ReplicationServer) DemoteVolume(ctx context.Context,
	req *replication.DemoteVolumeRequest,
) (*replication.DemoteVolumeResponse, error) {
	volumeID := csicommon.GetIDFromReplication(req)
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

	mgr := rbd.NewManager(rs.csiID, req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, getGRPCError(err)
	}
	mirror, err := rbdVol.ToMirror()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	creationTime, err := rbdVol.GetCreationTime(ctx)
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	if info.GetState() != librbd.MirrorImageEnabled.String() {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"mirroring is not enabled on %s, image is in %s Mode",
			volumeID,
			info.GetState())
	}

	// demote image to secondary
	if info.IsPrimary() {
		// store the image creation time for resync
		_, err = rbdVol.GetMetadata(imageCreationTimeKey)
		if err != nil && errors.Is(err, librbd.ErrNotFound) {
			log.DebugLog(ctx, "setting image creation time %s for %s", creationTime, rbdVol)
			err = rbdVol.SetMetadata(imageCreationTimeKey, timestampToString(creationTime))
		}
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, status.Error(codes.Internal, err.Error())
		}

		err = mirror.Demote(ctx)
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &replication.DemoteVolumeResponse{}, nil
}

// checkRemoteSiteStatus checks the state of the remote cluster.
// It returns true if the state of the remote cluster is up and unknown.
func checkRemoteSiteStatus(ctx context.Context, mirrorStatus []types.SiteStatus) bool {
	ready := true
	found := false
	for _, s := range mirrorStatus {
		log.UsefulLog(
			ctx,
			"peer site mirrorUUID=%q, daemon up=%t, mirroring state=%q, description=%q and lastUpdate=%d",
			s.GetMirrorUUID(),
			s.IsUP(),
			s.GetState(),
			s.GetDescription(),
			s.GetLastUpdate())
		if s.GetMirrorUUID() != "" {
			found = true
			// If ready is already "false" do not flip it based on another remote peer status
			if ready && (s.GetState() != librbd.MirrorImageStatusStateUnknown.String() || !s.IsUP()) {
				ready = false
			}
		}
	}

	// Return readiness only if at least one remote peer status was processed
	return found && ready
}

// ResyncVolume extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in demoted state.
// If yes it will resync the image to correct the split-brain.
//
//nolint:gocyclo,cyclop // TODO: reduce complexity
func (rs *ReplicationServer) ResyncVolume(ctx context.Context,
	req *replication.ResyncVolumeRequest,
) (*replication.ResyncVolumeResponse, error) {
	volumeID := csicommon.GetIDFromReplication(req)
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
	mgr := rbd.NewManager(rs.csiID, req.GetParameters(), req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, getGRPCError(err)
	}
	mirror, err := rbdVol.ToMirror()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		// in case of Resync the image will get deleted and gets recreated and
		// it takes time for this operation.
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Aborted, err.Error())
	}

	if info.GetState() != librbd.MirrorImageEnabled.String() {
		return nil, status.Error(codes.InvalidArgument, "image mirroring is not enabled")
	}

	// return error if the image is still primary
	if info.IsPrimary() {
		return nil, status.Error(codes.InvalidArgument, "image is in primary state")
	}

	sts, err := mirror.GetGlobalMirroringStatus(ctx)
	if err != nil {
		// the image gets recreated after issuing resync
		if errors.Is(err, corerbd.ErrImageNotFound) {
			// caller retries till RBD syncs an initial version of the image to
			// report its status in the resync call. Ideally, this line will not
			// be executed as the error would get returned due to getMirroringInfo
			// failing to find an image above.
			return nil, status.Error(codes.Aborted, err.Error())
		}
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}
	ready := false

	localStatus, err := sts.GetLocalSiteStatus()
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return nil, fmt.Errorf("failed to get local status: %w", err)
	}

	log.UsefulLog(
		ctx,
		"local status: daemon up=%t, image mirroring state=%q, description=%q and lastUpdate=%s",
		localStatus.IsUP(),
		localStatus.GetState(),
		localStatus.GetDescription(),
		localStatus.GetLastUpdate())

	//  To recover from split brain (up+error) state the image need to be
	//  demoted and requested for resync on site-a and then the image on site-b
	//  should be demoted. The volume should be marked to ready=true when the
	//  image state on both the clusters are up+unknown because during the last
	//  snapshot syncing the data gets copied first and then image state on the
	//  site-a changes to up+unknown.

	// If the image state on both the sites are up+unknown consider that
	// complete data is synced as the last snapshot
	// gets exchanged between the clusters.
	if localStatus.GetState() == librbd.MirrorImageStatusStateUnknown.String() && localStatus.IsUP() {
		ready = checkRemoteSiteStatus(ctx, sts.GetAllSitesStatus())
	}

	creationTime, err := rbdVol.GetCreationTime(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get image info for %s: %s", rbdVol, err.Error())
	}

	// image creation time is stored in the image metadata. it looks like
	// `"seconds:1692879841 nanos:631526669"`
	// If the image gets resynced the local image creation time will be
	// lost, if the keys is not present in the image metadata then we can
	// assume that the image is already resynced.
	savedImageTime, err := rbdVol.GetMetadata(imageCreationTimeKey)
	if err != nil && !errors.Is(err, librbd.ErrNotFound) {
		return nil, status.Errorf(codes.Internal,
			"failed to get %s key from image metadata for %s: %s",
			imageCreationTimeKey,
			rbdVol,
			err.Error())
	}

	if savedImageTime != "" {
		st, sErr := timestampFromString(savedImageTime)
		if sErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to parse image creation time: %s", sErr.Error())
		}
		log.DebugLog(ctx, "image %s, savedImageTime=%v, currentImageTime=%v", rbdVol, st, creationTime)
		if req.GetForce() && st.Equal(*creationTime) {
			err = mirror.Resync(ctx)
			if err != nil {
				return nil, getGRPCError(err)
			}
		}
	}

	if !ready {
		err = checkVolumeResyncStatus(ctx, localStatus)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	err = rbdVol.RepairResyncedImageID(ctx, ready)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to resync Image ID: %s", err.Error())
	}

	resp := &replication.ResyncVolumeResponse{
		Ready: ready,
	}

	return resp, nil
}

// timestampToString converts the time.Time object to string.
func timestampToString(st *time.Time) string {
	return fmt.Sprintf("seconds:%d nanos:%d", st.Unix(), st.Nanosecond())
}

// timestampFromString parses the timestamp string and returns the time.Time
// object.
func timestampFromString(timestamp string) (time.Time, error) {
	st := time.Time{}
	parts := strings.Fields(timestamp)
	if len(parts) != 2 {
		return st, fmt.Errorf("failed to parse image creation time: %s", timestamp)
	}
	if len(strings.Split(parts[0], ":")) != 2 || len(strings.Split(parts[1], ":")) != 2 {
		return st, fmt.Errorf("failed to parse image creation time: %s", timestamp)
	}
	secondsStr := strings.Split(parts[0], ":")[1]
	nanosStr := strings.Split(parts[1], ":")[1]

	seconds, err := strconv.ParseInt(secondsStr, 10, 64)
	if err != nil {
		return st, fmt.Errorf("failed to parse image creation time seconds: %s", err.Error())
	}

	nanos, err := strconv.ParseInt(nanosStr, 10, 32)
	if err != nil {
		return st, fmt.Errorf("failed to parse image creation time nenos: %s", err.Error())
	}

	st = time.Unix(seconds, nanos)

	return st, nil
}

func getGRPCError(err error) error {
	if err == nil {
		return status.Error(codes.OK, codes.OK.String())
	}

	errorStatusMap := map[error]codes.Code{
		corerbd.ErrImageNotFound:      codes.NotFound,
		util.ErrPoolNotFound:          codes.NotFound,
		corerbd.ErrInvalidArgument:    codes.InvalidArgument,
		corerbd.ErrFlattenInProgress:  codes.Aborted,
		corerbd.ErrAborted:            codes.Aborted,
		corerbd.ErrFailedPrecondition: codes.FailedPrecondition,
		corerbd.ErrUnavailable:        codes.Unavailable,
	}

	for e, code := range errorStatusMap {
		if errors.Is(err, e) {
			return status.Error(code, err.Error())
		}
	}

	// Handle any other non nil error not listed in the map as internal error
	return status.Error(codes.Internal, err.Error())
}

// GetVolumeReplicationInfo extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in primary state.
func (rs *ReplicationServer) GetVolumeReplicationInfo(ctx context.Context,
	req *replication.GetVolumeReplicationInfoRequest,
) (*replication.GetVolumeReplicationInfoResponse, error) {
	volumeID := csicommon.GetIDFromReplication(req)
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}
	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		log.ErrorLog(ctx, "failed to get user credentials: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	if acquired := rs.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer rs.VolumeLocks.Release(volumeID)
	mgr := rbd.NewManager(rs.csiID, nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get volume with id %q: %v", volumeID, err)

		switch {
		case errors.Is(err, corerbd.ErrImageNotFound):
			err = status.Error(codes.NotFound, err.Error())
		case errors.Is(err, util.ErrPoolNotFound):
			err = status.Error(codes.NotFound, err.Error())
		default:
			err = status.Error(codes.Internal, err.Error())
		}

		return nil, err
	}
	mirror, err := rbdVol.ToMirror()
	if err != nil {
		log.ErrorLog(ctx, "failed to convert volume %q to mirror type: %v", rbdVol, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to get info for mirror %q: %v", mirror, err)

		return nil, status.Error(codes.Aborted, err.Error())
	}

	if info.GetState() != librbd.MirrorImageEnabled.String() {
		return nil, status.Error(codes.InvalidArgument, "image mirroring is not enabled")
	}

	// return error if the image is not in primary state
	if !info.IsPrimary() {
		return nil, status.Error(codes.InvalidArgument, "image is not in primary state")
	}

	mirrorStatus, err := mirror.GetGlobalMirroringStatus(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to get status for mirror %q: %v", mirror, err)

		if errors.Is(err, corerbd.ErrImageNotFound) {
			return nil, status.Error(codes.Aborted, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	remoteStatus, err := mirrorStatus.GetRemoteSiteStatus(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to get remote site status for mirror %q: %v", mirror, err)

		if errors.Is(err, librbd.ErrNotExist) {
			return nil, status.Errorf(codes.NotFound, "failed to get remote status: %v", err)
		}

		return nil, status.Errorf(codes.Internal, "failed to get remote status: %v", err)
	}

	description := remoteStatus.GetDescription()
	resp, err := getLastSyncInfo(ctx, description)
	if err != nil {
		log.ErrorLog(ctx, "failed to parse last sync info from %q: %v", description, err)

		if errors.Is(err, corerbd.ErrLastSyncTimeNotFound) {
			return nil, status.Errorf(codes.NotFound, "failed to get last sync info: %v", err)
		}

		return nil, status.Errorf(codes.Internal, "failed to get last sync info: %v", err)
	}

	return resp, nil
}

// This function gets the local snapshot time, last sync snapshot seconds
// and last sync bytes from the description of localStatus and convert
// it into required types.
func getLastSyncInfo(ctx context.Context, description string) (*replication.GetVolumeReplicationInfoResponse, error) {
	// Format of the description will be as followed:
	// description = `replaying, {"bytes_per_second":0.0,"bytes_per_snapshot":81920.0,
	// "last_snapshot_bytes":81920,"last_snapshot_sync_seconds":0,
	// "local_snapshot_timestamp":1684675261,
	// "remote_snapshot_timestamp":1684675261,"replay_state":"idle"}`
	// In case there is no last snapshot bytes returns 0 as the
	// LastSyncBytes is optional.
	// In case there is no last snapshot sync seconds, it returns nil as the
	// LastSyncDuration is optional.
	// In case there is no local snapshot timestamp return an error as the
	// LastSyncTime is required.

	var response replication.GetVolumeReplicationInfoResponse

	if description == "" {
		return nil, fmt.Errorf("empty description: %w", corerbd.ErrLastSyncTimeNotFound)
	}
	log.DebugLog(ctx, "description: %s", description)
	splittedString := strings.SplitN(description, ",", 2)
	if len(splittedString) == 1 {
		return nil, fmt.Errorf("no snapshot details: %w", corerbd.ErrLastSyncTimeNotFound)
	}
	type localStatus struct {
		LocalSnapshotTime    int64  `json:"local_snapshot_timestamp"`
		LastSnapshotBytes    int64  `json:"last_snapshot_bytes"`
		LastSnapshotDuration *int64 `json:"last_snapshot_sync_seconds"`
	}

	var localSnapInfo localStatus
	err := json.Unmarshal([]byte(splittedString[1]), &localSnapInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal local snapshot info: %w", err)
	}

	// If the json unmarsal is successful but the local snapshot time is 0, we
	// need to consider it as an error as the LastSyncTime is required.
	if localSnapInfo.LocalSnapshotTime == 0 {
		return nil, fmt.Errorf("empty local snapshot timestamp: %w", corerbd.ErrLastSyncTimeNotFound)
	}
	if localSnapInfo.LastSnapshotDuration != nil {
		// converts localSnapshotDuration of type int64 to string format with
		// appended `s` seconds required  for time.ParseDuration
		lastDurationTime := fmt.Sprintf("%ds", *localSnapInfo.LastSnapshotDuration)
		// parse Duration from the lastDurationTime string
		lastDuration, err := time.ParseDuration(lastDurationTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse last snapshot duration: %w", err)
		}
		// converts time.Duration to *durationpb.Duration
		response.LastSyncDuration = durationpb.New(lastDuration)
	}

	// converts localSnapshotTime of type int64 to time.Time
	lastUpdateTime := time.Unix(localSnapInfo.LocalSnapshotTime, 0)
	lastSyncTime := timestamppb.New(lastUpdateTime)

	response.LastSyncTime = lastSyncTime
	response.LastSyncBytes = localSnapInfo.LastSnapshotBytes

	return &response, nil
}

func checkVolumeResyncStatus(ctx context.Context, localStatus types.SiteStatus) error {
	// we are considering local snapshot timestamp to check if the resync is
	// started or not, if we dont see local_snapshot_timestamp in the
	// description of localStatus, we are returning error. if we see the local
	// snapshot timestamp in the description we return resyncing started.
	description := localStatus.GetDescription()
	resp, err := getLastSyncInfo(ctx, description)
	if err != nil {
		return fmt.Errorf("failed to get last sync info: %w", err)
	}
	if resp.GetLastSyncTime() == nil {
		return errors.New("last sync time is nil")
	}

	return nil
}
