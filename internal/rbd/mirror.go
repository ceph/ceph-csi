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
	"fmt"
	"time"

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
			return fmt.Errorf("failed to delete temporary rbd image: %w", err)
		}

		err = rv.flattenRbdImage(ctx, true, 0, 0)
		if err != nil {
			return err
		}
	}

	if rv.ParentInTrash {
		return fmt.Errorf("%w: failed to enable mirroring on image %q:"+
			" parent is in trash",
			ErrFailedPrecondition, rv)
	}

	parent, err := rv.getParent()
	if err != nil {
		return err
	}
	parentMirroringInfo, err := parent.GetMirroringInfo()
	if err != nil {
		return fmt.Errorf(
			"failed to get mirroring info of parent %q of image %q: %w",
			parent, rv, err)
	}
	if parentMirroringInfo.GetState() != librbd.MirrorImageEnabled.String() {
		return fmt.Errorf("%w: failed to enable mirroring on image %q: "+
			"parent image %q is not enabled for mirroring",
			ErrFailedPrecondition, rv, parent)
	}

	return nil
}

// check that rbdVolume implements the types.Mirror interface.
var _ types.Mirror = &rbdVolume{}

// EnableMirroring enables mirroring on an image.
func (ri *rbdImage) EnableMirroring(mode librbd.ImageMirrorMode) error {
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
func (ri *rbdImage) DisableMirroring(force bool) error {
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
func (ri *rbdImage) GetMirroringInfo() (types.MirrorInfo, error) {
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
func (ri *rbdImage) Promote(force bool) error {
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
func (rv *rbdVolume) ForcePromote(cr *util.Credentials) error {
	promoteArgs := []string{
		"mirror", "image", "promote",
		rv.String(),
		"--force",
		"--id", cr.ID,
		"-m", rv.Monitors,
		"--keyfile=" + cr.KeyFile,
	}
	_, stderr, err := util.ExecCommandWithTimeout(
		context.TODO(),
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
func (ri *rbdImage) Demote() error {
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
func (ri *rbdImage) Resync() error {
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
	return fmt.Errorf("%w: awaiting initial resync due to split brain", ErrUnavailable)
}

// GetGlobalMirroringStatus get the mirroring status of an image.
func (ri *rbdImage) GetGlobalMirroringStatus() (types.GlobalStatus, error) {
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
