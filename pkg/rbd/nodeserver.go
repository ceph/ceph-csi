/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/kubernetes/pkg/util/mount"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	targetPathMutex.LockKey(targetPath)
	defer targetPathMutex.UnlockKey(targetPath)

	var volName string
	isBlock := req.GetVolumeCapability().GetBlock() != nil

	if isBlock {
		// Get volName from targetPath
		s := strings.Split(targetPath, "/")
		volName = s[len(s)-1]

		// Check if that target path exists properly
		// targetPath should exists and should be a file
		st, err := os.Stat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, status.Error(codes.NotFound, "targetPath not exist")
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
		if !st.Mode().IsRegular() {
			return nil, status.Error(codes.Internal, "targetPath is not regular file")
		}
	} else {
		// Get volName from targetPath
		if !strings.HasSuffix(targetPath, "/mount") {
			return nil, fmt.Errorf("rnd: malformed the value of target path: %s", targetPath)
		}
		s := strings.Split(strings.TrimSuffix(targetPath, "/mount"), "/")
		volName = s[len(s)-1]

		// Check if that target path exists properly
		notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				if err = os.MkdirAll(targetPath, 0750); err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
				notMnt = true
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		if !notMnt {
			return &csi.NodePublishVolumeResponse{}, nil
		}
	}

	// Mapping RBD image
	volOptions, err := getRBDVolumeOptions(req.VolumeAttributes)
	if err != nil {
		return nil, err
	}
	volOptions.VolName = volName
	devicePath, err := attachRBDImage(volOptions, volOptions.UserId, req.GetNodePublishSecrets())
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("rbd image: %s/%s was successfully mapped at %s\n", req.GetVolumeId(), volOptions.Pool, devicePath)

	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	attrib := req.GetVolumeAttributes()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	glog.V(4).Infof("target %v\nisBlock %v\nfstype %v\ndevice %v\nreadonly %v\nattributes %v\n mountflags %v\n",
		targetPath, isBlock, fsType, devicePath, readOnly, attrib, mountFlags)

	diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: mount.NewOsExec()}
	if isBlock {
		options := []string{"bind"}
		if err := diskMounter.Mount(devicePath, targetPath, fsType, options); err != nil {
			return nil, err
		}
	} else {
		options := []string{}
		if readOnly {
			options = append(options, "ro")
		}

		if err := diskMounter.FormatAndMount(devicePath, targetPath, fsType, options); err != nil {
			return nil, err
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	targetPathMutex.LockKey(targetPath)
	defer targetPathMutex.UnlockKey(targetPath)

	devicePath, cnt, err := mount.GetDeviceNameFromMount(ns.mounter, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("NodeUnpublishVolume: targetPath: %s, devicePath: %s\n", targetPath, devicePath)

	// Unmounting the image
	err = ns.mounter.Unmount(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	cnt--
	if cnt != 0 {
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	// Unmapping rbd device
	if err := detachRBDDevice(devicePath); err != nil {
		glog.V(3).Infof("failed to unmap rbd device: %s with error: %v", devicePath, err)
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest) (
	*csi.NodeStageVolumeResponse, error) {

	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest) (
	*csi.NodeUnstageVolumeResponse, error) {

	return nil, status.Error(codes.Unimplemented, "")
}
