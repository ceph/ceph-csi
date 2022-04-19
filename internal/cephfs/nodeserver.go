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

package cephfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeServer struct of ceph CSI driver with supported methods of CSI
// node server spec.
type NodeServer struct {
	*csicommon.DefaultNodeServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	VolumeLocks *util.VolumeLocks
}

func getCredentialsForVolume(
	volOptions *store.VolumeOptions,
	secrets map[string]string) (*util.Credentials, error) {
	var (
		err error
		cr  *util.Credentials
	)

	if volOptions.ProvisionVolume {
		// The volume is provisioned dynamically, use passed in admin credentials

		cr, err = util.NewAdminCredentials(secrets)
		if err != nil {
			return nil, fmt.Errorf("failed to get admin credentials from node stage secrets: %w", err)
		}
	} else {
		// The volume is pre-made, credentials are in node stage secrets

		cr, err = util.NewUserCredentials(secrets)
		if err != nil {
			return nil, fmt.Errorf("failed to get user credentials from node stage secrets: %w", err)
		}
	}

	return cr, nil
}

func (ns *NodeServer) getVolumeOptions(
	ctx context.Context,
	volID fsutil.VolumeID,
	volContext,
	volSecrets map[string]string,
) (*store.VolumeOptions, error) {
	volOptions, _, err := store.NewVolumeOptionsFromVolID(ctx, string(volID), volContext, volSecrets)
	if err != nil {
		if !errors.Is(err, cerrors.ErrInvalidVolID) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		volOptions, _, err = store.NewVolumeOptionsFromStaticVolume(string(volID), volContext)
		if err != nil {
			if !errors.Is(err, cerrors.ErrNonStaticVolume) {
				return nil, status.Error(codes.Internal, err.Error())
			}

			volOptions, _, err = store.NewVolumeOptionsFromMonitorList(string(volID), volContext, volSecrets)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return volOptions, nil
}

// NodeStageVolume mounts the volume to a staging path on the node.
func (ns *NodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if err := util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	// Configuration

	stagingTargetPath := req.GetStagingTargetPath()
	volID := fsutil.VolumeID(req.GetVolumeId())

	if acquired := ns.VolumeLocks.TryAcquire(req.GetVolumeId()); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetVolumeId())
	}
	defer ns.VolumeLocks.Release(req.GetVolumeId())

	volOptions, err := ns.getVolumeOptions(ctx, volID, req.GetVolumeContext(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer volOptions.Destroy()

	volOptions.NetNamespaceFilePath, err = util.GetCephFSNetNamespaceFilePath(
		util.CsiConfigFile,
		volOptions.ClusterID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	mnt, err := mounter.New(volOptions)
	if err != nil {
		log.ErrorLog(ctx, "failed to create mounter for volume %s: %v", volID, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check if the volume is already mounted

	if err = ns.tryRestoreFuseMountInNodeStage(ctx, mnt, stagingTargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to try to restore FUSE mounts: %v", err)
	}

	isMnt, err := util.IsMountPoint(stagingTargetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		log.DebugLog(ctx, "cephfs: volume %s is already mounted to %s, skipping", volID, stagingTargetPath)

		return &csi.NodeStageVolumeResponse{}, nil
	}

	// It's not, mount now

	if err = ns.mount(
		ctx,
		mnt,
		volOptions,
		fsutil.VolumeID(req.GetVolumeId()),
		req.GetStagingTargetPath(),
		req.GetSecrets(),
		req.GetVolumeCapability(),
	); err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "cephfs: successfully mounted volume %s to %s", volID, stagingTargetPath)

	if _, isFuse := mnt.(*mounter.FuseMounter); isFuse {
		// FUSE mount recovery needs NodeStageMountinfo records.

		if err = fsutil.WriteNodeStageMountinfo(volID, &fsutil.NodeStageMountinfo{
			VolumeCapability: req.GetVolumeCapability(),
			Secrets:          req.GetSecrets(),
		}); err != nil {
			log.ErrorLog(ctx, "cephfs: failed to write NodeStageMountinfo for volume %s: %v", volID, err)

			// Try to clean node stage mount.
			if unmountErr := mounter.UnmountVolume(ctx, stagingTargetPath); unmountErr != nil {
				log.ErrorLog(ctx, "cephfs: failed to unmount %s in WriteNodeStageMountinfo clean up: %v",
					stagingTargetPath, unmountErr)
			}

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (*NodeServer) mount(
	ctx context.Context,
	mnt mounter.VolumeMounter,
	volOptions *store.VolumeOptions,
	volID fsutil.VolumeID,
	stagingTargetPath string,
	secrets map[string]string,
	volCap *csi.VolumeCapability,
) error {
	cr, err := getCredentialsForVolume(volOptions, secrets)
	if err != nil {
		log.ErrorLog(ctx, "failed to get ceph credentials for volume %s: %v", volID, err)

		return status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	log.DebugLog(ctx, "cephfs: mounting volume %s with %s", volID, mnt.Name())

	readOnly := "ro"

	if volCap.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
		volCap.AccessMode.Mode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY {
		switch mnt.(type) {
		case *mounter.FuseMounter:
			if !csicommon.MountOptionContains(strings.Split(volOptions.FuseMountOptions, ","), readOnly) {
				volOptions.FuseMountOptions = util.MountOptionsAdd(volOptions.FuseMountOptions, readOnly)
			}
		case *mounter.KernelMounter:
			if !csicommon.MountOptionContains(strings.Split(volOptions.KernelMountOptions, ","), readOnly) {
				volOptions.KernelMountOptions = util.MountOptionsAdd(volOptions.KernelMountOptions, readOnly)
			}
		}
	}

	if err = mnt.Mount(ctx, stagingTargetPath, cr, volOptions); err != nil {
		log.ErrorLog(ctx,
			"failed to mount volume %s: %v Check dmesg logs if required.",
			volID,
			err)

		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// NodePublishVolume mounts the volume mounted to the staging path to the target
// path.
func (ns *NodeServer) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	mountOptions := []string{"bind", "_netdev"}
	if err := util.ValidateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	stagingTargetPath := req.GetStagingTargetPath()
	targetPath := req.GetTargetPath()
	volID := fsutil.VolumeID(req.GetVolumeId())

	// Considering kubelet make sure the stage and publish operations
	// are serialized, we dont need any extra locking in nodePublish

	if err := util.CreateMountPoint(targetPath); err != nil {
		log.ErrorLog(ctx, "failed to create mount point at %s: %v", targetPath, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := ns.tryRestoreFuseMountsInNodePublish(
		ctx,
		volID,
		stagingTargetPath,
		targetPath,
		req.GetVolumeContext(),
	); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to try to restore FUSE mounts: %v", err)
	}

	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	// Ensure staging target path is a mountpoint.

	if isMnt, err := util.IsMountPoint(stagingTargetPath); err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	} else if !isMnt {
		return nil, status.Errorf(
			codes.Internal, "staging path %s for volume %s is not a mountpoint", stagingTargetPath, volID,
		)
	}

	// Check if the volume is already mounted

	isMnt, err := util.IsMountPoint(targetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		log.DebugLog(ctx, "cephfs: volume %s is already bind-mounted to %s", volID, targetPath)

		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, mount now

	if err = mounter.BindMount(
		ctx,
		stagingTargetPath,
		targetPath,
		req.GetReadonly(),
		mountOptions); err != nil {
		log.ErrorLog(ctx, "failed to bind-mount volume %s: %v", volID, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully bind-mounted volume %s to %s", volID, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path.
func (ns *NodeServer) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, err
	}

	// considering kubelet make sure node operations like unpublish/unstage...etc can not be called
	// at same time, an explicit locking at time of nodeunpublish is not required.
	targetPath := req.GetTargetPath()
	isMnt, err := util.IsMountPoint(targetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		if os.IsNotExist(err) {
			// targetPath has already been deleted
			log.DebugLog(ctx, "targetPath: %s has already been deleted", targetPath)

			return &csi.NodeUnpublishVolumeResponse{}, nil
		}

		if !util.IsCorruptedMountError(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Corrupted mounts need to be unmounted properly too,
		// regardless of the mounter used. Continue as normal.
		log.DebugLog(ctx, "cephfs: detected corrupted mount in publish target path %s, trying to unmount anyway", targetPath)
		isMnt = true
	}
	if !isMnt {
		if err = os.RemoveAll(targetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	// Unmount the bind-mount
	if err = mounter.UnmountVolume(ctx, targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully unbounded volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path.
func (ns *NodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnstageVolumeRequest(req); err != nil {
		return nil, err
	}

	volID := req.GetVolumeId()
	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	stagingTargetPath := req.GetStagingTargetPath()

	if err = fsutil.RemoveNodeStageMountinfo(fsutil.VolumeID(volID)); err != nil {
		log.ErrorLog(ctx, "cephfs: failed to remove NodeStageMountinfo for volume %s: %v", volID, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	isMnt, err := util.IsMountPoint(stagingTargetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		if os.IsNotExist(err) {
			// targetPath has already been deleted
			log.DebugLog(ctx, "targetPath: %s has already been deleted", stagingTargetPath)

			return &csi.NodeUnstageVolumeResponse{}, nil
		}

		if !util.IsCorruptedMountError(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Corrupted mounts need to be unmounted properly too,
		// regardless of the mounter used. Continue as normal.
		log.DebugLog(ctx,
			"cephfs: detected corrupted mount in staging target path %s, trying to unmount anyway",
			stagingTargetPath)
		isMnt = true
	}
	if !isMnt {
		return &csi.NodeUnstageVolumeResponse{}, nil
	}
	// Unmount the volume
	if err = mounter.UnmountVolume(ctx, stagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully unmounted volume %s from %s", req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (ns *NodeServer) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
		},
	}, nil
}

// NodeGetVolumeStats returns volume stats.
func (ns *NodeServer) NodeGetVolumeStats(
	ctx context.Context,
	req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	var err error
	targetPath := req.GetVolumePath()
	if targetPath == "" {
		err = fmt.Errorf("targetpath %v is empty", targetPath)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	stat, err := os.Stat(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to get stat for targetpath %q: %v", targetPath, err)
	}

	if stat.Mode().IsDir() {
		return csicommon.FilesystemNodeGetVolumeStats(ctx, targetPath)
	}

	return nil, status.Errorf(codes.InvalidArgument, "targetpath %q is not a directory or device", targetPath)
}
