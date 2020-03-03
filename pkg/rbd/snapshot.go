/*
Copyright 2019 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func createRBDClone(ctx context.Context, rbdVol, cloneRbdVol *rbdVolume, cr *util.Credentials) (bool, error) {

	// generate snapshot from parent volume
	snap := generateSnapFromVol(rbdVol)

	// update snapshot name as cloned volume name as it will be always unique
	snap.RbdSnapName = cloneRbdVol.RbdImageName

	// create snapshot
	err := createSnapshot(ctx, snap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create snapshot: %v"), err)
		return false, status.Error(codes.Internal, err.Error())

	}

	cloneFailed := false
	ready := true
	// create clone image and delete snapshot
	err = cloneRbdImageFromSnapshot(ctx, cloneRbdVol, snap, cr)
	if err != nil {
		if _, ok := err.(ErrFlattenInProgress); ok {
			ready = false
		} else {
			klog.Errorf(util.Log(ctx, "failed to clone rbd image %s from snapshot %s: %v"), cloneRbdVol.RbdImageName, snap.RbdSnapName, err)
			cloneFailed = true
		}
	}

	err = flattenRbdImage(ctx, cloneRbdVol, rbdHardMaxCloneDepth, cr)
	if err != nil {
		if _, ok := err.(ErrFlattenInProgress); !ok {
			delErr := deleteImage(ctx, cloneRbdVol, cr)
			if delErr != nil {
				klog.Errorf(util.Log(ctx, "rbd: failed to delete %s/%s using mon %s: %v"), cloneRbdVol.Pool, cloneRbdVol.RbdImageName, cloneRbdVol.Monitors, delErr)
			}
		}
		ready = false

	}

	err = deleteSnapshot(ctx, snap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete snapshot: %v"), err)
		if !cloneFailed {
			err = fmt.Errorf("clone created but failed to delete snapshot due to other failures: %v", err)
		}
		errCleanUp := cleanUpSnapshot(ctx, snap, cloneRbdVol, cr)
		if errCleanUp != nil {
			klog.Errorf(util.Log(ctx, "failed to delete snapshot or image: %s/%s with error: %v"), snap.Pool, snap.RbdSnapName, errCleanUp)
		}
		err = status.Error(codes.Internal, err.Error())
	}

	if cloneFailed {
		err = fmt.Errorf("failed to clone rbd image %s from snapshot %s: %v", cloneRbdVol.RbdImageName, snap.RbdSnapName, err)
	} else {
		err = updateVolWithImageInfo(ctx, cloneRbdVol, cr)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to get rbd image: %s/%s details with error: %v"), cloneRbdVol.Pool, cloneRbdVol.VolName, err)
			delErr := deleteImage(ctx, cloneRbdVol, cr)
			if delErr != nil {
				klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s/%s with error: %v"), rbdVol.Pool, rbdVol.VolName, delErr)
			}
		}
	}
	return ready, err
}

func cleanUpSnapshot(ctx context.Context, rbdSnap *rbdSnapshot, rbdVol *rbdVolume, cr *util.Credentials) error {

	_, err := getSnapInfo(ctx, rbdSnap, cr)
	if err != nil {
		if _, ok := err.(ErrSnapNotFound); !ok {
			return err
		}
	} else {
		err = deleteSnapshot(ctx, rbdSnap, cr)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to delete snapshot: %v"), err)
			return err
		}
	}

	vol, err := getImageInfo(ctx, rbdVol.Monitors, cr, rbdVol.Pool, rbdVol.RbdImageName)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); !ok {
			return err
		}
	} else {
		rbdVol.ImageID = vol.ID
		err = deleteImage(ctx, rbdVol, cr)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s/%s with error: %v"), rbdVol.Pool, rbdVol.VolName, err)
			return err
		}
	}

	return nil
}

func generateVolFromSnap(rbdSnap *rbdSnapshot) *rbdVolume {
	vol := new(rbdVolume)
	vol.ClusterID = rbdSnap.ClusterID
	vol.Monitors = rbdSnap.Monitors
	vol.Pool = rbdSnap.Pool
	vol.RbdImageName = rbdSnap.RbdSnapName
	return vol
}

func generateSnapFromVol(rbdVol *rbdVolume) *rbdSnapshot {
	snap := new(rbdSnapshot)
	snap.ClusterID = rbdVol.ClusterID
	snap.Monitors = rbdVol.Monitors
	snap.Pool = rbdVol.Pool
	snap.RbdImageName = rbdVol.RbdImageName
	snap.RbdSnapName = rbdVol.RbdImageName
	return snap
}
