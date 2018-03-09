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

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"k8s.io/kubernetes/pkg/util/keymutex"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
}

var nsMtx = keymutex.NewKeyMutex()

func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVersion() == nil {
		return status.Error(codes.InvalidArgument, "Version missing in request")
	}

	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	attrs := req.GetVolumeAttributes()

	if _, ok := attrs["path"]; !ok {
		return status.Error(codes.InvalidArgument, "Missing path attribute")
	}

	if _, ok := attrs["user"]; !ok {
		return status.Error(codes.InvalidArgument, "Missing user attribute")
	}

	return nil
}

func validateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVersion() == nil {
		return status.Error(codes.InvalidArgument, "Version missing in request")
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	return nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	// Configuration

	volId := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	if err := tryLock(volId, nsMtx, "NodeServer"); err != nil {
		return nil, err
	}
	defer nsMtx.UnlockKey(volId)

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
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, exec ceph-fuse now

	// TODO honor req.GetReadOnly()

	attrs := req.GetVolumeAttributes()
	vol := volume{Root: attrs["path"], User: attrs["user"]}

	if err := vol.mount(targetPath); err != nil {
		glog.Errorf("mounting volume %s to %s failed: %v", vol.Root, targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("cephfs: volume %s successfuly mounted to %s", vol.Root, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, err
	}

	volId := req.GetVolumeId()

	if err := tryLock(volId, nsMtx, "NodeServer"); err != nil {
		return nil, err
	}
	defer nsMtx.UnlockKey(volId)

	if err := unmountVolume(req.GetTargetPath()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
