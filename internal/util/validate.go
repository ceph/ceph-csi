/*
Copyright 2025 The Ceph-CSI Authors.

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

package util

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ValidateControllerPublishVolumeRequest validates the controller publish request.
func ValidateControllerPublishVolumeRequest(req *csi.ControllerPublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability missing in request")
	}
	if req.GetNodeId() == "" {
		return status.Error(codes.InvalidArgument, "node ID missing in request")
	}

	return nil
}

// ValidateControllerUnpublishVolumeRequest validates the controller unpublish request.
func ValidateControllerUnpublishVolumeRequest(req *csi.ControllerUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if req.GetNodeId() == "" {
		return status.Error(codes.InvalidArgument, "node ID missing in request")
	}

	return nil
}

// ValidateNodeStageVolumeRequest validates the node stage request.
func ValidateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "staging target path missing in request")
	}

	if req.GetSecrets() == nil || len(req.GetSecrets()) == 0 {
		return status.Error(codes.InvalidArgument, "stage secrets cannot be nil or empty")
	}

	// validate stagingpath exists
	ok := checkDirExists(req.GetStagingTargetPath())
	if !ok {
		return status.Errorf(
			codes.InvalidArgument,
			"staging path %s does not exist on node",
			req.GetStagingTargetPath())
	}

	return nil
}

// ValidateNodeUnstageVolumeRequest validates the node unstage request.
func ValidateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "staging target path missing in request")
	}

	return nil
}

// ValidateNodePublishVolumeRequest validates the node publish request.
func ValidateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "target path missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "staging target path missing in request")
	}

	return nil
}

// ValidateNodeUnpublishVolumeRequest validates the node unpublish request.
func ValidateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "target path missing in request")
	}

	return nil
}

// CheckReadOnlyManyIsSupported checks the request is to create ReadOnlyMany
// volume is from source as empty ReadOnlyMany is not supported.
func CheckReadOnlyManyIsSupported(req *csi.CreateVolumeRequest) error {
	for _, capability := range req.GetVolumeCapabilities() {
		if m := capability.GetAccessMode().GetMode(); m == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
			m == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY {
			if req.GetVolumeContentSource() == nil {
				return status.Error(codes.InvalidArgument, "readOnly accessMode is supported only with content source")
			}
		}
	}

	return nil
}
