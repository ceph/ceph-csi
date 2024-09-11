/*
Copyright 2023 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/internal/rbd/types"

	librbd "github.com/ceph/go-ceph/rbd"
)

// repairResyncedImageID updates the existing image ID with new one.
func (rv *rbdVolume) RepairResyncedImageID(ctx context.Context, ready bool) error {
	// During resync operation the local image will get deleted and a new
	// image is recreated by the rbd mirroring. The new image will have a
	// new image ID. Once resync is completed update the image ID in the OMAP
	// to get the image removed from the trash during DeleteVolume.

	// if the image is not completely resynced skip repairing image ID.
	if !ready {
		return nil
	}
	j, err := volJournal.Connect(rv.Monitors, rv.RadosNamespace, rv.conn.Creds)
	if err != nil {
		return err
	}
	defer j.Destroy()
	// reset the image ID which is stored in the existing OMAP
	return rv.repairImageID(ctx, j, true)
}

func DisableVolumeReplication(mirror types.Mirror,
	ctx context.Context,
	primary,
	force bool,
) error {
	if !primary {
		// Return success if the below condition is met
		// Local image is secondary
		// Local image is in up+replaying state

		// If the image is in a secondary and its state is  up+replaying means
		// its a healthy secondary and the image is primary somewhere in the
		// remote cluster and the local image is getting replayed. Return
		// success for the Disabling mirroring as we cannot disable mirroring
		// on the secondary image, when the image on the primary site gets
		// disabled the image on all the remote (secondary) clusters will get
		// auto-deleted. This helps in garbage collecting the volume
		// replication Kubernetes artifacts after failback operation.
		sts, rErr := mirror.GetGlobalMirroringStatus(ctx)
		if rErr != nil {
			return fmt.Errorf("failed to get global state: %w", rErr)
		}

		localStatus, err := sts.GetLocalSiteStatus()
		if err != nil {
			return fmt.Errorf("failed to get local state: %w", ErrInvalidArgument)
		}
		if localStatus.IsUP() && localStatus.GetState() == librbd.MirrorImageStatusStateReplaying.String() {
			return nil
		}

		return fmt.Errorf("%w: secondary image status is up=%t and state=%s",
			ErrInvalidArgument, localStatus.IsUP(), localStatus.GetState())
	}
	err := mirror.DisableMirroring(ctx, force)
	if err != nil {
		return fmt.Errorf("failed to disable image mirroring: %w", err)
	}
	// the image state can be still disabling once we disable the mirroring
	// check the mirroring is disabled or not
	info, err := mirror.GetMirroringInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get mirroring info of image: %w", err)
	}

	// error out if the image is not in disabled state.
	if info.GetState() != librbd.MirrorImageDisabled.String() {
		return fmt.Errorf("%w: image is in %q state, expected state %q", ErrAborted,
			info.GetState(), librbd.MirrorImageDisabled.String())
	}

	return nil
}
