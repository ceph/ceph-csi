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
	"context"
	"fmt"
	"os"
	"strings"

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/resizefs"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

// NodeServer struct of ceph rbd driver with supported methods of CSI
// node server spec
type NodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	VolumeLocks *util.VolumeLocks
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

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	stagingParentPath := req.GetStagingTargetPath()
	stagingTargetPath := stagingParentPath + "/" + volID

	isLegacyVolume, volName, err := getVolumeNameByID(volID, stagingParentPath)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

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
	volOptions.VolID = req.GetVolumeId()

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
	klog.V(4).Infof(util.Log(ctx, "rbd image: %s/%s was successfully mapped at %s\n"),
		req.GetVolumeId(), volOptions.Pool, devicePath)

	if volOptions.Encrypted {
		devicePath, err = ns.processEncryptedDevice(ctx, volOptions, devicePath, cr, req.GetSecrets())
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

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

	// #nosec - allow anyone to write inside the target path
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
		err = detachRBDDevice(ctx, devicePath, volID)
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
		pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0600)
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
	volID := req.GetVolumeId()
	stagingPath += "/" + volID

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

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
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: utilexec.New()}
	// rbd images are thin-provisioned and return zeros for unwritten areas.  A freshly created
	// image will not benefit from discard and we also want to avoid as much unnecessary zeroing
	// as possible.  Open-code mkfs here because FormatAndMount() doesn't accept custom mkfs
	// options.
	//
	// Note that "freshly" is very important here.  While discard is more of a nice to have,
	// lazy_journal_init=1 is plain unsafe if the image has been written to before and hasn't
	// been zeroed afterwards (unlike the name suggests, it leaves the journal completely
	// uninitialized and carries a risk until the journal is overwritten and wraps around for
	// the first time).
	existingFormat, err := diskMounter.GetDiskFormat(devicePath)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get disk format for path %s, error: %v"), devicePath, err)
		return err
	}
	// TODO: update this when adding support for static (pre-provisioned) PVs
	if existingFormat == "" /* && !staticVol */ {
		args := []string{}
		if fsType == "ext4" {
			args = []string{"-m0", "-Enodiscard,lazy_itable_init=1,lazy_journal_init=1", devicePath}
		} else if fsType == "xfs" {
			args = []string{"-K", devicePath}
		}
		if len(args) > 0 {
			cmdOut, cmdErr := diskMounter.Exec.Command("mkfs."+fsType, args...).CombinedOutput()
			if cmdErr != nil {
				klog.Errorf(util.Log(ctx, "failed to run mkfs error: %v, output: %v"), cmdErr, cmdOut)
				return cmdErr
			}
		}
	}

	opt := []string{"_netdev"}
	isBlock := req.GetVolumeCapability().GetBlock() != nil

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
	mountOptions := []string{"bind", "_netdev"}
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	targetPath := req.GetTargetPath()

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	klog.V(4).Infof(util.Log(ctx, "target %v\nisBlock %v\nfstype %v\nstagingPath %v\nreadonly %v\nmountflags %v\n"),
		targetPath, isBlock, fsType, stagingPath, readOnly, mountOptions)

	if readOnly {
		mountOptions = append(mountOptions, "ro")
	}
	if err := util.Mount(stagingPath, targetPath, fsType, mountOptions); err != nil {
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
	volID := req.GetVolumeId()

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

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

	volID := req.GetVolumeId()

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

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
	if err = detachRBDImageOrDeviceSpec(ctx, imageSpec, true, imgInfo.NbdAccess, req.GetVolumeId()); err != nil {
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

func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path must be provided")
	}

	if acquired := ns.VolumeLocks.TryAcquire(volumeID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volumeID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer ns.VolumeLocks.Release(volumeID)

	volName, err := getVolumeName(volumeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	// volumePath is targetPath for block PVC and stagingPath for filesystem.
	// check the path is mountpoint or not, if it is
	// mountpoint treat this as block PVC or else it is filesystem PVC
	// TODO remove this once ceph-csi supports CSI v1.2.0 spec
	notMnt, err := mount.IsNotMountPoint(ns.mounter, volumePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !notMnt {
		return &csi.NodeExpandVolumeResponse{}, nil
	}
	imgInfo, devicePath, err := getDevicePathAndImageInfo(ctx, volumePath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if volName != imgInfo.ImageName {
		return nil, status.Errorf(codes.InvalidArgument, "volume name missmatch between request (%s) and stored metadata (%s)", volName, imgInfo.ImageName)
	}
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: utilexec.New()}
	// TODO check size and return success or error
	volumePath += "/" + volumeID
	resizer := resizefs.NewResizeFs(diskMounter)
	ok, err := resizer.Resize(devicePath, volumePath)
	if !ok {
		return nil, fmt.Errorf("rbd: resize failed on path %s, error: %v", req.GetVolumePath(), err)
	}
	return &csi.NodeExpandVolumeResponse{}, nil
}

func getDevicePathAndImageInfo(ctx context.Context, volumePath string) (rbdImageMetadataStash, string, error) {
	imgInfo, err := lookupRBDImageMetadataStash(volumePath)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to find image metadata: %v"), err)
	}
	device, found := findDeviceMappingImage(ctx, imgInfo.Pool, imgInfo.ImageName, imgInfo.NbdAccess)
	if found {
		return imgInfo, device, nil
	}
	return rbdImageMetadataStash{}, "", fmt.Errorf("failed to get device for stagingtarget path %v", volumePath)
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
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (ns *NodeServer) processEncryptedDevice(ctx context.Context, volOptions *rbdVolume, devicePath string, cr *util.Credentials, secrets map[string]string) (string, error) {
	imageSpec := volOptions.Pool + "/" + volOptions.RbdImageName
	encrypted, err := util.CheckRbdImageEncrypted(ctx, cr, volOptions.Monitors, imageSpec)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get encryption status for rbd image %s: %v"),
			imageSpec, err)
		return "", err
	}

	if encrypted == rbdImageRequiresEncryption {
		diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: utilexec.New()}
		// TODO: update this when adding support for static (pre-provisioned) PVs
		var existingFormat string
		existingFormat, err = diskMounter.GetDiskFormat(devicePath)
		if err != nil {
			return "", fmt.Errorf("failed to get disk format for path %s, error: %v", devicePath, err)
		}
		if existingFormat != "" {
			return "", fmt.Errorf("can not encrypt rbdImage %s that already has file system: %s",
				imageSpec, existingFormat)
		}
		err = encryptDevice(ctx, volOptions, secrets, cr, devicePath)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt rbd image %s: %v", imageSpec, err)
		}
	} else if encrypted != rbdImageEncrypted {
		return "", fmt.Errorf("rbd image %s found mounted with unexpected encryption status %s",
			imageSpec, encrypted)
	}

	devicePath, err = openEncryptedDevice(ctx, volOptions, devicePath, secrets)
	if err != nil {
		return "", err
	}

	return devicePath, nil
}

