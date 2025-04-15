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
	"fmt"
	"strings"
	"time"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	librbd "github.com/ceph/go-ceph/rbd"
)

// HandleParentImageExistence checks the image's parent.
// if the parent image does not exist and is not in trash, it returns nil.
// if the flattenMode is FlattenModeForce, it flattens the image itself.
// if the parent image is in trash, it returns an error.
// if the parent image exists and is not enabled for mirroring, it returns an error.
func (rv *rbdVolume) HandleParentImageExistence(
	ctx context.Context,
	mode types.FlattenMode,
) error {
	if rv.ParentName == "" && !rv.ParentInTrash {
		return nil
	}
	if mode == types.FlattenModeForce {
		// Delete temp image that exists for volume datasource since
		// it is no longer required when the live image is flattened.
		err := rv.DeleteTempImage(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete temporary rbd image %s: %w", rv, err)
		}

		err = rv.flattenRbdImage(ctx, true, 0, 0)
		if err != nil {
			return fmt.Errorf("failed to flatten image %s: %w", rv, err)
		}
	}

	if rv.ParentInTrash {
		return fmt.Errorf("%w: failed to enable mirroring on image %q:"+
			" parent is in trash",
			rbderrors.ErrFailedPrecondition, rv)
	}

	parent, err := rv.getParent()
	if err != nil {
		return fmt.Errorf("failed to get parent of image %s: %w", rv, err)
	}
	parentMirroringInfo, err := parent.GetMirroringInfo(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to get mirroring info of parent %q of image %q: %w",
			parent, rv, err)
	}
	if parentMirroringInfo.GetState() != librbd.MirrorImageEnabled.String() {
		return fmt.Errorf("%w: failed to enable mirroring on image %q: "+
			"parent image %q is not enabled for mirroring",
			rbderrors.ErrFailedPrecondition, rv, parent)
	}

	return nil
}

// check that rbdVolume implements the types.Mirror interface.
var _ types.Mirror = &rbdVolume{}

// EnableMirroring enables mirroring on an image.
func (ri *rbdImage) EnableMirroring(_ context.Context, mode librbd.ImageMirrorMode) error {
	image, err := ri.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()

	err = image.MirrorEnable(mode)
	if err != nil {
		return fmt.Errorf("failed to enable mirroring on %q with error: %w", ri, err)
	}

	return nil
}

// DisableMirroring disables mirroring on an image.
func (ri *rbdImage) DisableMirroring(_ context.Context, force bool) error {
	image, err := ri.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()

	err = image.MirrorDisable(force)
	if err != nil {
		return fmt.Errorf("failed to disable mirroring on %q with error: %w", ri, err)
	}

	return nil
}

// GetMirroringInfo gets mirroring information of an image.
func (ri *rbdImage) GetMirroringInfo(_ context.Context) (types.MirrorInfo, error) {
	image, err := ri.open()
	if err != nil {
		return nil, fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()

	info, err := image.GetMirrorImageInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get mirroring info of %q with error: %w", ri, err)
	}

	return ImageStatus{MirrorImageInfo: info}, nil
}

// Promote promotes image to primary.
func (ri *rbdImage) Promote(_ context.Context, force bool) error {
	image, err := ri.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()
	err = image.MirrorPromote(force)
	if err != nil {
		return fmt.Errorf("failed to promote image %q with error: %w", ri, err)
	}

	return nil
}

// ForcePromote promotes image to primary with force option with 2 minutes
// timeout. If there is no response within 2 minutes,the rbd CLI process will be
// killed and an error is returned.
func (rv *rbdVolume) ForcePromote(ctx context.Context, cr *util.Credentials) error {
	promoteArgs := []string{
		"mirror", "image", "promote",
		rv.String(),
		"--force",
		"--id", cr.ID,
		"-m", rv.Monitors,
		"--keyfile=" + cr.KeyFile,
	}
	_, stderr, err := util.ExecCommandWithTimeout(
		ctx,
		// 2 minutes timeout as the Replication RPC timeout is 2.5 minutes.
		2*time.Minute,
		"rbd",
		promoteArgs...,
	)
	if err != nil {
		return fmt.Errorf("failed to promote image %q with error: %w", rv, err)
	}

	if stderr != "" {
		return fmt.Errorf("failed to promote image %q with stderror: %s", rv, stderr)
	}

	return nil
}

// Demote demotes image to secondary.
func (ri *rbdImage) Demote(_ context.Context) error {
	image, err := ri.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()
	err = image.MirrorDemote()
	if err != nil {
		return fmt.Errorf("failed to demote image %q with error: %w", ri, err)
	}

	return nil
}

// Resync resync image to correct the split-brain.
func (ri *rbdImage) Resync(_ context.Context) error {
	image, err := ri.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()
	err = image.MirrorResync()
	if err != nil {
		return fmt.Errorf("failed to resync image %q with error: %w", ri, err)
	}

	// If we issued a resync, return a non-final error as image needs to be recreated
	// locally. Caller retries till RBD syncs an initial version of the image to
	// report its status in the resync request.
	return fmt.Errorf("%w: awaiting initial resync due to split brain", rbderrors.ErrUnavailable)
}

// GetGlobalMirroringStatus get the mirroring status of an image.
func (ri *rbdImage) GetGlobalMirroringStatus(_ context.Context) (types.GlobalStatus, error) {
	image, err := ri.open()
	if err != nil {
		return nil, fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()
	statusInfo, err := image.GetGlobalMirrorStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get image mirroring status %q with error: %w", ri, err)
	}

	return GlobalMirrorStatus{GlobalMirrorImageStatus: statusInfo}, nil
}

// ImageStatus is a wrapper around librbd.MirrorImageInfo that contains the
// image mirror status.
type ImageStatus struct {
	*librbd.MirrorImageInfo
}

