/*
Copyright 2021 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// validateCreateVolumeRequest validates the Controller CreateVolume request.
func (cs *ControllerServer) validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid CreateVolumeRequest: %w", err)
	}

	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "volume Name cannot be empty")
	}

	reqCaps := req.GetVolumeCapabilities()
	if reqCaps == nil {
		return status.Error(codes.InvalidArgument, "volume Capabilities cannot be empty")
	}

	for _, capability := range reqCaps {
		if capability.GetBlock() != nil {
			return status.Error(codes.Unimplemented, "block volume not supported")
		}
	}

	// Allow readonly access mode for volume with content source
	err := util.CheckReadOnlyManyIsSupported(req)
	if err != nil {
		return err
	}

	if req.VolumeContentSource != nil {
		volumeSource := req.VolumeContentSource
		switch volumeSource.Type.(type) {
		case *csi.VolumeContentSource_Snapshot:
			snapshot := req.VolumeContentSource.GetSnapshot()
			// CSI spec requires returning NOT_FOUND when the volumeSource is missing/incorrect.
			if snapshot == nil {
				return status.Error(codes.NotFound, "volume Snapshot cannot be empty")
			}
			if snapshot.GetSnapshotId() == "" {
				return status.Error(codes.NotFound, "volume Snapshot ID cannot be empty")
			}
		case *csi.VolumeContentSource_Volume:
			// CSI spec requires returning NOT_FOUND when the volumeSource is missing/incorrect.
			vol := req.VolumeContentSource.GetVolume()
			if vol == nil {
				return status.Error(codes.NotFound, "volume cannot be empty")
			}
			if vol.GetVolumeId() == "" {
				return status.Error(codes.NotFound, "volume ID cannot be empty")
			}

		default:
			return status.Error(codes.InvalidArgument, "unsupported volume data source")
		}
	}

	return nil
}

// validateDeleteVolumeRequest validates the Controller DeleteVolume request.
func (cs *ControllerServer) validateDeleteVolumeRequest() error {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid DeleteVolumeRequest: %w", err)
	}

	return nil
}

// validateExpandVolumeRequest validates the Controller ExpandVolume request.
func (cs *ControllerServer) validateExpandVolumeRequest(req *csi.ControllerExpandVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		return fmt.Errorf("invalid ExpandVolumeRequest: %w", err)
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}

	return nil
}
