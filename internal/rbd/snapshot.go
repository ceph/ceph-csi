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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		delErr := cloneRbdVol.Delete(ctx)
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
		err := rbdVol.Delete(ctx)
		if err != nil {
			if !errors.Is(err, ErrImageNotFound) {
				log.ErrorLog(ctx, "failed to delete rbd image %q with error: %v", rbdVol, err)

				return err
			}
		}
	}

	return nil
}

func (rv *rbdVolume) toSnapshot() *rbdSnapshot {
	return &rbdSnapshot{
		rbdImage: rbdImage{
			ClusterID:      rv.ClusterID,
			VolID:          rv.VolID,
			VolSize:        rv.VolSize,
			Monitors:       rv.Monitors,
			Pool:           rv.Pool,
			JournalPool:    rv.JournalPool,
			RadosNamespace: rv.RadosNamespace,
			RbdImageName:   rv.RbdImageName,
			ImageID:        rv.ImageID,
			CreatedAt:      rv.CreatedAt,
			// copyEncryptionConfig cannot be used here because the volume and the
			// snapshot will have the same volumeID which cases the panic in
			// copyEncryptionConfig function.
			blockEncryption: rv.blockEncryption,
			fileEncryption:  rv.fileEncryption,
		},
	}
}

func (rbdSnap *rbdSnapshot) toVolume() *rbdVolume {
	return &rbdVolume{
		rbdImage: rbdImage{
			ClusterID:      rbdSnap.ClusterID,
			VolID:          rbdSnap.VolID,
			Monitors:       rbdSnap.Monitors,
			Pool:           rbdSnap.Pool,
			JournalPool:    rbdSnap.JournalPool,
			RadosNamespace: rbdSnap.RadosNamespace,
			RbdImageName:   rbdSnap.RbdSnapName,
			ImageID:        rbdSnap.ImageID,
			CreatedAt:      rbdSnap.CreatedAt,
			// copyEncryptionConfig cannot be used here because the volume and the
			// snapshot will have the same volumeID which cases the panic in
			// copyEncryptionConfig function.
			blockEncryption: rbdSnap.blockEncryption,
			fileEncryption:  rbdSnap.fileEncryption,
		},
	}
}

func (rbdSnap *rbdSnapshot) ToCSI(ctx context.Context) (*csi.Snapshot, error) {
	return &csi.Snapshot{
		SizeBytes:      rbdSnap.VolSize,
		SnapshotId:     rbdSnap.VolID,
		SourceVolumeId: rbdSnap.SourceVolumeID,
		CreationTime:   timestamppb.New(*rbdSnap.CreatedAt),
		ReadyToUse:     true,
	}, nil
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
