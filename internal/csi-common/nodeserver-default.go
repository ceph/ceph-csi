/*
Copyright 2017 The Kubernetes Authors.

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

package csicommon

import (
	"fmt"
	"os"

	"github.com/ceph/ceph-csi/internal/util"

	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
)

// DefaultNodeServer stores driver object.
type DefaultNodeServer struct {
	Driver *CSIDriver
	Type   string
}

// NodeStageVolume returns unimplemented response.
func (ns *DefaultNodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeUnstageVolume returns unimplemented response.
func (ns *DefaultNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeExpandVolume returns unimplemented response.
func (ns *DefaultNodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeGetInfo returns node ID.
func (ns *DefaultNodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	util.TraceLog(ctx, "Using default NodeGetInfo")

	csiTopology := &csi.Topology{
		Segments: ns.Driver.topology,
	}

	return &csi.NodeGetInfoResponse{
		NodeId:             ns.Driver.nodeID,
		AccessibleTopology: csiTopology,
	}, nil
}

// NodeGetCapabilities returns RPC unknown capability.
func (ns *DefaultNodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	util.TraceLog(ctx, "Using default NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

// NodeGetVolumeStats returns volume stats.
func (ns *DefaultNodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	var err error
	targetPath := req.GetVolumePath()
	if targetPath == "" {
		err = fmt.Errorf("targetpath %v is empty", targetPath)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	/*
		volID := req.GetVolumeId()

		TODO: Map the volumeID to the targetpath.

		CephFS:
		   we need secret to connect to the ceph cluster to get the volumeID from volume
		   Name, however `secret` field/option is not available  in NodeGetVolumeStats spec,
		   Below issue covers this request and once its available, we can do the validation
		   as per the spec.

		   https://github.com/container-storage-interface/spec/issues/371

		RBD:
		   Below issue covers this request for RBD and once its available, we can do the validation
		   as per the spec.

		   https://github.com/ceph/ceph-csi/issues/511

	*/

	isMnt, err := util.IsMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.InvalidArgument, "targetpath %s does not exist", targetPath)
		}
		return nil, err
	}
	if !isMnt {
		return nil, status.Errorf(codes.InvalidArgument, "targetpath %s is not mounted", targetPath)
	}

	cephMetricsProvider := volume.NewMetricsStatFS(targetPath)
	volMetrics, volMetErr := cephMetricsProvider.GetMetrics()
	if volMetErr != nil {
		return nil, status.Error(codes.Internal, volMetErr.Error())
	}

	available, ok := (*(volMetrics.Available)).AsInt64()
	if !ok {
		klog.Errorf(util.Log(ctx, "failed to fetch available bytes"))
	}
	capacity, ok := (*(volMetrics.Capacity)).AsInt64()
	if !ok {
		klog.Errorf(util.Log(ctx, "failed to fetch capacity bytes"))
		return nil, status.Error(codes.Unknown, "failed to fetch capacity bytes")
	}
	used, ok := (*(volMetrics.Used)).AsInt64()
	if !ok {
		klog.Errorf(util.Log(ctx, "failed to fetch used bytes"))
	}
	inodes, ok := (*(volMetrics.Inodes)).AsInt64()
	if !ok {
		klog.Errorf(util.Log(ctx, "failed to fetch available inodes"))
		return nil, status.Error(codes.Unknown, "failed to fetch available inodes")
	}
	inodesFree, ok := (*(volMetrics.InodesFree)).AsInt64()
	if !ok {
		klog.Errorf(util.Log(ctx, "failed to fetch free inodes"))
	}

	inodesUsed, ok := (*(volMetrics.InodesUsed)).AsInt64()
	if !ok {
		klog.Errorf(util.Log(ctx, "failed to fetch used inodes"))
	}
	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: available,
				Total:     capacity,
				Used:      used,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

// ConstructMountOptions returns only unique mount options in slice.
func ConstructMountOptions(mountOptions []string, volCap *csi.VolumeCapability) []string {
	if m := volCap.GetMount(); m != nil {
		hasOption := func(options []string, opt string) bool {
			for _, o := range options {
				if o == opt {
					return true
				}
			}
			return false
		}
		for _, f := range m.MountFlags {
			if !hasOption(mountOptions, f) {
				mountOptions = append(mountOptions, f)
			}
		}
	}
	return mountOptions
}

// MountOptionContains checks the opt is present in mountOptions.
func MountOptionContains(mountOptions []string, opt string) bool {
	for _, mnt := range mountOptions {
		if mnt == opt {
			return true
		}
	}
	return false
}
