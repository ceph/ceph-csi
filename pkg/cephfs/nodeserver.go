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

package cephfs

import (
	"context"
	"fmt"
	"os"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
}

func getCredentialsForVolume(volOptions *volumeOptions, volId volumeID, req *csi.NodeStageVolumeRequest) (*credentials, error) {
	var (
		userCr = &credentials{}
		err    error
	)

	if volOptions.ProvisionVolume {
		// The volume is provisioned dynamically, get the credentials directly from Ceph

		// First, store admin credentials - those are needed for retrieving the user credentials

		adminCr, err := getAdminCredentials(req.GetNodeStageSecrets())
		if err != nil {
			return nil, fmt.Errorf("failed to get admin credentials from node stage secrets: %v", err)
		}

		if err = storeCephCredentials(volId, adminCr); err != nil {
			return nil, fmt.Errorf("failed to store ceph admin credentials: %v", err)
		}

		// Then get the ceph user

		entity, err := getCephUser(adminCr, volId)
		if err != nil {
			return nil, fmt.Errorf("failed to get ceph user: %v", err)
		}

		userCr = entity.toCredentials()
	} else {
		// The volume is pre-made, credentials are in node stage secrets

		userCr, err = getUserCredentials(req.GetNodeStageSecrets())
		if err != nil {
			return nil, fmt.Errorf("failed to get user credentials from node stage secrets: %v", err)
		}
	}

	if err = storeCephCredentials(volId, userCr); err != nil {
		return nil, fmt.Errorf("failed to store ceph user credentials: %v", err)
	}

	return userCr, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if err := validateNodeStageVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Configuration

	stagingTargetPath := req.GetStagingTargetPath()
	volId := volumeID(req.GetVolumeId())

	volOptions, err := newVolumeOptions(req.GetVolumeAttributes())
	if err != nil {
		glog.Errorf("error reading volume options for volume %s: %v", volId, err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if volOptions.ProvisionVolume {
		// Dynamically provisioned volumes don't have their root path set, do it here
		volOptions.RootPath = getVolumeRootPath_ceph(volId)
	}

	if err = createMountPoint(stagingTargetPath); err != nil {
		glog.Errorf("failed to create staging mount point at %s for volume %s: %v", stagingTargetPath, volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	cephConf := cephConfigData{Monitors: volOptions.Monitors, VolumeID: volId}
	if err = cephConf.writeToFile(); err != nil {
		glog.Errorf("failed to write ceph config file to %s for volume %s: %v", getCephConfPath(volId), volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check if the volume is already mounted

	isMnt, err := isMountPoint(stagingTargetPath)

	if err != nil {
		glog.Errorf("stat failed: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		glog.Infof("cephfs: volume %s is already mounted to %s, skipping", volId, stagingTargetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// It's not, mount now

	cr, err := getCredentialsForVolume(volOptions, volId, req)
	if err != nil {
		glog.Errorf("failed to get ceph credentials for volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	m, err := newMounter(volOptions)
	if err != nil {
		glog.Errorf("failed to create mounter for volume %s: %v", volId, err)
	}

	glog.V(4).Infof("cephfs: mounting volume %s with %s", volId, m.name())

	if err = m.mount(stagingTargetPath, cr, volOptions, volId); err != nil {
		glog.Errorf("failed to mount volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.Infof("cephfs: successfully mounted volume %s to %s", volId, stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Configuration

	targetPath := req.GetTargetPath()
	volId := req.GetVolumeId()

	if err := createMountPoint(targetPath); err != nil {
		glog.Errorf("failed to create mount point at %s: %v", targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check if the volume is already mounted

	isMnt, err := isMountPoint(targetPath)

	if err != nil {
		glog.Errorf("stat failed: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		glog.Infof("cephfs: volume %s is already bind-mounted to %s", volId, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, mount now

	if err = bindMount(req.GetStagingTargetPath(), req.GetTargetPath(), req.GetReadonly()); err != nil {
		glog.Errorf("failed to bind-mount volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.Infof("cephfs: successfuly bind-mounted volume %s to %s", volId, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	targetPath := req.GetTargetPath()

	// Unmount the bind-mount
	if err := unmountVolume(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	os.Remove(targetPath)

	glog.Infof("cephfs: successfuly unbinded volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if err := validateNodeUnstageVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	stagingTargetPath := req.GetStagingTargetPath()

	// Unmount the volume
	if err := unmountVolume(stagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	os.Remove(stagingTargetPath)

	glog.Infof("cephfs: successfuly umounted volume %s from %s", req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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
