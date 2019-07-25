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

// NodeStageVolume mounts the volume to a staging path on the node.
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if err := util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	stagingTargetPath := req.GetStagingTargetPath()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	disableInUseChecks := false
	// MULTI_NODE_MULTI_WRITER is supported by default for Block access type volumes
	if req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
		if isBlock {
			disableInUseChecks = true
		} else {
			klog.Warningf("MULTI_NODE_MULTI_WRITER currently only supported with volumes of access type `block`, invalid AccessMode for volume: %v", req.GetVolumeId())
			return nil, status.Error(codes.InvalidArgument, "rbd: RWX access mode request is only valid for volumes with access type `block`")
		}
	}

	volID := req.GetVolumeId()

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	isLegacyVolume := false
	volName, err := getVolumeName(req.GetVolumeId())
	if err != nil {
		// error ErrInvalidVolID may mean this is an 1.0.0 version volume, check for name
		// pattern match in addition to error to ensure this is a likely v1.0.0 volume
		if _, ok := err.(ErrInvalidVolID); !ok || !isLegacyVolumeID(req.GetVolumeId()) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		volName, err = getLegacyVolumeName(req.GetStagingTargetPath())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		isLegacyVolume = true
	}

	if isBlock {
		stagingTargetPath += "/" + volID
	}

	idLk := nodeVolumeIDLocker.Lock(volID)
	defer nodeVolumeIDLocker.Unlock(idLk, volID)

	var isNotMnt bool
	// check if stagingPath is already mounted
	isNotMnt, err = mount.IsNotMountPoint(ns.mounter, stagingTargetPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if !isNotMnt {
		klog.Infof("rbd: volume %s is already mounted to %s, skipping", req.GetVolumeId(), stagingTargetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	volOptions, err := genVolFromVolumeOptions(req.GetVolumeContext(), req.GetSecrets(), disableInUseChecks, isLegacyVolume)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	volOptions.RbdImageName = volName

	// Mapping RBD image
	devicePath, err := attachRBDImage(volOptions, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof("rbd image: %s/%s was successfully mapped at %s\n", req.GetVolumeId(), volOptions.Pool, devicePath)

	isMounted := false
	isStagePathCreated := false
	// if mounting to stagingpath fails unmap the rbd device. this wont leave any
	// stale rbd device if unstage is not called
	defer func() {
		if err != nil {
			ns.cleanupStagingPath(stagingTargetPath, devicePath, volID, isStagePathCreated, isBlock, isMounted)
		}
	}()
	err = ns.createStageMountPoint(stagingTargetPath, isBlock)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	isStagePathCreated = true

	// nodeStage Path
	err = ns.mountVolumeToStagePath(req, stagingTargetPath, devicePath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	isMounted = true

	err = os.Chmod(stagingTargetPath, 0777)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof("rbd: successfully mounted volume %s to stagingTargetPath %s", req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) cleanupStagingPath(stagingTargetPath, devicePath, volID string, isStagePathCreated, isBlock, isMounted bool) {
	var err error
	if isMounted {
		err = ns.mounter.Unmount(stagingTargetPath)
		if err != nil {
			klog.Errorf("failed to unmount stagingtargetPath: %s with error: %v", stagingTargetPath, err)
		}
	}
	// remove the block file created on staging path
	if isBlock && isStagePathCreated {
		err = os.Remove(stagingTargetPath)
		if err != nil {
			klog.Errorf("failed to remove stagingtargetPath: %s with error: %v", stagingTargetPath, err)
		}
	}
	// Unmapping rbd device
	if err = detachRBDDevice(devicePath); err != nil {
		klog.Errorf("failed to unmap rbd device: %s for volume %s with error: %v", devicePath, volID, err)
	}
}

func (ns *NodeServer) createStageMountPoint(mountPath string, isBlock bool) error {
	if isBlock {
		pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0750)
		if err != nil {
			klog.Errorf("failed to create mountPath:%s with error: %v", mountPath, err)
			return status.Error(codes.Internal, err.Error())
		}
		if err = pathFile.Close(); err != nil {
			klog.Errorf("failed to close mountPath:%s with error: %v", mountPath, err)
			return status.Error(codes.Internal, err.Error())
		}
	}
	return nil
}

// NodePublishVolume mounts the volume mounted to the device path to the target
// path
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	err := util.ValidateNodePublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}
	targetPath := req.GetTargetPath()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	stagingPath := req.GetStagingTargetPath()
	if isBlock {
		stagingPath += "/" + req.GetVolumeId()
	}

	idLk := targetPathLocker.Lock(targetPath)
	defer targetPathLocker.Unlock(idLk, targetPath)

	// Check if that target path exists properly
	notMnt, err := ns.createTargetMountPath(targetPath, isBlock)
	if err != nil {
		return nil, err
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Publish Path
	err = ns.mountVolume(stagingPath, req)
	if err != nil {
		return nil, err
	}

	klog.Infof("rbd: successfully mounted stagingPath %s to targetPath %s", stagingPath, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func getVolumeName(volID string) (string, error) {
	var vi util.CSIIdentifier

	err := vi.DecomposeCSIID(volID)
	if err != nil {
		err = fmt.Errorf("error decoding volume ID (%s) (%s)", err, volID)
		return "", ErrInvalidVolID{err}
	}

	return volJournal.NamingPrefix() + vi.ObjectUUID, nil
}

func getLegacyVolumeName(mountPath string) (string, error) {
	var volName string

	if strings.HasSuffix(mountPath, "/globalmount") {
		s := strings.Split(strings.TrimSuffix(mountPath, "/globalmount"), "/")
		volName = s[len(s)-1]
		return volName, nil
	}

	if strings.HasSuffix(mountPath, "/mount") {
		s := strings.Split(strings.TrimSuffix(mountPath, "/mount"), "/")
		volName = s[len(s)-1]
		return volName, nil
	}

	// get volume name for block volume
	s := strings.Split(mountPath, "/")
	if len(s) == 0 {
		return "", fmt.Errorf("rbd: malformed value of stage target path: %s", mountPath)
	}
	volName = s[len(s)-1]
	return volName, nil
}

func (ns *NodeServer) mountVolumeToStagePath(req *csi.NodeStageVolumeRequest, stagingPath, devicePath string) error {
	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: mount.NewOsExec()}
	opt := []string{}
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	var err error

	if isBlock {
		opt = append(opt, "bind")
		err = diskMounter.Mount(devicePath, stagingPath, fsType, opt)
	} else {
		err = diskMounter.FormatAndMount(devicePath, stagingPath, fsType, opt)
	}
	if err != nil {
		klog.Errorf("failed to mount device path (%s) to staging path (%s) for volume (%s) error %s", devicePath, stagingPath, req.GetVolumeId(), err)
	}
	return err
}

func (ns *NodeServer) mountVolume(stagingPath string, req *csi.NodePublishVolumeRequest) error {
	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	targetPath := req.GetTargetPath()
	klog.V(4).Infof("target %v\nisBlock %v\nfstype %v\nstagingPath %v\nreadonly %v\nmountflags %v\n",
		targetPath, isBlock, fsType, stagingPath, readOnly, mountFlags)
	mountFlags = append(mountFlags, "bind")
	if readOnly {
		mountFlags = append(mountFlags, "ro")
	}
	if isBlock {
		if err := util.Mount(stagingPath, targetPath, fsType, mountFlags); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	} else {
		if err := util.Mount(stagingPath, targetPath, "", mountFlags); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
	return nil
}

func (ns *NodeServer) createTargetMountPath(mountPath string, isBlock bool) (bool, error) {
	// Check if that mount path exists properly
	notMnt, err := mount.IsNotMountPoint(ns.mounter, mountPath)
	if err != nil {
		if os.IsNotExist(err) {
			if isBlock {
				// #nosec
				pathFile, e := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0750)
				if e != nil {
					klog.V(4).Infof("Failed to create mountPath:%s with error: %v", mountPath, err)
					return notMnt, status.Error(codes.Internal, e.Error())
				}
				if err = pathFile.Close(); err != nil {
					klog.V(4).Infof("Failed to close mountPath:%s with error: %v", mountPath, err)
					return notMnt, status.Error(codes.Internal, err.Error())
				}
			} else {
				// Create a directory
				if err = util.CreateMountPoint(mountPath); err != nil {
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
	err := util.ValidateNodeUnpublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	notMnt, err := mount.IsNotMountPoint(ns.mounter, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// targetPath has already been deleted
			klog.V(4).Infof("targetPath: %s has already been deleted", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if notMnt {
		if err = os.RemoveAll(targetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err = ns.mounter.Unmount(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = os.RemoveAll(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof("rbd: successfully unbinded volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnstageVolumeRequest(req); err != nil {
		return nil, err
	}

	stagingTargetPath := req.GetStagingTargetPath()

	// kind of hack to unmount block volumes
	blockStagingPath := stagingTargetPath + "/" + req.GetVolumeId()
unmount:
	notMnt, err := mount.IsNotMountPoint(ns.mounter, stagingTargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// staging targetPath has already been deleted
			klog.V(4).Infof("stagingTargetPath: %s has already been deleted", stagingTargetPath)
			return &csi.NodeUnstageVolumeResponse{}, nil
		}
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if notMnt {
		_, err = os.Stat(blockStagingPath)
		if err == nil && (stagingTargetPath != blockStagingPath) {
			stagingTargetPath = blockStagingPath
			goto unmount
		}
		if stagingTargetPath == blockStagingPath {
			if err = os.Remove(stagingTargetPath); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		return &csi.NodeUnstageVolumeResponse{}, nil
	}
	// Unmount the volume
	devicePath, cnt, err := mount.GetDeviceNameFromMount(ns.mounter, stagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = ns.unmount(stagingTargetPath, devicePath, cnt); err != nil {
		return nil, err
	}

	if stagingTargetPath == blockStagingPath {
		if err = os.Remove(stagingTargetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	klog.Infof("rbd: successfully unmounted volume %s from %s", req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
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
		return status.Error(codes.Internal, err.Error())
	}

	// Remove targetPath
	if err = os.RemoveAll(targetPath); err != nil {
		klog.V(3).Infof("failed to remove targetPath: %s with error: %v", targetPath, err)
		return status.Error(codes.Internal, err.Error())
	}
	return nil
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

// NodeGetCapabilities returns the supported capabilities of the node server
func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}
