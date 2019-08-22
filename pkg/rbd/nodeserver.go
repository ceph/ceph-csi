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
// Implementation notes:
// - stagingTargetPath is the directory passed in the request where the volume needs to be staged
//   - We stage the volume into a directory, named after the VolumeID inside stagingTargetPath if
//    it is a file system
//   - We stage the volume into a file, named after the VolumeID inside stagingTargetPath if it is
//    a block volume
// - Order of operation execution: (useful for defer stacking and when Unstaging to ensure steps
//	are done in reverse, this is done in undoStagingTransaction)
//   - Stash image metadata under staging path
//   - Map the image (creates a device)
//   - Create the staging file/directory under staging path
//   - Stage the device (mount the device mapped for image)
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if err := util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	isBlock := req.GetVolumeCapability().GetBlock() != nil
	disableInUseChecks := false
	// MULTI_NODE_MULTI_WRITER is supported by default for Block access type volumes
	if req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
		if isBlock {
			disableInUseChecks = true
		} else {
			klog.Warningf(util.Log(ctx, "MULTI_NODE_MULTI_WRITER currently only supported with volumes of access type `block`, invalid AccessMode for volume: %v"), req.GetVolumeId())
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

	stagingParentPath := req.GetStagingTargetPath()
	stagingTargetPath := stagingParentPath + "/" + req.GetVolumeId()

	idLk := nodeVolumeIDLocker.Lock(volID)
	defer nodeVolumeIDLocker.Unlock(idLk, volID)

	var isNotMnt bool
	// check if stagingPath is already mounted
	isNotMnt, err = mount.IsNotMountPoint(ns.mounter, stagingTargetPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if !isNotMnt {
		klog.Infof(util.Log(ctx, "rbd: volume %s is already mounted to %s, skipping"), req.GetVolumeId(), stagingTargetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	volOptions, err := genVolFromVolumeOptions(ctx, req.GetVolumeContext(), req.GetSecrets(), disableInUseChecks, isLegacyVolume)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	volOptions.RbdImageName = volName

	isMounted := false
	isStagePathCreated := false
	devicePath := ""

	// Stash image details prior to mapping the image (useful during Unstage as it has no
	// voloptions passed to the RPC as per the CSI spec)
	err = stashRBDImageMetadata(volOptions, stagingParentPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err != nil {
			ns.undoStagingTransaction(ctx, stagingParentPath, devicePath, volID, isStagePathCreated, isMounted)
		}
	}()

	// Mapping RBD image
	devicePath, err = attachRBDImage(ctx, volOptions, cr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof(util.Log(ctx, "rbd image: %s/%s was successfully mapped at %s\n"), req.GetVolumeId(), volOptions.Pool, devicePath)

	err = ns.createStageMountPoint(ctx, stagingTargetPath, isBlock)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	isStagePathCreated = true

	// nodeStage Path
	err = ns.mountVolumeToStagePath(ctx, req, stagingTargetPath, devicePath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	isMounted = true

	err = os.Chmod(stagingTargetPath, 0777)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof(util.Log(ctx, "rbd: successfully mounted volume %s to stagingTargetPath %s"), req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) undoStagingTransaction(ctx context.Context, stagingParentPath, devicePath, volID string, isStagePathCreated, isMounted bool) {
	var err error

	stagingTargetPath := stagingParentPath + "/" + volID
	if isMounted {
		err = ns.mounter.Unmount(stagingTargetPath)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to unmount stagingtargetPath: %s with error: %v"), stagingTargetPath, err)
			return
		}
	}

	// remove the file/directory created on staging path
	if isStagePathCreated {
		err = os.Remove(stagingTargetPath)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to remove stagingtargetPath: %s with error: %v"), stagingTargetPath, err)
			// continue on failure to unmap the image, as leaving stale images causes more issues than a stale file/directory
		}
	}

	// Unmapping rbd device
	if devicePath != "" {
		err = detachRBDDevice(ctx, devicePath)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to unmap rbd device: %s for volume %s with error: %v"), devicePath, volID, err)
			// continue on failure to delete the stash file, as kubernetes will fail to delete the staging path otherwise
		}
	}

	// Cleanup the stashed image metadata
	if err = cleanupRBDImageMetadataStash(stagingParentPath); err != nil {
		klog.Errorf(util.Log(ctx, "failed to cleanup image metadata stash (%v)"), err)
		return
	}
}

func (ns *NodeServer) createStageMountPoint(ctx context.Context, mountPath string, isBlock bool) error {
	if isBlock {
		pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0750)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to create mountPath:%s with error: %v"), mountPath, err)
			return status.Error(codes.Internal, err.Error())
		}
		if err = pathFile.Close(); err != nil {
			klog.Errorf(util.Log(ctx, "failed to close mountPath:%s with error: %v"), mountPath, err)
			return status.Error(codes.Internal, err.Error())
		}

		return nil
	}

	err := os.Mkdir(mountPath, 0750)
	if err != nil {
		if !os.IsExist(err) {
			klog.Errorf(util.Log(ctx, "failed to create mountPath:%s with error: %v"), mountPath, err)
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
	stagingPath += "/" + req.GetVolumeId()

	idLk := targetPathLocker.Lock(targetPath)
	defer targetPathLocker.Unlock(idLk, targetPath)

	// Check if that target path exists properly
	notMnt, err := ns.createTargetMountPath(ctx, targetPath, isBlock)
	if err != nil {
		return nil, err
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Publish Path
	err = ns.mountVolume(ctx, stagingPath, req)
	if err != nil {
		return nil, err
	}

	klog.Infof(util.Log(ctx, "rbd: successfully mounted stagingPath %s to targetPath %s"), stagingPath, targetPath)
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

func (ns *NodeServer) mountVolumeToStagePath(ctx context.Context, req *csi.NodeStageVolumeRequest, stagingPath, devicePath string) error {
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
		klog.Errorf(util.Log(ctx, "failed to mount device path (%s) to staging path (%s) for volume (%s) error %s"), devicePath, stagingPath, req.GetVolumeId(), err)
	}
	return err
}

func (ns *NodeServer) mountVolume(ctx context.Context, stagingPath string, req *csi.NodePublishVolumeRequest) error {
	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	targetPath := req.GetTargetPath()
	klog.V(4).Infof(util.Log(ctx, "target %v\nisBlock %v\nfstype %v\nstagingPath %v\nreadonly %v\nmountflags %v\n"),
		targetPath, isBlock, fsType, stagingPath, readOnly, mountFlags)
	mountFlags = append(mountFlags, "bind")
	if readOnly {
		mountFlags = append(mountFlags, "ro")
	}
	if err := util.Mount(stagingPath, targetPath, fsType, mountFlags); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (ns *NodeServer) createTargetMountPath(ctx context.Context, mountPath string, isBlock bool) (bool, error) {
	// Check if that mount path exists properly
	notMnt, err := mount.IsNotMountPoint(ns.mounter, mountPath)
	if err != nil {
		if os.IsNotExist(err) {
			if isBlock {
				// #nosec
				pathFile, e := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0750)
				if e != nil {
					klog.V(4).Infof(util.Log(ctx, "Failed to create mountPath:%s with error: %v"), mountPath, err)
					return notMnt, status.Error(codes.Internal, e.Error())
				}
				if err = pathFile.Close(); err != nil {
					klog.V(4).Infof(util.Log(ctx, "Failed to close mountPath:%s with error: %v"), mountPath, err)
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
			klog.V(4).Infof(util.Log(ctx, "targetPath: %s has already been deleted"), targetPath)
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

	klog.Infof(util.Log(ctx, "rbd: successfully unbound volume %s from %s"), req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnstageVolumeRequest(req); err != nil {
		return nil, err
	}

	stagingParentPath := req.GetStagingTargetPath()
	stagingTargetPath := stagingParentPath + "/" + req.GetVolumeId()

	notMnt, err := mount.IsNotMountPoint(ns.mounter, stagingTargetPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		// Continue on ENOENT errors as we may still have the image mapped
		notMnt = true
	}
	if !notMnt {
		// Unmounting the image
		err = ns.mounter.Unmount(stagingTargetPath)
		if err != nil {
			klog.V(3).Infof(util.Log(ctx, "failed to unmount targetPath: %s with error: %v"), stagingTargetPath, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err = os.Remove(stagingTargetPath); err != nil {
		// Any error is critical as Staging path is expected to be empty by Kubernetes, it otherwise
		// keeps invoking Unstage. Hence any errors removing files within this path is a critical
		// error
		if !os.IsNotExist(err) {
			klog.Errorf(util.Log(ctx, "failed to remove staging target path (%s): (%v)"), stagingTargetPath, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	imgInfo, err := lookupRBDImageMetadataStash(stagingParentPath)
	if err != nil {
		klog.V(2).Infof(util.Log(ctx, "failed to find image metadata: %v"), err)
		// It is an error if it was mounted, as we should have found the image metadata file with
		// no errors
		if !notMnt {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If not mounted, and error is anything other than metadata file missing, it is an error
		if _, ok := err.(ErrMissingStash); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// It was not mounted and image metadata is also missing, we are done as the last step in
		// in the staging transaction is complete
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	// Unmapping rbd device
	imageSpec := imgInfo.Pool + "/" + imgInfo.ImageName
	if err = detachRBDImageOrDeviceSpec(ctx, imageSpec, true, imgInfo.NbdAccess); err != nil {
		klog.Errorf(util.Log(ctx, "error unmapping volume (%s) from staging path (%s): (%v)"), req.GetVolumeId(), stagingTargetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof(util.Log(ctx, "successfully unmounted volume (%s) from staging path (%s)"),
		req.GetVolumeId(), stagingTargetPath)

	if err = cleanupRBDImageMetadataStash(stagingParentPath); err != nil {
		klog.Errorf(util.Log(ctx, "failed to cleanup image metadata stash (%v)"), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
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
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
		},
	}, nil
}
