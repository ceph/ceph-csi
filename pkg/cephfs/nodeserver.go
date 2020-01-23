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
	"fmt"
	"os"

	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

// NodeServer struct of ceph CSI driver with supported methods of CSI
// node server spec.
type NodeServer struct {
	*csicommon.DefaultNodeServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	VolumeLocks *util.VolumeLocks
}

func getCredentialsForVolume(volOptions *volumeOptions, req *csi.NodeStageVolumeRequest) (*util.Credentials, error) {
	var (
		err     error
		cr      *util.Credentials
		secrets = req.GetSecrets()
	)

	if volOptions.ProvisionVolume {
		// The volume is provisioned dynamically, use passed in admin credentials

		cr, err = util.NewAdminCredentials(secrets)
		if err != nil {
			return nil, fmt.Errorf("failed to get admin credentials from node stage secrets: %v", err)
		}
	} else {
		// The volume is pre-made, credentials are in node stage secrets

		cr, err = util.NewUserCredentials(req.GetSecrets())
		if err != nil {
			return nil, fmt.Errorf("failed to get user credentials from node stage secrets: %v", err)
		}
	}

	return cr, nil
}

// NodeStageVolume mounts the volume to a staging path on the node.
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var (
		volOptions *volumeOptions
	)
	if err := util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	// Configuration

	stagingTargetPath := req.GetStagingTargetPath()
	volID := volumeID(req.GetVolumeId())

	if acquired := ns.VolumeLocks.TryAcquire(req.GetVolumeId()); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetVolumeId())
	}
	defer ns.VolumeLocks.Release(req.GetVolumeId())

	volOptions, _, err := newVolumeOptionsFromVolID(ctx, string(volID), req.GetVolumeContext(), req.GetSecrets())
	if err != nil {
		if _, ok := err.(ErrInvalidVolID); !ok {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// check for pre-provisioned volumes (plugin versions > 1.0.0)
		volOptions, _, err = newVolumeOptionsFromStaticVolume(string(volID), req.GetVolumeContext())
		if err != nil {
			if _, ok := err.(ErrNonStaticVolume); !ok {
				return nil, status.Error(codes.Internal, err.Error())
			}

			// check for volumes from plugin versions <= 1.0.0
			volOptions, _, err = newVolumeOptionsFromVersion1Context(string(volID), req.GetVolumeContext(),
				req.GetSecrets())
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	// Check if the volume is already mounted

	isMnt, err := util.IsMountPoint(stagingTargetPath)

	if err != nil {
		klog.Errorf(util.Log(ctx, "stat failed: %v"), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		klog.Infof(util.Log(ctx, "cephfs: volume %s is already mounted to %s, skipping"), volID, stagingTargetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// It's not, mount now
	if err = ns.mount(ctx, volOptions, req); err != nil {
		return nil, err
	}

	klog.Infof(util.Log(ctx, "cephfs: successfully mounted volume %s to %s"), volID, stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (*NodeServer) mount(ctx context.Context, volOptions *volumeOptions, req *csi.NodeStageVolumeRequest) error {
	stagingTargetPath := req.GetStagingTargetPath()
	volID := volumeID(req.GetVolumeId())

	cr, err := getCredentialsForVolume(volOptions, req)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get ceph credentials for volume %s: %v"), volID, err)
		return status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	m, err := newMounter(volOptions)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to create mounter for volume %s: %v"), volID, err)
		return status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infof(util.Log(ctx, "cephfs: mounting volume %s with %s"), volID, m.name())

	if err = m.mount(ctx, stagingTargetPath, cr, volOptions); err != nil {
		klog.Errorf(util.Log(ctx, "failed to mount volume %s: %v"), volID, err)
		return status.Error(codes.Internal, err.Error())
	}
	if err := volumeMountCache.nodeStageVolume(ctx, req.GetVolumeId(), stagingTargetPath, volOptions.Mounter, req.GetSecrets()); err != nil {
		klog.Warningf(util.Log(ctx, "mount-cache: failed to stage volume %s %s: %v"), volID, stagingTargetPath, err)
	}
	return nil
}

// NodePublishVolume mounts the volume mounted to the staging path to the target
// path
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	mountOptions := []string{"bind", "_netdev"}
	if err := util.ValidateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	volID := req.GetVolumeId()

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	if err := util.CreateMountPoint(targetPath); err != nil {
		klog.Errorf(util.Log(ctx, "failed to create mount point at %s: %v"), targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	// Check if the volume is already mounted

	isMnt, err := util.IsMountPoint(targetPath)

	if err != nil {
		klog.Errorf(util.Log(ctx, "stat failed: %v"), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		klog.Infof(util.Log(ctx, "cephfs: volume %s is already bind-mounted to %s"), volID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, mount now

	if err = bindMount(ctx, req.GetStagingTargetPath(), req.GetTargetPath(), req.GetReadonly(), mountOptions); err != nil {
		klog.Errorf(util.Log(ctx, "failed to bind-mount volume %s: %v"), volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = volumeMountCache.nodePublishVolume(ctx, volID, targetPath, req.GetReadonly()); err != nil {
		klog.Warningf(util.Log(ctx, "mount-cache: failed to publish volume %s %s: %v"), volID, targetPath, err)
	}

	klog.Infof(util.Log(ctx, "cephfs: successfully bind-mounted volume %s to %s"), volID, targetPath)

	// #nosec - allow anyone to write inside the target path
	err = os.Chmod(targetPath, 0777)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to change targetpath permission for volume %s: %v"), volID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path
func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, err
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		klog.Infof(util.Log(ctx, util.VolumeOperationAlreadyExistsFmt), volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	if err = volumeMountCache.nodeUnPublishVolume(ctx, volID, targetPath); err != nil {
		klog.Warningf(util.Log(ctx, "mount-cache: failed to unpublish volume %s %s: %v"), volID, targetPath, err)
	}

	// Unmount the bind-mount
	if err = unmountVolume(ctx, targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof(util.Log(ctx, "cephfs: successfully unbinded volume %s from %s"), req.GetVolumeId(), targetPath)

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

	stagingTargetPath := req.GetStagingTargetPath()

	if err = volumeMountCache.nodeUnStageVolume(volID); err != nil {
		klog.Warningf(util.Log(ctx, "mount-cache: failed to unstage volume %s %s: %v"), volID, stagingTargetPath, err)
	}

	// Unmount the volume
	if err = unmountVolume(ctx, stagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.Infof(util.Log(ctx, "cephfs: successfully unmounted volume %s from %s"), req.GetVolumeId(), stagingTargetPath)

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
