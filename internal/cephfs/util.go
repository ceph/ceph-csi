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
	"time"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type volumeID string

func execCommandErr(ctx context.Context, program string, args ...string) error {
	_, _, err := util.ExecCommand(ctx, program, args...)

	return err
}

// Controller service request validation.
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

func (cs *ControllerServer) validateDeleteVolumeRequest() error {
	if err := cs.Driver.ValidateControllerServiceRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid DeleteVolumeRequest: %w", err)
	}

	return nil
}

// Controller expand volume request validation.
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

func genSnapFromOptions(ctx context.Context, req *csi.CreateSnapshotRequest) (snap *cephfsSnapshot, err error) {
	cephfsSnap := &cephfsSnapshot{}
	cephfsSnap.RequestName = req.GetName()
	snapOptions := req.GetParameters()

	cephfsSnap.Monitors, cephfsSnap.ClusterID, err = util.GetMonsAndClusterID(snapOptions)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%s)", err)

		return nil, err
	}
	if namePrefix, ok := snapOptions["snapshotNamePrefix"]; ok {
		cephfsSnap.NamePrefix = namePrefix
	}

	return cephfsSnap, nil
}

func parseTime(ctx context.Context, createTime time.Time) (*timestamp.Timestamp, error) {
	tm, err := ptypes.TimestampProto(createTime)
	if err != nil {
		log.ErrorLog(ctx, "failed to convert time %s %v", createTime, err)

		return tm, err
	}

	return tm, nil
}
