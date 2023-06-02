/*
Copyright 2020 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

func createRBDClone(
	ctx context.Context,
	parentVol, cloneRbdVol *rbdVolume,
	snap *rbdSnapshot,
) error {
	// create snapshot
	err := parentVol.createSnapshot(ctx, snap)
	if err != nil {
		log.ErrorLog(ctx, "failed to create snapshot %s: %v", snap, err)

		return err
	}

	snap.RbdImageName = parentVol.RbdImageName
	// create clone image and delete snapshot
	err = cloneRbdVol.cloneRbdImageFromSnapshot(ctx, snap, parentVol)
	if err != nil {
		log.ErrorLog(
			ctx,
			"failed to clone rbd image %s from snapshot %s: %v",
			cloneRbdVol.RbdImageName,
			snap.RbdSnapName,
			err)
		err = fmt.Errorf(
			"failed to clone rbd image %s from snapshot %s: %w",
			cloneRbdVol.RbdImageName,
			snap.RbdSnapName,
			err)
	}
	errSnap := parentVol.deleteSnapshot(ctx, snap)
	if errSnap != nil {
		log.ErrorLog(ctx, "failed to delete snapshot: %v", errSnap)
		delErr := cloneRbdVol.deleteImage(ctx)
		if delErr != nil {
			log.ErrorLog(ctx, "failed to delete rbd image: %s with error: %v", cloneRbdVol, delErr)
		}

		return err
	}

	return nil
}

// cleanUpSnapshot removes the RBD-snapshot (rbdSnap) from the RBD-image
// (parentVol) and deletes the RBD-image rbdVol.
func cleanUpSnapshot(
	ctx context.Context,
	parentVol *rbdVolume,
	rbdSnap *rbdSnapshot,
	rbdVol *rbdVolume,
) error {
	err := parentVol.deleteSnapshot(ctx, rbdSnap)
	if err != nil {
		if !errors.Is(err, ErrSnapNotFound) {
			log.ErrorLog(ctx, "failed to delete snapshot %q: %v", rbdSnap, err)

			return err
		}
	}

	if rbdVol != nil {
		err := rbdVol.deleteImage(ctx)
		if err != nil {
			if !errors.Is(err, ErrImageNotFound) {
				log.ErrorLog(ctx, "failed to delete rbd image %q with error: %v", rbdVol, err)

				return err
			}
		}
	}

	return nil
}

func generateVolFromSnap(rbdSnap *rbdSnapshot) *rbdVolume {
	vol := new(rbdVolume)
	vol.ClusterID = rbdSnap.ClusterID
	vol.VolID = rbdSnap.VolID
	vol.Monitors = rbdSnap.Monitors
	vol.Pool = rbdSnap.Pool
	vol.JournalPool = rbdSnap.JournalPool
	vol.RadosNamespace = rbdSnap.RadosNamespace
	vol.RbdImageName = rbdSnap.RbdSnapName
	vol.ImageID = rbdSnap.ImageID
	// copyEncryptionConfig cannot be used here because the volume and the
	// snapshot will have the same volumeID which cases the panic in
	// copyEncryptionConfig function.
	vol.blockEncryption = rbdSnap.blockEncryption
	vol.fileEncryption = rbdSnap.fileEncryption

	return vol
}

func undoSnapshotCloning(
	ctx context.Context,
	parentVol *rbdVolume,
	rbdSnap *rbdSnapshot,
	cloneVol *rbdVolume,
	cr *util.Credentials,
) error {
	err := cleanUpSnapshot(ctx, parentVol, rbdSnap, cloneVol)
	if err != nil {
		log.ErrorLog(ctx, "failed to clean up  %s or %s: %v", cloneVol, rbdSnap, err)

		return err
	}
	err = undoSnapReservation(ctx, rbdSnap, cr)

	return err
}