func (status ImageStatus) GetState() string {
	return status.State.String()
}

func (status ImageStatus) IsPrimary() bool {
	return status.Primary
}

// GlobalMirrorStatus is a wrapper around librbd.GlobalMirrorImageStatus that contains the
// global mirror image status.
type GlobalMirrorStatus struct {
	librbd.GlobalMirrorImageStatus
}

func (status GlobalMirrorStatus) GetState() string {
	return status.GlobalMirrorImageStatus.Info.State.String()
}

func (status GlobalMirrorStatus) IsPrimary() bool {
	return status.GlobalMirrorImageStatus.Info.Primary
}

func (status GlobalMirrorStatus) GetLocalSiteStatus() (types.SiteStatus, error) {
	s, err := status.GlobalMirrorImageStatus.LocalStatus()
	if err != nil {
		err = fmt.Errorf("failed to get local site status: %w", err)
	}

	return SiteMirrorImageStatus{
		SiteMirrorImageStatus: s,
	}, err
}

func (status GlobalMirrorStatus) GetAllSitesStatus() []types.SiteStatus {
	var siteStatuses []types.SiteStatus
	for _, ss := range status.SiteStatuses {
		siteStatuses = append(siteStatuses, SiteMirrorImageStatus{SiteMirrorImageStatus: ss})
	}

	return siteStatuses
}

// RemoteStatus returns one SiteMirrorImageStatus item from the SiteStatuses
// slice that corresponds to the remote site's status. If the remote status
// is not found than the error ErrNotExist will be returned.
func (status GlobalMirrorStatus) GetRemoteSiteStatus(ctx context.Context) (types.SiteStatus, error) {
	var (
		ss  librbd.SiteMirrorImageStatus
		err error = librbd.ErrNotExist
	)

	for i := range status.SiteStatuses {
		log.DebugLog(
			ctx,
			"Site status of MirrorUUID: %s, state: %s, description: %s, lastUpdate: %v, up: %t",
			status.SiteStatuses[i].MirrorUUID,
			status.SiteStatuses[i].State,
			status.SiteStatuses[i].Description,
			status.SiteStatuses[i].LastUpdate,
			status.SiteStatuses[i].Up)

		if status.SiteStatuses[i].MirrorUUID != "" {
			ss = status.SiteStatuses[i]
			err = nil

			break
		}
	}

	return SiteMirrorImageStatus{SiteMirrorImageStatus: ss}, err
}

// SiteMirrorImageStatus is a wrapper around librbd.SiteMirrorImageStatus that contains the
// site mirror image status.
type SiteMirrorImageStatus struct {
	librbd.SiteMirrorImageStatus
}

func (status SiteMirrorImageStatus) GetMirrorUUID() string {
	return status.MirrorUUID
}

func (status SiteMirrorImageStatus) GetState() string {
	return status.State.String()
}

func (status SiteMirrorImageStatus) GetDescription() string {
	return status.Description
}

func (status SiteMirrorImageStatus) IsUP() bool {
	return status.Up
}

func (status SiteMirrorImageStatus) GetLastUpdate() time.Time {
	// convert the last update time to UTC
	return time.Unix(status.LastUpdate, 0).UTC()
}

func (status SiteMirrorImageStatus) GetLastSyncInfo(ctx context.Context) (types.SyncInfo, error) {
	return newSyncInfo(ctx, status.Description)
}

type syncInfo struct {
	LocalSnapshotTime    int64       `json:"local_snapshot_timestamp"`
	LastSnapshotBytes    int64       `json:"last_snapshot_bytes"`
	LastSnapshotDuration *int64      `json:"last_snapshot_sync_seconds"`
	ReplayState          replayState `json:"replay_state"`
}

type replayState string

const (
	idle    replayState = "idle"
	syncing replayState = "syncing"
)

// Type assertion for ensuring an implementation of the full SyncInfo interface.
var _ types.SyncInfo = &syncInfo{}

func newSyncInfo(ctx context.Context, description string) (types.SyncInfo, error) {
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

	if description == "" {
		return nil, fmt.Errorf("empty description: %w", rbderrors.ErrLastSyncTimeNotFound)
	}
	log.DebugLog(ctx, "description: %s", description)
	splittedString := strings.SplitN(description, ",", 2)
	if len(splittedString) == 1 {
		return nil, fmt.Errorf("no snapshot details: %w", rbderrors.ErrLastSyncTimeNotFound)
	}

	var localSnapInfo syncInfo
	err := json.Unmarshal([]byte(splittedString[1]), &localSnapInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal description %q into syncInfo: %w", description, err)
	}

	// If the json unmarsal is successful but the local snapshot time is 0, we
	// need to consider it as an error as the LastSyncTime is required.
	if localSnapInfo.LocalSnapshotTime == 0 {
		return nil, fmt.Errorf("empty local snapshot timestamp: %w", rbderrors.ErrLastSyncTimeNotFound)
	}

	return &localSnapInfo, nil
}

func (si *syncInfo) GetLastSyncTime() time.Time {
	// converts localSnapshotTime of type int64 to time.Time
	return time.Unix(si.LocalSnapshotTime, 0)
}

func (si *syncInfo) GetLastSyncBytes() int64 {
	return si.LastSnapshotBytes
}

func (si *syncInfo) GetLastSyncDuration() *time.Duration {
	var duration time.Duration

	if si.LastSnapshotDuration == nil {
		duration = time.Duration(0)
	} else {
		// time.Duration is in nanoseconds
		duration = time.Duration(*si.LastSnapshotDuration) * time.Second
	}

	return &duration
}

func (si *syncInfo) IsSyncing() bool {
	return si.ReplayState == syncing
}
