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
	"context"

	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
)

// DefaultNodeServer stores driver object.
type DefaultNodeServer struct {
	csi.UnimplementedNodeServer
	Driver  *CSIDriver
	Type    string
	Mounter mount.Interface
}

// NodeExpandVolume returns unimplemented response.
func (ns *DefaultNodeServer) NodeExpandVolume(
	ctx context.Context,
	req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeGetInfo returns node ID.
func (ns *DefaultNodeServer) NodeGetInfo(
	ctx context.Context,
	req *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {
	log.TraceLog(ctx, "Using default NodeGetInfo")

	csiTopology := &csi.Topology{
		Segments: ns.Driver.topology,
	}

	return &csi.NodeGetInfoResponse{
		NodeId:             ns.Driver.nodeID,
		AccessibleTopology: csiTopology,
	}, nil
}

// NodeGetCapabilities returns RPC unknown capability.
func (ns *DefaultNodeServer) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	log.TraceLog(ctx, "Using default NodeGetCapabilities")

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
