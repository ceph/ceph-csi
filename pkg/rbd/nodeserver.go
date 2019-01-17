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
	"os/exec"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
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
	} else {
		// Get volName from targetPath
		if !strings.HasSuffix(targetPath, "/mount") {
			return nil, fmt.Errorf("rbd: malformed the value of target path: %s", targetPath)
		}
		s := strings.Split(strings.TrimSuffix(targetPath, "/mount"), "/")
		volName = s[len(s)-1]
	}

	// Check if that target path exists properly
	notMnt, err := ns.mounter.IsNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if isBlock {
				// create an empty file
				targetPathFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, 0750)
				if err != nil {
					glog.V(4).Infof("Failed to create targetPath:%s with error: %v", targetPath, err)
					return nil, status.Error(codes.Internal, err.Error())
				}
				if err := targetPathFile.Close(); err != nil {
					glog.V(4).Infof("Failed to close targetPath:%s with error: %v", targetPath, err)
					return nil, status.Error(codes.Internal, err.Error())
				}
			} else {
				// Create a directory
				if err = os.MkdirAll(targetPath, 0750); err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	volOptions, err := getRBDVolumeOptions(req.GetVolumeContext())
	if err != nil {
		return nil, err
	}
	volOptions.VolName = volName
	// Mapping RBD image
	devicePath, err := attachRBDImage(volOptions, volOptions.UserId, req.GetSecrets())
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("rbd image: %s/%s was successfully mapped at %s\n", req.GetVolumeId(), volOptions.Pool, devicePath)

	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	attrib := req.GetVolumeContext()
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

	notMnt, err := ns.mounter.IsNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// targetPath has already been deleted
			glog.V(4).Infof("targetPath: %s has already been deleted", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if notMnt {
		// TODO should consider deleting path instead of returning error,
		// once all codes become ready for csi 1.0.
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	devicePath, cnt, err := mount.GetDeviceNameFromMount(ns.mounter, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Bind mounted device needs to be resolved by using resolveBindMountedBlockDevice
	if devicePath == "devtmpfs" {
		var err error
		devicePath, err = resolveBindMountedBlockDevice(targetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		glog.V(4).Infof("NodeUnpublishVolume: devicePath: %s, (original)cnt: %d\n", devicePath, cnt)
		// cnt for GetDeviceNameFromMount is broken for bind mouted device,
		// it counts total number of mounted "devtmpfs", instead of counting this device.
		// So, forcibly setting cnt to 1 here.
		// TODO : fix this properly
		cnt = 1
	}

	glog.V(4).Infof("NodeUnpublishVolume: targetPath: %s, devicePath: %s\n", targetPath, devicePath)

	// Unmounting the image
	err = ns.mounter.Unmount(targetPath)
	if err != nil {
		glog.V(3).Infof("failed to unmount targetPath: %s with error: %v", targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	cnt--
	if cnt != 0 {
		// TODO should this be fixed not to success, so that driver can retry unmounting?
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	// Unmapping rbd device
	if err := detachRBDDevice(devicePath); err != nil {
		glog.V(3).Infof("failed to unmap rbd device: %s with error: %v", devicePath, err)
		return nil, err
	}

	// Remove targetPath
	if err := os.RemoveAll(targetPath); err != nil {
		glog.V(3).Infof("failed to remove targetPath: %s with error: %v", targetPath, err)
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

func resolveBindMountedBlockDevice(mountPath string) (string, error) {
	cmd := exec.Command("findmnt", "-n", "-o", "SOURCE", "--first-only", "--target", mountPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		glog.V(2).Infof("Failed findmnt command for path %s: %s %v", mountPath, out, err)
		return "", err
	}
	return parseFindMntResolveSource(string(out))
}

// parse output of "findmnt -o SOURCE --first-only --target" and return just the SOURCE
func parseFindMntResolveSource(out string) (string, error) {
	// cut trailing newline
	out = strings.TrimSuffix(out, "\n")
	// Check if out is a mounted device
	reMnt := regexp.MustCompile("^(/[^/]+(?:/[^/]*)*)$")
	if match := reMnt.FindStringSubmatch(out); match != nil {
		return match[1], nil
	}
	// Check if out is a block device
	reBlk := regexp.MustCompile("^devtmpfs\\[(/[^/]+(?:/[^/]*)*)\\]$")
	if match := reBlk.FindStringSubmatch(out); match != nil {
		return fmt.Sprintf("/dev%s", match[1]), nil
	}
	return "", fmt.Errorf("parseFindMntResolveSource: %s doesn't match to any expected findMnt output", out)
}