func encryptDevice(ctx context.Context, rbdVol *rbdVolume, secret map[string]string, cr *util.Credentials, devicePath string) error {
	passphrase, err := util.GetCryptoPassphrase(secret)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get crypto passphrase for %s/%s: %v"),
			rbdVol.Pool, rbdVol.RbdImageName, err)
		return err
	}

	if err = util.EncryptVolume(ctx, devicePath, passphrase); err != nil {
		err = fmt.Errorf("failed to encrypt volume %s/%s: %v", rbdVol.Pool, rbdVol.RbdImageName, err)
		klog.Errorf(util.Log(ctx, err.Error()))
		return err
	}

	imageSpec := rbdVol.Pool + "/" + rbdVol.RbdImageName
	err = util.SaveRbdImageEncryptionStatus(ctx, cr, rbdVol.Monitors, imageSpec, rbdImageEncrypted)

	return err
}

func openEncryptedDevice(ctx context.Context, volOptions *rbdVolume, devicePath string, secrets map[string]string) (string, error) {
	passphrase, err := util.GetCryptoPassphrase(secrets)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get passphrase for encrypted device %s/%s: %v"),
			volOptions.Pool, volOptions.RbdImageName, err)
		return "", status.Error(codes.Internal, err.Error())
	}

	mapperFile, mapperFilePath := util.VolumeMapper(volOptions.VolID)

	isOpen, err := util.IsDeviceOpen(ctx, mapperFilePath)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to check device %s encryption status: %s"), devicePath, err)
		return devicePath, err
	}
	if isOpen {
		klog.V(4).Infof(util.Log(ctx, "encrypted device is already open at %s"), mapperFilePath)
	} else {
		err = util.OpenEncryptedVolume(ctx, devicePath, mapperFile, passphrase)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to open device %s/%s: %v"),
				volOptions.Pool, volOptions.RbdImageName, err)
			return devicePath, err
		}
	}

	return mapperFilePath, nil
}

func getVolumeNameByID(volID, stagingTargetPath string) (bool, string, error) {
	volName, err := getVolumeName(volID)
	if err != nil {
		// error ErrInvalidVolID may mean this is an 1.0.0 version volume, check for name
		// pattern match in addition to error to ensure this is a likely v1.0.0 volume
		if _, ok := err.(ErrInvalidVolID); !ok || !isLegacyVolumeID(volID) {
			return false, "", status.Error(codes.InvalidArgument, err.Error())
		}

		volName, err = getLegacyVolumeName(stagingTargetPath)
		if err != nil {
			return false, "", status.Error(codes.InvalidArgument, err.Error())
		}

		return true, volName, nil
	}

	return false, volName, nil
}
