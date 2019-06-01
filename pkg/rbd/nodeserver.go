/*
Copyright 2018 The Ceph-CSI Authors.

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

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
)

// NodeServer struct of ceph rbd driver with supported methods of CSI
// node server spec
type NodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
}

// NodePublishVolume mounts the volume mounted to the device path to the target
// path
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "empty target path in request")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "empty volume capability in request")
	}

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	cr, err := util.GetUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	targetPathMutex.LockKey(targetPath)
	defer func() {
		if err = targetPathMutex.UnlockKey(targetPath); err != nil {
			klog.Warningf("failed to unlock mutex targetpath:%s %v", targetPath, err)
		}
	}()
	disableInUseChecks := false

	volName, err := ns.getVolumeName(req)
	if err != nil {
		return nil, err
	}

	isBlock := req.GetVolumeCapability().GetBlock() != nil
	// Check if that target path exists properly
	notMnt, err := ns.createTargetPath(targetPath, isBlock)
	if err != nil {
		return nil, err
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// MULTI_NODE_MULTI_WRITER is supported by default for Block access type volumes
	if req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
		if isBlock {
			disableInUseChecks = true
		} else {
			klog.Warningf("MULTI_NODE_MULTI_WRITER currently only supported with volumes of access type `block`, invalid AccessMode for volume: %v", req.GetVolumeId())
			return nil, status.Error(codes.InvalidArgument, "rbd: RWX access mode request is only valid for volumes with access type `block`")
		}
	}

	volOptions, err := genVolFromVolumeOptions(req.GetVolumeContext(), disableInUseChecks)
	if err != nil {
		return nil, err
	}
	volOptions.RbdImageName = volName
	// Mapping RBD image
	devicePath, err := attachRBDImage(volOptions, cr)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("rbd image: %s/%s was successfully mapped at %s\n", req.GetVolumeId(), volOptions.Pool, devicePath)

	// Publish Path
	err = ns.mountVolume(req, devicePath)
	if err != nil {
		return nil, err
	}
	err = os.Chmod(targetPath, 0777)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) getVolumeName(req *csi.NodePublishVolumeRequest) (string, error) {
	var vi util.CSIIdentifier

	err := vi.DecomposeCSIID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("error decoding volume ID (%s) (%s)", err, req.GetVolumeId())
		return "", status.Error(codes.InvalidArgument, err.Error())
	}

	return volJournal.NamingPrefix() + vi.ObjectUUID, nil
}

func (ns *NodeServer) mountVolume(req *csi.NodePublishVolumeRequest, devicePath string) error {
	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	attrib := req.GetVolumeContext()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	targetPath := req.GetTargetPath()

	klog.V(4).Infof("target %v\nisBlock %v\nfstype %v\ndevice %v\nreadonly %v\nattributes %v\n mountflags %v\n",
		targetPath, isBlock, fsType, devicePath, readOnly, attrib, mountFlags)

	diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: mount.NewOsExec()}
	if isBlock {
		options := []string{"bind"}
		if err := diskMounter.Mount(devicePath, targetPath, fsType, options); err != nil {
			return err
		}
	} else {
		options := []string{}
		if readOnly {
			options = append(options, "ro")
		}

		if err := diskMounter.FormatAndMount(devicePath, targetPath, fsType, options); err != nil {
			return err
		}
	}
	return nil
}

func (ns *NodeServer) createTargetPath(targetPath string, isBlock bool) (bool, error) {
	// Check if that target path exists properly
	notMnt, err := ns.mounter.IsNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if isBlock {
				// create an empty file
				// #nosec
				targetPathFile, e := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, 0750)
				if e != nil {
					klog.V(4).Infof("Failed to create targetPath:%s with error: %v", targetPath, err)
					return notMnt, status.Error(codes.Internal, e.Error())
				}
				if err = targetPathFile.Close(); err != nil {
					klog.V(4).Infof("Failed to close targetPath:%s with error: %v", targetPath, err)
					return notMnt, status.Error(codes.Internal, err.Error())
				}
			} else {
				// Create a directory
				if err = os.MkdirAll(targetPath, 0750); err != nil {
					return notMnt, status.Error(codes.Internal, err.Error())
				}
			}
			notMnt = true
		} else {
			return false, status.Error(codes.Internal, err.Error())
		}
	}
	return notMnt, err

}

// NodeUnpublishVolume unmounts the volume from the target path
func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "empty target path in request")
	}

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	targetPathMutex.LockKey(targetPath)

	defer func() {
		if err := targetPathMutex.UnlockKey(targetPath); err != nil {
			klog.Warningf("failed to unlock mutex targetpath:%s %v", targetPath, err)
		}
	}()

	notMnt, err := ns.mounter.IsNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// targetPath has already been deleted
			klog.V(4).Infof("targetPath: %s has already been deleted", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if notMnt {
		// TODO should consider deleting path instead of returning error,
		// once all codes become ready for csi 1.0.
		return nil, status.Error(codes.NotFound, "volume not mounted")
	}

	devicePath, cnt, err := mount.GetDeviceNameFromMount(ns.mounter, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = ns.unmount(targetPath, devicePath, cnt); err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeServer) unmount(targetPath, devicePath string, cnt int) error {
	var err error
	// Bind mounted device needs to be resolved by using resolveBindMountedBlockDevice
	if devicePath == "devtmpfs" {
		devicePath, err = resolveBindMountedBlockDevice(targetPath)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		klog.V(4).Infof("NodeUnpublishVolume: devicePath: %s, (original)cnt: %d\n", devicePath, cnt)
		// cnt for GetDeviceNameFromMount is broken for bind mouted device,
		// it counts total number of mounted "devtmpfs", instead of counting this device.
		// So, forcibly setting cnt to 1 here.
		// TODO : fix this properly
		cnt = 1
	}

	klog.V(4).Infof("NodeUnpublishVolume: targetPath: %s, devicePath: %s\n", targetPath, devicePath)

	// Unmounting the image
	err = ns.mounter.Unmount(targetPath)
	if err != nil {
		klog.V(3).Infof("failed to unmount targetPath: %s with error: %v", targetPath, err)
		return status.Error(codes.Internal, err.Error())
	}

	cnt--
	if cnt != 0 {
		// TODO should this be fixed not to success, so that driver can retry unmounting?
		return nil
	}

	// Unmapping rbd device
	if err = detachRBDDevice(devicePath); err != nil {
		klog.V(3).Infof("failed to unmap rbd device: %s with error: %v", devicePath, err)
		return err
	}

	// Remove targetPath
	if err = os.RemoveAll(targetPath); err != nil {
		klog.V(3).Infof("failed to remove targetPath: %s with error: %v", targetPath, err)
	}
	return err
}
func resolveBindMountedBlockDevice(mountPath string) (string, error) {
	// #nosec
	cmd := exec.Command("findmnt", "-n", "-o", "SOURCE", "--first-only", "--target", mountPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		klog.V(2).Infof("Failed findmnt command for path %s: %s %v", mountPath, out, err)
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
	// nolint
	reBlk := regexp.MustCompile("^devtmpfs\\[(/[^/]+(?:/[^/]*)*)\\]$")
	if match := reBlk.FindStringSubmatch(out); match != nil {
		return fmt.Sprintf("/dev%s", match[1]), nil
	}
	return "", fmt.Errorf("parseFindMntResolveSource: %s doesn't match to any expected findMnt output", out)
}
