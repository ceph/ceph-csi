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

	klog "k8s.io/klog/v2"
)

func createRBDClone(ctx context.Context, parentVol, cloneRbdVol *rbdVolume, snap *rbdSnapshot, cr *util.Credentials) error {
	// create snapshot
	err := parentVol.createSnapshot(ctx, snap)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create snapshot %s: %v"), snap, err)
		return err
	}

	snap.RbdImageName = parentVol.RbdImageName
	// create clone image and delete snapshot
	err = cloneRbdVol.cloneRbdImageFromSnapshot(ctx, snap)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to clone rbd image %s from snapshot %s: %v"), cloneRbdVol.RbdImageName, snap.RbdSnapName, err)
		err = fmt.Errorf("failed to clone rbd image %s from snapshot %s: %w", cloneRbdVol.RbdImageName, snap.RbdSnapName, err)
	}
	errSnap := parentVol.deleteSnapshot(ctx, snap)
	if errSnap != nil {
		klog.Errorf(util.Log(ctx, "failed to delete snapshot: %v"), errSnap)
		delErr := deleteImage(ctx, cloneRbdVol, cr)
		if delErr != nil {
			klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s with error: %v"), cloneRbdVol, delErr)
		}
		return err
	}

	err = cloneRbdVol.getImageInfo()
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get rbd image: %s details with error: %v"), cloneRbdVol, err)
		delErr := deleteImage(ctx, cloneRbdVol, cr)
		if delErr != nil {
			klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s with error: %v"), cloneRbdVol, delErr)
		}
		return err
	}

	return nil
}

func cleanUpSnapshot(ctx context.Context, parentVol *rbdVolume, rbdSnap *rbdSnapshot, rbdVol *rbdVolume, cr *util.Credentials) error {
	err := parentVol.deleteSnapshot(ctx, rbdSnap)
	if err != nil {
		if !errors.Is(err, ErrSnapNotFound) {
			klog.Errorf(util.Log(ctx, "failed to delete snapshot: %v"), err)
			return err
		}
	}
	err = deleteImage(ctx, rbdVol, cr)
	if err != nil {
		if !errors.Is(err, ErrImageNotFound) {
			klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s/%s with error: %v"), rbdVol.Pool, rbdVol.VolName, err)
			return err
		}
	}
	return nil
}

func generateVolFromSnap(rbdSnap *rbdSnapshot) *rbdVolume {
	vol := new(rbdVolume)
	vol.ClusterID = rbdSnap.ClusterID
	vol.VolID = rbdSnap.SnapID
	vol.Monitors = rbdSnap.Monitors
	vol.Pool = rbdSnap.Pool
	vol.JournalPool = rbdSnap.JournalPool
	vol.RbdImageName = rbdSnap.RbdSnapName
	vol.ImageID = rbdSnap.ImageID
	return vol
}

func undoSnapshotCloning(ctx context.Context, parentVol *rbdVolume, rbdSnap *rbdSnapshot, cloneVol *rbdVolume, cr *util.Credentials) error {
	err := cleanUpSnapshot(ctx, parentVol, rbdSnap, cloneVol, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to clean up  %s or %s: %v"), cloneVol, rbdSnap, err)
		return err
	}
	err = undoSnapReservation(ctx, rbdSnap, cr)
	return err
}
