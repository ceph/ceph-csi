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

package types

import (
	"context"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/util"
)

// MetadataCallback is a function type for sending responses in the SMS controller server.
type MetadataCallback func([]*csi.BlockMetadata) error

type Snapshot interface {
	journalledObject

	// Delete removes the snapshot from the storage backend.
	Delete(ctx context.Context) error

	ToCSI(ctx context.Context) (*csi.Snapshot, error)

	GetCreationTime(ctx context.Context) (*time.Time, error)

	// SetVolumeGroup sets the CSI volume group ID in the snapshot.
	SetVolumeGroup(ctx context.Context, creds *util.Credentials, vgID string) error

	// GetSize returns the size of the snapshot in bytes.
	GetSize() int64

	// ProcessMetadata processes the block metadata for a snapshot.
	// If baseRBDSnap is provided, the delta between the
	// current snapshot and the base snapshot is processed.
	ProcessMetadata(ctx context.Context,
		startingOffset int64,
		maxResults int32,
		baseRBDSnap Snapshot,
		sendResponse MetadataCallback) error
}
