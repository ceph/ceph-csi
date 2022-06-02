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

	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"

	librbd "github.com/ceph/go-ceph/rbd"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// checkCloneImage check the cloned image exists, if the cloned image is not
// found it will check the temporary cloned snapshot exists, and again it will
// check the snapshot exists on the temporary cloned image, if yes it will
// create a new cloned and delete the temporary snapshot and adds a task to
// flatten the temp cloned image and return success.
//
// if the temporary snapshot does not exists it creates a temporary snapshot on
// temporary cloned image and creates  a new cloned with user-provided image
// features and delete the temporary snapshot and adds a task to flatten the
// temp cloned image and return success
//
// if the temporary clone does not exist and if there is a temporary snapshot
// present on the parent image it will delete the temporary snapshot and
// returns.
func (rv *rbdVolume) checkCloneImage(ctx context.Context, parentVol *rbdVolume) (bool, error) {
	// generate temp cloned volume
	tempClone := rv.generateTempClone()
	defer tempClone.Destroy()

	snap := &rbdSnapshot{}
	defer snap.Destroy()
	snap.RbdSnapName = rv.RbdImageName
	snap.Pool = rv.Pool

	err := tempClone.checkSnapExists(snap)
	if err != nil {
		switch {
		case errors.Is(err, ErrSnapNotFound):
			// as the snapshot is not present, create new snapshot,clone and
			// delete the temporary snapshot
			err = createRBDClone(ctx, tempClone, rv, snap)
			if err != nil {
				return false, err
			}

			return true, nil

		case errors.Is(err, ErrImageNotFound):
			// as the temp clone does not exist,check snapshot exists on parent volume
			// snapshot name is same as temporary clone image
			snap.RbdImageName = tempClone.RbdImageName
			err = parentVol.checkSnapExists(snap)
			if err == nil {
				// the temp clone exists, delete it lets reserve a new ID and
				// create new resources for a cleaner approach
				err = parentVol.deleteSnapshot(ctx, snap)
			}
			if errors.Is(err, ErrSnapNotFound) {
				return false, nil
			}

			return false, err

		default:
			// any error other than the above return error
			return false, err
		}
	}
	// snap will be created after we flatten the temporary cloned image,no
	// need to check for flatten here.
	// as the snap exists,create clone image and delete temporary snapshot
	// and add task to flatten temporary cloned image
	err = rv.cloneRbdImageFromSnapshot(ctx, snap, parentVol)
	if err != nil {
		log.ErrorLog(ctx, "failed to clone rbd image %s from snapshot %s: %v", rv.RbdImageName, snap.RbdSnapName, err)
		err = fmt.Errorf("failed to clone rbd image %s from snapshot %s: %w", rv.RbdImageName, snap.RbdSnapName, err)

		return false, err
	}
	err = tempClone.deleteSnapshot(ctx, snap)
	if err != nil {
		log.ErrorLog(ctx, "failed to delete snapshot: %v", err)

		return false, err
	}

	return true, nil
}

func (rv *rbdVolume) generateTempClone() *rbdVolume {
	tempClone := rbdVolume{}
	tempClone.conn = rv.conn.Copy()
	// The temp clone image need to have deep flatten feature
	f := []string{librbd.FeatureNameLayering, librbd.FeatureNameDeepFlatten}
	tempClone.ImageFeatureSet = librbd.FeatureSetFromNames(f)
	tempClone.ClusterID = rv.ClusterID
	tempClone.Monitors = rv.Monitors
	tempClone.Pool = rv.Pool
	tempClone.RadosNamespace = rv.RadosNamespace
	// The temp cloned image name will be always (rbd image name + "-temp")
	// this name will be always unique, as cephcsi never creates an image with
	// this format for new rbd images
	tempClone.RbdImageName = rv.RbdImageName + "-temp"

	return &tempClone
}

func (rv *rbdVolume) createCloneFromImage(ctx context.Context, parentVol *rbdVolume) error {
	j, err := volJournal.Connect(rv.Monitors, rv.RadosNamespace, rv.conn.Creds)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer j.Destroy()

	err = rv.doSnapClone(ctx, parentVol)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			log.DebugLog(ctx, "Removing clone image %q", rv)
			errDefer := rv.deleteImage(ctx)
			if errDefer != nil {
				log.ErrorLog(ctx, "failed to delete clone image %q: %v", rv, errDefer)
			}
		}
	}()

	err = rv.getImageID()
	if err != nil {
		log.ErrorLog(ctx, "failed to get volume id %s: %v", rv, err)

		return err
	}

	err = parentVol.copyEncryptionConfig(&rv.rbdImage, true)
	if err != nil {
		return fmt.Errorf("failed to copy encryption config for %q: %w", rv, err)
	}

	err = j.StoreImageID(ctx, rv.JournalPool, rv.ReservedID, rv.ImageID)
	if err != nil {
		log.ErrorLog(ctx, "failed to store volume %s: %v", rv, err)

		return err
	}

	// expand the image if the requested size is greater than the current size
	err = rv.expand()
	if err != nil {
		log.ErrorLog(ctx, "failed to resize volume %s: %v", rv, err)

		return err
	}

	return nil
}

func (rv *rbdVolume) doSnapClone(ctx context.Context, parentVol *rbdVolume) error {
	var errClone error

	// generate temp cloned volume
	tempClone := rv.generateTempClone()
	// snapshot name is same as temporary cloned image, This helps to
	// flatten the temporary cloned images as we cannot have more than 510
	// snapshots on an rbd image
	tempSnap := &rbdSnapshot{}
	tempSnap.RbdSnapName = tempClone.RbdImageName
	tempSnap.Pool = rv.Pool

	cloneSnap := &rbdSnapshot{}
	cloneSnap.RbdSnapName = rv.RbdImageName
	cloneSnap.Pool = rv.Pool

	// create snapshot and temporary clone and delete snapshot
	err := createRBDClone(ctx, parentVol, tempClone, tempSnap)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil || errClone != nil {
			cErr := cleanUpSnapshot(ctx, tempClone, cloneSnap, rv)
			if cErr != nil {
				log.ErrorLog(ctx, "failed to cleanup image %s or snapshot %s: %v", cloneSnap, tempClone, cErr)
			}
		}

		if err != nil {
			// cleanup snapshot
			cErr := cleanUpSnapshot(ctx, parentVol, tempSnap, tempClone)
			if cErr != nil {
				log.ErrorLog(ctx, "failed to cleanup image %s or snapshot %s: %v", tempClone, tempSnap, cErr)
			}
		}
	}()

	err = tempClone.unsetAllMetadata(k8s.GetVolumeMetadataKeys())
	if err != nil {
		log.ErrorLog(ctx, "failed to unset volume metadata on temp clone image %q: %v", tempClone, err)

		return err
	}

	// create snap of temp clone from temporary cloned image
	// create final clone
	// delete snap of temp clone
	errClone = createRBDClone(ctx, tempClone, rv, cloneSnap)
	if errClone != nil {
		return errClone
	}

	err = parentVol.copyEncryptionConfig(&rv.rbdImage, true)
	if err != nil {
		return fmt.Errorf("failed to copy encryption config for %q: %w", rv, err)
	}

	return nil
}
