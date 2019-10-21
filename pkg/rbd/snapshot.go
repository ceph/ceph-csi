package rbd

import (
	"context"
	"fmt"

	"github.com/ceph/ceph-csi/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func createRBDClone(ctx context.Context, cloneRbdVol *rbdVolume, snap *rbdSnapshot, cr *util.Credentials) error {

	// update snapshot name as cloned volume name as it will be always unique
	snap.RbdSnapName = cloneRbdVol.RbdImageName

	// check backend image is present as we are reserving single omap key for
	// both snapshot and cloned rbd image
	_, err := getImageInfo(ctx, cloneRbdVol.Monitors, cr, cloneRbdVol.Pool, cloneRbdVol.RbdImageName)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); !ok {
			return status.Errorf(codes.Internal, "failed to get image details for %s", cloneRbdVol.RbdImageName)
		}
	} else {
		// cloned volume already present return success to create snapshot from it
		return nil
	}

	// check snapshot is present in parent volume
	_, err = getSnapInfo(ctx, snap.Monitors, cr, snap.Pool,
		snap.RbdImageName, snap.RbdSnapName)
	if err != nil {
		if _, ok := err.(ErrSnapNotFound); !ok {
			return status.Error(codes.Internal, err.Error())
		}
		// create snapshot
		err = createSnapshot(ctx, snap, cr)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to create snapshot: %v"), err)
			return status.Error(codes.Internal, err.Error())

		}
	}
	cloneFailed := false
	// create clone image and delete snapshot
	err = cloneRbdImageFromSnapshot(ctx, cloneRbdVol, snap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to clone rbd image %s from snapshot %s: %v"), cloneRbdVol.RbdImageName, snap.RbdSnapName, err)
		cloneFailed = true
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
	}
	return err
}

func cleanUpSnapshot(ctx context.Context, rbdSnap *rbdSnapshot, rbdVol *rbdVolume, cr *util.Credentials) error {
	err := deleteSnapshot(ctx, rbdSnap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete snapshot: %v"), err)
		return err
	}
	err = deleteImage(ctx, rbdVol, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s/%s with error: %v"), rbdVol.Pool, rbdVol.VolName, err)
	}
	return err
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
