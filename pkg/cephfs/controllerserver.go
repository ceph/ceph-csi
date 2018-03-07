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
	"fmt"
	"os"
	"path"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

const (
	oneGB = 1073741824
)

func GetVersionString(v *csi.Version) string {
	return fmt.Sprintf("%d.%d.%d", v.GetMajor(), v.GetMinor(), v.GetPatch())
}

func (cs *controllerServer) validateRequest(v *csi.Version) error {
	if v == nil {
		return status.Error(codes.InvalidArgument, "Version missing in request")
	}

	return cs.Driver.ValidateControllerServiceRequest(v, csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME)
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateRequest(req.Version); err != nil {
		glog.Warningf("invalid create volume request: %v", req)
		return nil, err
	}

	// Configuration

	volOptions, err := newVolumeOptions(req.GetParameters())
	if err != nil {
		return nil, err
	}

	volId := newVolumeIdentifier(volOptions, req)
	volSz := int64(oneGB)

	if req.GetCapacityRange() != nil {
		volSz = int64(req.GetCapacityRange().GetRequiredBytes())
	}

	if err := createMountPoint(provisionRoot); err != nil {
		glog.Errorf("failed to create provision root at %s: %v", provisionRoot, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Exec ceph-fuse only if cephfs has not been not mounted yet

	isMnt, err := isMountPoint(provisionRoot)

	if err != nil {
		glog.Errorf("stat failed: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if !isMnt {
		if err = mountFuse(provisionRoot); err != nil {
			glog.Error(err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Create a new directory inside the provision root for bind-mounting done by NodePublishVolume

	volPath := path.Join(provisionRoot, volId.id)
	if err := os.Mkdir(volPath, 0750); err != nil {
		glog.Errorf("failed to create volume %s: %v", volPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Set attributes & quotas

	if err = setVolAttributes(volPath, volSz); err != nil {
		glog.Errorf("failed to set attributes for volume %s: %v", volPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("cephfs: created volume %s", volPath)

	return &csi.CreateVolumeResponse{
		VolumeInfo: &csi.VolumeInfo{
			Id:            volId.id,
			CapacityBytes: uint64(volSz),
			Attributes:    req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateRequest(req.Version); err != nil {
		glog.Warningf("invalid delete volume request: %v", req)
		return nil, err
	}

	volId := req.GetVolumeId()
	volPath := path.Join(provisionRoot, volId)

	glog.V(4).Infof("deleting volume %s", volPath)

	if err := deleteVolumePath(volPath); err != nil {
		glog.Errorf("failed to delete volume %s: %v", volPath, err)
		return nil, err
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	res := &csi.ValidateVolumeCapabilitiesResponse{}

	for _, capability := range req.VolumeCapabilities {
		if capability.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return res, nil
		}
	}

	res.Supported = true
	return res, nil
}
