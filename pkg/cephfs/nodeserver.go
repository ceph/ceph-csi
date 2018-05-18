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

func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	return nil
}

func validateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	return nil
}

func handleUser(volOptions *volumeOptions, volUuid string, req *csi.NodePublishVolumeRequest) (*credentials, error) {
	var (
		cr  = &credentials{}
		err error
	)

	// Retrieve the credentials (possibly create a new user as well)

	if volOptions.ProvisionVolume {
		// The volume is provisioned dynamically, create a dedicated user

		if ent, err := createCephUser(volOptions, volUuid, req.GetReadonly()); err != nil {
			return nil, err
		} else {
			cr.id = ent.Entity[len(cephEntityClientPrefix):]
			cr.key = ent.Key
		}

		// Set the correct volume root path
		volOptions.RootPath = getVolumeRootPath_ceph(volUuid)
	} else {
		// The volume is pre-made, credentials are supplied by the user

		cr, err = getUserCredentials(req.GetNodePublishSecrets())

		if err != nil {
			return nil, err
		}
	}

	if err = storeCephUserCredentials(volUuid, cr, volOptions); err != nil {
		return nil, err
	}

	return cr, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	// Configuration

	targetPath := req.GetTargetPath()
	volId := req.GetVolumeId()
	volUuid := uuidFromVolumeId(volId)

	volOptions, err := newVolumeOptions(req.GetVolumeAttributes())
	if err != nil {
		glog.Errorf("error reading volume options: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = createMountPoint(targetPath); err != nil {
		glog.Errorf("failed to create mount point at %s: %v", targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	conf := cephConfigData{Monitors: volOptions.Monitors, VolumeUuid: volUuid}
	if err = conf.writeToFile(); err != nil {
		glog.Errorf("couldn't generate ceph.conf: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check if the volume is already mounted

	isMnt, err := isMountPoint(targetPath)

	if err != nil {
		glog.Errorf("stat failed: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		glog.V(4).Infof("cephfs: volume %s is already mounted to %s", volId, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, mount now

	cr, err := handleUser(volOptions, volUuid, req)

	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	m := newMounter(volOptions)
	if err = m.mount(targetPath, cr, volOptions, volUuid, req.GetReadonly()); err != nil {
		glog.Errorf("failed to mount volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("cephfs: volume %s successfuly mounted to %s", volId, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, err
	}

	volId := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	// Unmount the bind-mount
	if err := unmountVolume(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	localVolRoot := getVolumeRootPath_local(uuidFromVolumeId(volId))

	// Unmount the volume root
	if err := unmountVolume(localVolRoot); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	os.Remove(localVolRoot)

	glog.V(4).Infof("cephfs: volume %s successfuly unmounted from %s", volId, req.GetTargetPath())

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
