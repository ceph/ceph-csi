/*
Copyright 2024 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// validateCreateVolumeGroupSnapshotRequest validates the request for creating
// a group snapshot of volumes.
func (cs *ControllerServer) validateCreateVolumeGroupSnapshotRequest(
	ctx context.Context,
	req *csi.CreateVolumeGroupSnapshotRequest,
) error {
	if err := cs.Driver.ValidateGroupControllerServiceRequest(
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT); err != nil {
		log.ErrorLog(ctx, "invalid create volume group snapshot req: %v", protosanitizer.StripSecrets(req))

		return err
	}

	// Check sanity of request volume group snapshot Name, Source Volume Id's
	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "volume group snapshot Name cannot be empty")
	}

	if len(req.GetSourceVolumeIds()) == 0 {
		return status.Error(codes.InvalidArgument, "source volume ids cannot be empty")
	}

	param := req.GetParameters()
	// check for ClusterID and fsName
	if value, ok := param["clusterID"]; !ok || value == "" {
		return status.Error(codes.InvalidArgument, "missing or empty clusterID")
	}

	if value, ok := param["fsName"]; !ok || value == "" {
		return status.Error(codes.InvalidArgument, "missing or empty fsName")
	}

	return nil
}
