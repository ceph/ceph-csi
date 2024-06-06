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

	"github.com/ceph/ceph-csi/internal/util"

	librbd "github.com/ceph/go-ceph/rbd"
)

// FlattenMode is used to indicate the flatten mode for an RBD image.
type FlattenMode string

const (
	// FlattenModeNever indicates that the image should never be flattened.
	FlattenModeNever FlattenMode = "never"
	// FlattenModeForce indicates that the image with the parent must be flattened.
	FlattenModeForce FlattenMode = "force"
)

// HandleParentImageExistence checks the image's parent.
// if the parent image does not exist and is not in trash, it returns nil.
// if the flattenMode is FlattenModeForce, it flattens the image itself.
// if the parent image is in trash, it returns an error.
// if the parent image exists and is not enabled for mirroring, it returns an error.
func (rv *rbdVolume) HandleParentImageExistence(
	ctx context.Context,
	flattenMode FlattenMode,
) error {
	if rv.ParentName == "" && !rv.ParentInTrash {
		return nil
	}

	if flattenMode == FlattenModeForce {
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
	parentMirroringInfo, err := parent.GetImageMirroringInfo()
	if err != nil {
		return fmt.Errorf(
			"failed to get mirroring info of parent %q of image %q: %w",
			parent, rv, err)
	}

	if parentMirroringInfo.State != librbd.MirrorImageEnabled {
		return fmt.Errorf("%w: failed to enable mirroring on image %q: "+
			"parent image %q is not enabled for mirroring",
			ErrFailedPrecondition, rv, parent)
	}

	return nil
}

// EnableImageMirroring enables mirroring on an image.
func (ri *rbdImage) EnableImageMirroring(mode librbd.ImageMirrorMode) error {
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

// DisableImageMirroring disables mirroring on an image.
func (ri *rbdImage) DisableImageMirroring(force bool) error {
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

// GetImageMirroringInfo gets mirroring information of an image.
func (ri *rbdImage) GetImageMirroringInfo() (*librbd.MirrorImageInfo, error) {
	image, err := ri.open()
	if err != nil {
		return nil, fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()

	info, err := image.GetMirrorImageInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get mirroring info of %q with error: %w", ri, err)
	}

	return info, nil
}

// PromoteImage promotes image to primary.
func (ri *rbdImage) PromoteImage(force bool) error {
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

// ForcePromoteImage promotes image to primary with force option with 2 minutes
// timeout. If there is no response within 2 minutes,the rbd CLI process will be
// killed and an error is returned.
func (rv *rbdVolume) ForcePromoteImage(cr *util.Credentials) error {
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

// DemoteImage demotes image to secondary.
func (ri *rbdImage) DemoteImage() error {
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

// resyncImage resync image to correct the split-brain.
func (ri *rbdImage) resyncImage() error {
	image, err := ri.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()
	err = image.MirrorResync()
	if err != nil {
		return fmt.Errorf("failed to resync image %q with error: %w", ri, err)
	}

	return nil
}

// GetImageMirroringStatus get the mirroring status of an image.
func (ri *rbdImage) GetImageMirroringStatus() (*librbd.GlobalMirrorImageStatus, error) {
	image, err := ri.open()
	if err != nil {
		return nil, fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()
	statusInfo, err := image.GetGlobalMirrorStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get image mirroring status %q with error: %w", ri, err)
	}

	return &statusInfo, nil
}

// GetLocalState returns the local state of the image.
func (ri *rbdImage) GetLocalState() (librbd.SiteMirrorImageStatus, error) {
	localStatus := librbd.SiteMirrorImageStatus{}
	image, err := ri.open()
	if err != nil {
		return localStatus, fmt.Errorf("failed to open image %q with error: %w", ri, err)
	}
	defer image.Close()

	statusInfo, err := image.GetGlobalMirrorStatus()
	if err != nil {
		return localStatus, fmt.Errorf("failed to get image mirroring status %q with error: %w", ri, err)
	}
	localStatus, err = statusInfo.LocalStatus()
	if err != nil {
		return localStatus, fmt.Errorf("failed to get local status: %w", err)
	}

	return localStatus, nil
}
