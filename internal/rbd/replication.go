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

	librbd "github.com/ceph/go-ceph/rbd"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/rbd/types"
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

// checkSecondaryForDisable validates whether mirroring can be disabled on a
// secondary image. It returns nil when the secondary is in a safe state:
//   - up+replaying: remote has promoted, local is actively replaying.
//   - up+unknown on both local and remote: final sync is done, no promotion yet.
//
// Mirroring on a secondary is auto-disabled when the primary disables it,
// so returning success here enables garbage collection of Kubernetes
// volume replication artifacts after a failback operation.
func checkSecondaryForDisable(ctx context.Context, mirror types.Mirror) error {
	sts, err := mirror.GetGlobalMirroringStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get global state: %w", err)
	}

	localStatus, err := sts.GetLocalSiteStatus()
	if err != nil {
		return fmt.Errorf("failed to get local state: %w", rbderrors.ErrInvalidArgument)
	}

	localUp := localStatus.IsUP()
	localState := localStatus.GetState()

	// Remote cluster has already promoted the image.
	// The local secondary is in up+replaying state, actively receiving
	// data from the new primary.
	if localUp && localState == librbd.MirrorImageStatusStateReplaying.String() {
		return nil
	}

	// Remote cluster has not yet promoted the image.
	// Both local and remote are in up+unknown state because the final
	// sync has completed but no promotion has happened yet.
	if localUp && localState == librbd.MirrorImageStatusStateUnknown.String() {
		rmStatus, err := sts.GetRemoteSiteStatus(ctx)
		if err != nil {
			return fmt.Errorf("failed to get remote state: %w", err)
		}
		if rmStatus.IsUP() && rmStatus.GetState() == librbd.MirrorImageStatusStateUnknown.String() {
			return nil
		}
	}

	return fmt.Errorf("%w: secondary image status is up=%t and state=%s",
		rbderrors.ErrInvalidArgument, localUp, localState)
}

func DisableVolumeReplication(mirror types.Mirror,
	ctx context.Context,
	primary,
	force bool,
) error {
	if !primary {
		return checkSecondaryForDisable(ctx, mirror)
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
		return fmt.Errorf("%w: image is in %q state, expected state %q", rbderrors.ErrAborted,
			info.GetState(), librbd.MirrorImageDisabled.String())
	}

	return nil
}
