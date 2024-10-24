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

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ceph/ceph-csi/internal/rbd/types"
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
	created, err := rbdSnap.GetCreationTime(ctx)
	if err != nil {
		return nil, err
	}

	return &csi.Snapshot{
		SizeBytes:      rbdSnap.VolSize,
		SnapshotId:     rbdSnap.VolID,
		SourceVolumeId: rbdSnap.SourceVolumeID,
		CreationTime:   timestamppb.New(*created),
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

// NewSnapshotByID creates a new rbdSnapshot from the rbdVolume.
//
// Parameters:
// - name of the new rbd-image backing the snapshot
// - id of the rbd-snapshot to clone
//
// FIXME: When resolving the Snapshot, the RbdImageName will be set to the name
// of the parent image. This is can cause issues when not accounting for that
// and the Snapshot is deleted; instead of deleting the snapshot image, the
// parent image is removed.
func (rv *rbdVolume) NewSnapshotByID(
	ctx context.Context,
	cr *util.Credentials,
	name string,
	id uint64,
) (types.Snapshot, error) {
	snap := rv.toSnapshot()
	snap.RequestName = name

	// reserveSnap sets snap.{RbdSnapName,ReservedID,VolID}
	err := reserveSnap(ctx, snap, rv, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to create a reservation in the journal for snapshot image %q: %w", snap, err)
	}
	defer func() {
		if err != nil {
			undoErr := undoSnapReservation(ctx, snap, cr)
			if undoErr != nil {
				log.WarningLog(ctx, "failed undoing reservation of snapshot %q: %v", name, undoErr)
			}
		}
	}()

	// a new snapshot image will be created, needs to have a unique name
	snap.RbdImageName = snap.RbdSnapName

	err = rv.Connect(cr)
	if err != nil {
		return nil, err
	}

	err = rv.openIoctx()
	if err != nil {
		return nil, err
	}

	options, err := rv.constructImageOptions(ctx)
	if err != nil {
		return nil, err
	}
	defer options.Destroy()

	err = options.SetUint64(librbd.ImageOptionCloneFormat, 2)
	if err != nil {
		return nil, err
	}

	// indicator to remove the snapshot after a failure
	removeSnap := true
	var snapImage *librbd.Snapshot

	log.DebugLog(ctx, "going to clone snapshot image %q from image %q with snapshot ID %d", snap, rv, id)

	err = librbd.CloneImageByID(rv.ioctx, rv.RbdImageName, id, rv.ioctx, snap.RbdImageName, options)
	if err != nil && !errors.Is(librbd.ErrExist, err) {
		log.ErrorLog(ctx, "failed to clone snapshot %q with id %d: %v", snap, id, err)

		return nil, fmt.Errorf("failed to clone %q with snapshot id %d as new image %q: %w", rv.RbdImageName, id, snap, err)
	}
	defer func() {
		if !removeSnap {
			// success, no need to remove the snapshot image
			return
		}

		if snapImage != nil {
			err = snapImage.Remove()
			if err != nil {
				log.ErrorLog(ctx, "failed to remove snapshot of image %q after failure: %v", snap, err)
			}
		}

		err = librbd.RemoveImage(rv.ioctx, snap.RbdImageName)
		if err != nil {
			log.ErrorLog(ctx, "failed to remove snapshot image %q after failure: %v", snap, err)
		}
	}()

	// update the snapshot image in the journal, after the image info is updated
	j, err := snapJournal.Connect(snap.Monitors, snap.RadosNamespace, cr)
	if err != nil {
		return nil, fmt.Errorf("snapshot image %q failed to connect to journal: %w", snap, err)
	}
	defer j.Destroy()

	err = snap.Connect(cr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect snapshot image %q: %w", snap, err)
	}
	defer snap.Destroy(ctx)

	image, err := snap.open()
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot image %q: %w", snap, err)
	}
	defer image.Close()

	snapImage, err = image.CreateSnapshot(snap.RbdSnapName)
	if err != nil && !errors.Is(librbd.ErrExist, err) {
		return nil, fmt.Errorf("failed to create snapshot on image %q: %w", snap, err)
	}

	err = snap.repairImageID(ctx, j, true)
	if err != nil {
		return nil, fmt.Errorf("failed to repair image id for snapshot image %q: %w", snap, err)
	}

	// all ok, don't remove the snapshot image in a defer statement
	removeSnap = false

	return snap, nil
}
