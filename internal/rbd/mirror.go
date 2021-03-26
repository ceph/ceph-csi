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

	"github.com/ceph/ceph-csi/internal/util"

	librbd "github.com/ceph/go-ceph/rbd"
)

// enableImageMirroring enables mirroring on an image.
func (rv *rbdVolume) enableImageMirroring(mode librbd.ImageMirrorMode) error {
	image, err := rv.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", rv.String(), err)
	}
	defer image.Close()

	err = image.MirrorEnable(mode)
	if err != nil {
		return fmt.Errorf("failed to enable mirroring on %q with error: %w", rv.String(), err)
	}
	return nil
}

// disableImageMirroring disables mirroring on an image.
func (rv *rbdVolume) disableImageMirroring(force bool) error {
	image, err := rv.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", rv.String(), err)
	}
	defer image.Close()

	err = image.MirrorDisable(force)
	if err != nil {
		return fmt.Errorf("failed to disable mirroring on %q with error: %w", rv.String(), err)
	}
	return nil
}

// getImageMirroringInfo gets mirroring information of an image.
func (rv *rbdVolume) getImageMirroringInfo() (*librbd.MirrorImageInfo, error) {
	image, err := rv.open()
	if err != nil {
		return nil, fmt.Errorf("failed to open image %q with error: %w", rv.String(), err)
	}
	defer image.Close()

	info, err := image.GetMirrorImageInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get mirroring info of %q with error: %w", rv.String(), err)
	}
	return info, nil
}

// promoteImage promotes image to primary.
func (rv *rbdVolume) promoteImage(force bool) error {
	image, err := rv.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", rv.String(), err)
	}
	defer image.Close()
	err = image.MirrorPromote(force)
	if err != nil {
		return fmt.Errorf("failed to promote image %q with error: %w", rv.String(), err)
	}
	return nil
}

// demoteImage demotes image to secondary.
func (rv *rbdVolume) demoteImage() error {
	image, err := rv.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", rv.String(), err)
	}
	defer image.Close()
	err = image.MirrorDemote()
	if err != nil {
		return fmt.Errorf("failed to demote image %q with error: %w", rv.String(), err)
	}
	return nil
}

// resyncImage resync image to correct the split-brain.
func (rv *rbdVolume) resyncImage() error {
	image, err := rv.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q with error: %w", rv.String(), err)
	}
	defer image.Close()
	err = image.MirrorResync()
	if err != nil {
		return fmt.Errorf("failed to resync image %q with error: %w", rv.String(), err)
	}
	return nil
}

type imageMirrorStatus struct {
	Name        string `json:"name"`  // name of the rbd image
	State       string `json:"state"` // rbd image state
	Description string `json:"description"`
	LastUpdate  string `json:"last_update"`
}

// FIXME: once https://github.com/ceph/go-ceph/issues/460 is fixed use go-ceph.
// getImageMirroingStatus get the mirroring status of an image.
func (rv *rbdVolume) getImageMirroingStatus() (*imageMirrorStatus, error) {
	// rbd mirror image status --format=json info [image-spec | snap-spec]
	var imgStatus imageMirrorStatus
	stdout, stderr, err := util.ExecCommand(
		context.TODO(),
		"rbd",
		"-m", rv.Monitors,
		"--id", rv.conn.Creds.ID,
		"--keyfile="+rv.conn.Creds.KeyFile,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"mirror",
		"image",
		"status",
		rv.String())
	if err != nil {
		if strings.Contains(stderr, "rbd: error opening image "+rv.RbdImageName+
			": (2) No such file or directory") {
			return nil, util.JoinErrors(ErrImageNotFound, err)
		}
		return nil, err
	}

	if stdout != "" {
		err = json.Unmarshal([]byte(stdout), &imgStatus)
		if err != nil {
			return nil, fmt.Errorf("unmarshal failed (%w), raw buffer response: %s", err, stdout)
		}
	}
	return &imgStatus, nil
}
