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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func (cs *ControllerServer) createCloneFromImage(ctx context.Context, req *csi.CreateVolumeRequest, cloneRbdVol *rbdVolume, cr *util.Credentials) error {

	var err error

	volumeSource := req.VolumeContentSource.GetVolume()
	if volumeSource == nil {
		return status.Error(codes.InvalidArgument, "volume content source cannot be empty")
	}

	volumeID := volumeSource.GetVolumeId()
	if volumeID == "" {
		return status.Error(codes.InvalidArgument, "volume content source volume ID cannot be empty")
	}
	if acquired := cs.VolumeLocks.TryAcquire(volumeID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volumeID)
		return status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.VolumeLocks.Release(volumeID)
	rbdVol := &rbdVolume{}
	// validate parent volume is present
	err = genVolFromVolID(ctx, rbdVol, volumeID, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get volume details from %s: %v"), volumeID, err)
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to get volume details from %s: %v", volumeID, err))
	}

	klog.V(4).Infof("creating volume %s for request %s from source volume %s", cloneRbdVol.RbdImageName, cloneRbdVol.RequestName, rbdVol.RbdImageName)
	// generate snapshot from parent volume
	parentSnap := generateSnapFromVol(rbdVol)
	// generate temparory rbd volume from request rbd volume
	tempCloneVol := generatTempVol(cloneRbdVol)
	tempCloneVol.ImageFormat = rbdImageFormat2
	tempCloneVol.ImageFeatures = "layering,deep-flatten"

	err = createRBDClone(ctx, tempCloneVol, parentSnap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create temparory clone image %q: %v"), tempCloneVol.RbdImageName, err)
		return status.Error(codes.Internal, err.Error())
	}
	cloneSnap := generateSnapFromVol(tempCloneVol)
	requestCloneCreated := false
	err = createRBDClone(ctx, cloneRbdVol, cloneSnap, cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to clone image %q: %v"), cloneRbdVol.RbdImageName, err)
		err = status.Error(codes.Internal, err.Error())
	} else {
		requestCloneCreated = true
	}
	// delete temparory cloned image
	errDel := deleteImage(ctx, tempCloneVol, cr)
	if errDel != nil {
		// delete requested cloned volume if temparory image deletion fails
		klog.Errorf(util.Log(ctx, "failed to delete temparory clone image %q: %v"), tempCloneVol.RbdImageName, errDel)
		if requestCloneCreated {
			errDel = deleteImage(ctx, cloneRbdVol, cr)
			if errDel != nil {
				// delete requested cloned volume if temparory image deletion fails
				klog.Errorf(util.Log(ctx, "failed to delete requested clone image %q: %v"), cloneRbdVol.RbdImageName, errDel)
			}
		}
		return status.Error(codes.Internal, errDel.Error())
	}

	return err
}

func generatTempVol(rbdVol *rbdVolume) *rbdVolume {
	vol := new(rbdVolume)
	*vol = *rbdVol
	vol.RbdImageName = fmt.Sprintf("%s-1", rbdVol.RbdImageName)
	return vol
}
