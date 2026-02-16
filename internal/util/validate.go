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
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// VolumeContextServiceAccountKey is the key in the volume context that
	// contains the pod's service account name, set by Kubelet when
	// podInfoOnMount is enabled in the CSIDriver spec.
	VolumeContextServiceAccountKey = "csi.storage.k8s.io/serviceAccount.name"

	// PublishContextServiceAccount is the publish context key for the allowed
	// service account, set during ControllerPublishVolume.
	PublishContextServiceAccount = "serviceAccount"
)

// A regex to verify the expected format: 0000-0000-arbitrary-number-of-000-and-chars.
// First two blocks are hexadecimal.
var validator = regexp.MustCompile(`^[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[a-zA-Z0-9\-_]+$`)

// ValidateControllerPublishVolumeRequest validates the controller publish request.
func ValidateControllerPublishVolumeRequest(req *csi.ControllerPublishVolumeRequest) error {
	if err := ValidateVolumeID(req.GetVolumeId(), IsStaticVol(req.GetVolumeContext())); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
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
	if err := ValidateVolumeID(req.GetVolumeId(), true); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
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

	if err := ValidateVolumeID(req.GetVolumeId(), IsStaticVol(req.GetVolumeContext())); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
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
	if err := ValidateVolumeID(req.GetVolumeId(), true); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
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

	if err := ValidateVolumeID(req.GetVolumeId(), IsStaticVol(req.GetVolumeContext())); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
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
	if err := ValidateVolumeID(req.GetVolumeId(), true); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
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

// ValidateVolumeID checks if the specified volumeID matches
// the expected format: 0000-0000-arbitrary-number-of-000-and-chars
// and is void of path traversal characters. The check for expected
// format is not enforced when `skipFormatCheck` is true.
func ValidateVolumeID(volumeID string, skipFormatCheck bool) error {
	// should be non empty
	if volumeID == "" {
		return fmt.Errorf("the volumeID cannot be empty: %q", volumeID)
	}

	// should not contain path traversal sequences
	if strings.Contains(volumeID, "..") {
		return fmt.Errorf("the volumeID contains path traversal sequences: %q", volumeID)
	}
	if strings.ContainsAny(volumeID, "/\\") {
		return fmt.Errorf("volumeID contains invalid path characters: %q", volumeID)
	}

	// Should match the expected format: 0000-0000-arbitrary-number-of-000-and-chars.
	// This is checked only when the volume is not statically provisioned.
	if matches := validator.MatchString(volumeID); !skipFormatCheck && !matches {
		return fmt.Errorf("the volumeID has an unexpected format: %q", volumeID)
	}

	// Is a valid volumeID.
	return nil
}

// IsStaticVol checks the volumeAttribute of a volume to determine
// if it is statically provisioned.
func IsStaticVol(volAttrs map[string]string) bool {
	val, ok := volAttrs["staticVolume"]
	if ok {
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			return false
		}

		return boolVal
	}

	return false
}

// ValidateServiceAccountRestriction checks whether the pod's service
// account is allowed to mount the volume. allowedSA is a
// comma-separated list of permitted service accounts (empty means no
// restriction). podSA is the service account of the requesting pod.
// volumeID is used only for log/error messages.
func ValidateServiceAccountRestriction(
	ctx context.Context,
	allowedSA, podSA, volumeID string,
) error {
	if allowedSA == "" {
		return nil
	}

	if podSA == "" {
		// podInfoOnMount is not enabled, cannot enforce restriction
		log.WarningLog(ctx,
			"volume %s has service account restriction "+
				"but podInfoOnMount is not enabled, "+
				"skipping check",
			volumeID)

		return nil
	}

	allowedSAs := strings.Split(allowedSA, ",")
	if !slices.Contains(allowedSAs, podSA) {
		return fmt.Errorf(
			"volume %s service account restriction "+
				"does not match pod's service account",
			volumeID)
	}

	log.DebugLog(ctx,
		"service account is allowed to mount volume %s",
		volumeID)

	return nil
}
