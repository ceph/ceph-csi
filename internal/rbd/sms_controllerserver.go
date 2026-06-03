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

package rbd

import (
	"context"
	"errors"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/rbd/features"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// defaultMaxResults is the internal limit for max results in metadata streaming
	// This is set to a reasonable value to prevent excessive memory usage
	// and ensure efficient processing of metadata requests.
	// TODO: Consider making this configurable via a cmdline flag.
	defaultMaxResults int32 = 128
)

// normalizeMaxResults normalizes the maxResults parameter. It handles zero values,
// applies internal limits, and provides logging for debugging.
func normalizeMaxResults(ctx context.Context, requestedMaxResults int32) int32 {
	// Handle zero case - user wants default behavior
	if requestedMaxResults == 0 {
		log.DebugLog(ctx, "maxResults was 0, using internal default: %d", defaultMaxResults)

		return defaultMaxResults
	}

	// Apply internal max limit for memory management
	if requestedMaxResults > defaultMaxResults {
		log.DebugLog(ctx, "maxResults limited from %d to %d (internal max limit)",
			requestedMaxResults, defaultMaxResults)

		return defaultMaxResults
	}
	log.DebugLog(ctx, "using requested maxResults: %d", requestedMaxResults)

	return requestedMaxResults
}

func validateMetadataAllocatedReq(req *csi.GetMetadataAllocatedRequest) error {
	if req.GetSnapshotId() == "" {
		return status.Error(codes.InvalidArgument, "snapshot ID cannot be empty")
	}

	if req.GetSecrets() == nil || len(req.GetSecrets()) == 0 {
		return status.Error(codes.InvalidArgument, "secrets cannot be nil or empty")
	}

	if req.GetMaxResults() < 0 {
		return status.Error(codes.InvalidArgument, "max results cannot be negative")
	}

	if req.GetStartingOffset() < 0 {
		return status.Error(codes.OutOfRange, "starting offset cannot be negative")
	}

	return nil
}

// GetMetadataAllocated streams the allocated block metadata for a snapshot.
func (cs *ControllerServer) GetMetadataAllocated(
	req *csi.GetMetadataAllocatedRequest,
	stream csi.SnapshotMetadata_GetMetadataAllocatedServer,
) error {
	ctx := stream.Context()
	rbdSnapDiffByIDSupported, err := features.SupportsRBDSnapDiffByID()
	if err != nil {
		log.ErrorLog(ctx, "error checking for snapshot diff by ID support: %s", err)

		return status.Error(codes.Internal, err.Error())
	} else if !rbdSnapDiffByIDSupported {
		log.ErrorLog(ctx, "snapshot diff by ID feature is not supported")

		return status.Error(codes.Unimplemented, "snapshot diff by ID feature is not supported")
	}

	err = validateMetadataAllocatedReq(req)
	if err != nil {
		return err
	}

	snapshotID := req.GetSnapshotId()

	mgr := NewManager(cs.Driver.GetInstanceID(), nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	rbdSnap, err := mgr.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		if errors.Is(err, rbderrors.ErrImageNotFound) {
			log.ErrorLog(ctx, "snapshot %q not found: %v", snapshotID, err)

			return status.Errorf(codes.NotFound, "snapshot %q not found: %v", snapshotID, err)
		}

		return status.Error(codes.Internal, err.Error())
	}
	defer rbdSnap.Destroy(ctx)

	volSize := rbdSnap.GetSize()

	startingOffset := req.GetStartingOffset()
	if startingOffset >= volSize {
		return status.Errorf(codes.OutOfRange, "starting offset %d is out of range for volume size %d",
			startingOffset, volSize)
	}

	maxResults := normalizeMaxResults(ctx, req.GetMaxResults())

	sendResponse := func(changedBlocks []*csi.BlockMetadata) error {
		return stream.Send(&csi.GetMetadataAllocatedResponse{
			BlockMetadataType:   csi.BlockMetadataType_VARIABLE_LENGTH,
			VolumeCapacityBytes: volSize,
			BlockMetadata:       changedBlocks,
		})
	}

	err = rbdSnap.ProcessMetadata(ctx, startingOffset, maxResults, nil, sendResponse)
	if err != nil {
		log.ErrorLog(ctx, "failed to stream metadata allocated: %v", err)

		return status.Error(codes.Internal, err.Error())
	}
	log.DebugLog(ctx, "successfully streamed metadata allocated")

	return nil
}

