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
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"

	"github.com/ceph/ceph-csi/pkg/util"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	MetadataStore util.CachePersister
}

type controllerCacheEntry struct {
	VolOptions volumeOptions
	VolumeID   volumeID
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(req); err != nil {
		glog.Errorf("CreateVolumeRequest validation failed: %v", err)
		return nil, err
	}
	// Configuration

	volOptions, err := newVolumeOptions(req.GetParameters())
	if err != nil {
		glog.Errorf("validation of volume options failed: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volId := newVolumeID()

	conf := cephConfigData{Monitors: volOptions.Monitors, VolumeID: volId}
	if err = conf.writeToFile(); err != nil {
		glog.Errorf("failed to write ceph config file to %s: %v", getCephConfPath(volId), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Create a volume in case the user didn't provide one

	if volOptions.ProvisionVolume {
		// Admin credentials are required
		cr, err := getAdminCredentials(req.GetControllerCreateSecrets())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if err = storeCephCredentials(volId, cr); err != nil {
			glog.Errorf("failed to store admin credentials for '%s': %v", cr.id, err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		if err = createVolume(volOptions, cr, volId, req.GetCapacityRange().GetRequiredBytes()); err != nil {
			glog.Errorf("failed to create volume %s: %v", req.GetName(), err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		if _, err = createCephUser(volOptions, cr, volId); err != nil {
			glog.Errorf("failed to create ceph user for volume %s: %v", req.GetName(), err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		glog.Infof("cephfs: successfully created volume %s", volId)
	} else {
		glog.Infof("cephfs: volume %s is provisioned statically", volId)
	}

	ce := &controllerCacheEntry{VolOptions: *volOptions, VolumeID: volId}
	if err := cs.MetadataStore.Create(string(volId), ce); err != nil {
		glog.Errorf("failed to store a cache entry for volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			Id:            string(volId),
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			Attributes:    req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateDeleteVolumeRequest(req); err != nil {
		glog.Errorf("DeleteVolumeRequest validation failed: %v", err)
		return nil, err
	}

	var (
		volId = volumeID(req.GetVolumeId())
		err   error
	)

	ce := &controllerCacheEntry{}
	if err := cs.MetadataStore.Get(string(volId), ce); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if !ce.VolOptions.ProvisionVolume {
		// DeleteVolume() is forbidden for statically provisioned volumes!

		glog.Warningf("volume %s is provisioned statically, aborting delete", volId)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// Deleting a volume requires admin credentials

	cr, err := getAdminCredentials(req.GetControllerDeleteSecrets())
	if err != nil {
		glog.Errorf("failed to retrieve admin credentials: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = purgeVolume(volId, cr, &ce.VolOptions); err != nil {
		glog.Errorf("failed to delete volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = deleteCephUser(cr, volId); err != nil {
		glog.Errorf("failed to delete ceph user for volume %s: %v", volId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := cs.MetadataStore.Delete(string(volId)); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.Infof("cephfs: successfully deleted volume %s", volId)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// Cephfs doesn't support Block volume
	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			return &csi.ValidateVolumeCapabilitiesResponse{Supported: false, Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Supported: true}, nil
}
