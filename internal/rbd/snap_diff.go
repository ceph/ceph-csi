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
	"fmt"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/rbd/types"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// ProcessMetadata streams the block metadata for a snapshot.
// If baseRBDSnap is provided, it calculates the delta between the
// current snapshot and the base snapshot.
func (rbdSnap *rbdSnapshot) ProcessMetadata(
	ctx context.Context,
	startingOffset int64,
	maxResults int32,
	baseSnap types.Snapshot,
	sendResponse types.MetadataCallback,
) error {
	var fromSnapID uint64

	image, err := rbdSnap.open()
	if err != nil {
		return fmt.Errorf("failed to open image %q: %w", rbdSnap, err)
	}
	defer func() {
		cErr := image.Close()
		if cErr != nil {
			log.WarningLog(ctx, "resource leak, failed to close image: %v", cErr)
		}
	}()

	snapID, err := rbdSnap.getRBDSnapID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get snapshot ID of %q: %w", rbdSnap, err)
	}

	err = image.SetSnapByID(snapID)
	if err != nil {
		return fmt.Errorf("failed to set snapshot %q by ID %d: %w",
			rbdSnap, snapID, err)
	}

	if baseSnap != nil {
		baseRBDSnap, err := rbdSnapFromSnapshot(baseSnap)
		if err != nil {
			return fmt.Errorf("failed to convert base snapshot to rbdSnapshot: %w", err)
		}
		fromSnapID, err = baseRBDSnap.getRBDSnapID(ctx)
		if err != nil {
			return fmt.Errorf("failed to get ID of base snapshot ID: %w", err)
		}
	}

	luksHeaderPadding, err := rbdSnap.getLuksHeaderSizeMetadata()
	if err != nil {
		return fmt.Errorf("failed to get LUKS header size from metadata: %w", err)
	}
	// We need to adjust the starting offset to account for the LUKS header padding.
	startingOffset += int64(luksHeaderPadding)

	// If the starting offset is greater than the volume size after adding padding,
	// we can return early since there are no changed blocks to report.
	if startingOffset > rbdSnap.VolSize {
		return nil
	}

	changedBlocks := make([]*csi.BlockMetadata, 0)
	cb := createDiffIterateByIDCB(
		ctx,
		&changedBlocks,
		maxResults,
		luksHeaderPadding,
		sendResponse,
	)
	diffIterateByIDConfig := librbd.DiffIterateByIDConfig{
		Offset:   uint64(startingOffset),
		Length:   uint64(rbdSnap.VolSize) - uint64(startingOffset),
		Callback: cb,
	}
	if baseSnap != nil {
		// If a base snapshot is provided, set the FromSnapID to
		// the base snapshot ID to get the delta between the two snapshots.
		diffIterateByIDConfig.FromSnapID = fromSnapID
	}

	diffIterateErr := image.DiffIterateByID(diffIterateByIDConfig)
	err = handleDiffIterateError(diffIterateErr)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if len(changedBlocks) != 0 {
		// Send any remaining changed blocks after the loop.
		err = sendResponse(changedBlocks)
		if err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	}
	// Successfully completed the diff operation.
	return nil
}

// handleDiffIterateError checks the error returned by DiffIterateByID.
// It handles specific error codes and returns a formatted error message
// for unrecognized error codes.
func handleDiffIterateError(err error) error {
	if err == nil {
		// No error, return nil.
		return nil
	}
	// Check if the error implements ErrorCode to handle specific error codes.
	var errCode rbderrors.ErrorCode
	ok := errors.As(err, &errCode)
	if !ok {
		// The error does not implement ErrorCode, return a generic internal error.
		return fmt.Errorf("failed to get diff: %w", err)
	}

	switch errCode.ErrorCode() {
	case int(codes.OK):
		// No error, the diff operation completed successfully.
		return nil
	case int(codes.Canceled):
		// The stream was closed by the client.
		// This is not an error, just a normal termination of the stream.
		// Log the closure for debugging purposes.
		return nil
	case int(codes.Unknown):
		// An error occurred while sending the response.
		return fmt.Errorf("failed to send response: %w", err)
	}

	// An unexpected error occurred.
	return fmt.Errorf("failed to get diff: %w, unrecognized error code: %d", err, errCode)
}

// createDiffIterateByIDCB creates a DiffIterateCallback function that collects
// changed blocks and sends them to the stream when the maxResults limit is reached.
func createDiffIterateByIDCB(
	ctx context.Context,
	changedBlocks *[]*csi.BlockMetadata,
	maxResults int32,
	luksHeaderPadding uint64,
	sendResponse types.MetadataCallback,
) librbd.DiffIterateCallback {
	return func(offset, sizeBytes uint64, _ int, _ interface{}) int {
		select {
		case <-ctx.Done():
			return int(codes.Canceled)
		default:
			// subtract the LUKS header padding from the offset
			// since user data starts after the LUKS header.
			offset -= luksHeaderPadding
			*changedBlocks = append(*changedBlocks,
				&csi.BlockMetadata{
					ByteOffset: int64(offset),
					SizeBytes:  int64(sizeBytes),
				},
			)
		}
		if len(*changedBlocks) < int(maxResults) {
			// If we haven't reached the maxResults limit, continue collecting changed blocks.
			return int(codes.OK)
		}

		// Send the current batch of changed blocks to the stream since
		// we have reached the maxResults limit.
		err := sendResponse(*changedBlocks)
		if err != nil {
			return int(codes.Unknown)
		}
		// Reset the changedBlocks slice for the next batch.
		*changedBlocks = (*changedBlocks)[:0]

		return int(codes.OK)
	}
}