func validateMetadataDeltaReq(req *csi.GetMetadataDeltaRequest) error {
	if req.GetBaseSnapshotId() == "" {
		return status.Error(codes.InvalidArgument, "base snapshot ID cannot be empty")
	}

	if req.GetTargetSnapshotId() == "" {
		return status.Error(codes.InvalidArgument, "target snapshot ID cannot be empty")
	}

	if req.GetSecrets() == nil || len(req.GetSecrets()) == 0 {
		return status.Error(codes.InvalidArgument, "secrets cannot be nil or empty")
	}

	if req.GetMaxResults() < 0 {
		return status.Error(codes.InvalidArgument, "max results cannot be negative")
	}

	if req.GetStartingOffset() < 0 {
		return status.Error(codes.OutOfRange, "starting offset cannot be negative")
	}

	return nil
}

// GetMetadataDelta streams the block metadata delta between two snapshots.
func (cs *ControllerServer) GetMetadataDelta(
	req *csi.GetMetadataDeltaRequest,
	stream csi.SnapshotMetadata_GetMetadataDeltaServer,
) error {
	ctx := stream.Context()
	rbdSnapDiffByIDSupported, err := features.SupportsRBDSnapDiffByID()
	if err != nil {
		log.ErrorLog(ctx, "failed to check for snapshot diff by ID support: %s", err)

		return status.Error(codes.Internal, err.Error())
	} else if !rbdSnapDiffByIDSupported {
		log.ErrorLog(ctx, "snapshot diff by ID feature is not supported")

		return status.Error(codes.Unimplemented, "snapshot diff by ID feature is not supported")
	}

	err = validateMetadataDeltaReq(req)
	if err != nil {
		return err
	}

	baseSnapshotID := req.GetBaseSnapshotId()
	targetSnapshotID := req.GetTargetSnapshotId()

	mgr := NewManager(cs.Driver.GetInstanceID(), nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	baseRBDSnap, err := mgr.GetSnapshotByID(ctx, baseSnapshotID)
	if err != nil {
		if errors.Is(err, rbderrors.ErrImageNotFound) {
			log.ErrorLog(ctx, "snapshot %q not found: %v", baseSnapshotID, err)

			return status.Errorf(codes.NotFound, "snapshot %q not found: %v", baseSnapshotID, err)
		}

		return status.Error(codes.Internal, err.Error())
	}
	defer baseRBDSnap.Destroy(ctx)

	targetRBDSnap, err := mgr.GetSnapshotByID(ctx, targetSnapshotID)
	if err != nil {
		if errors.Is(err, rbderrors.ErrImageNotFound) {
			log.ErrorLog(ctx, "snapshot %q not found: %v", targetSnapshotID, err)

			return status.Errorf(codes.NotFound, "snapshot %q not found: %v", targetSnapshotID, err)
		}

		return status.Error(codes.Internal, err.Error())
	}
	defer targetRBDSnap.Destroy(ctx)

	volSize := targetRBDSnap.GetSize()

	startingOffset := req.GetStartingOffset()
	if startingOffset >= volSize {
		return status.Errorf(codes.OutOfRange, "starting offset %d is out of range for volume size %d",
			startingOffset, volSize)
	}

	maxResults := normalizeMaxResults(ctx, req.GetMaxResults())

	sendResponse := func(changedBlocks []*csi.BlockMetadata) error {
		return stream.Send(&csi.GetMetadataDeltaResponse{
			BlockMetadataType:   csi.BlockMetadataType_VARIABLE_LENGTH,
			VolumeCapacityBytes: volSize,
			BlockMetadata:       changedBlocks,
		})
	}

	err = targetRBDSnap.ProcessMetadata(ctx, startingOffset, maxResults, baseRBDSnap, sendResponse)
	if err != nil {
		log.ErrorLog(ctx, "failed to stream metadata delta: %v", err)

		return status.Error(codes.Internal, err.Error())
	}
	log.DebugLog(ctx, "successfully streamed metadata delta")

	return nil
}
