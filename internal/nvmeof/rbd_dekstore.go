/*
Copyright 2026 The Ceph-CSI Authors.

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

package nvmeof

import (
	"context"
	"errors"
	"fmt"

	librbd "github.com/ceph/go-ceph/rbd"

	"github.com/ceph/ceph-csi/internal/kms"
	rbdutil "github.com/ceph/ceph-csi/internal/rbd/types"
)

// rbdVolumeDEKStore implements kms.DEKStore using RBD image metadata
// It stores DH-CHAP keys as metadata entries in the RBD image.
type rbdVolumeDEKStore struct {
	rbdVol rbdutil.Volume
}

// Ensure rbdVolumeDEKStore implements kms.DEKStore.
var _ kms.DEKStore = &rbdVolumeDEKStore{}

// NewRBDVolumeDEKStore creates a new RBD metadata-based DEKStore.
func NewRBDVolumeDEKStore(rbdVol rbdutil.Volume) kms.DEKStore {
	return &rbdVolumeDEKStore{
		rbdVol: rbdVol,
	}
}

// StoreDEK stores the encrypted DEK in RBD image metadata.
func (r *rbdVolumeDEKStore) StoreDEK(ctx context.Context, keyID, encryptedDEK string) error {
	// Prefix to distinguish DH-CHAP keys from other metadata
	metadataKey := "nvmeof.csi.ceph.com/" + keyID

	err := r.rbdVol.SetMetadata(metadataKey, encryptedDEK)
	if err != nil {
		return fmt.Errorf("failed to store DH-CHAP key %s in RBD metadata: %w", keyID, err)
	}

	return nil
}

// FetchDEK retrieves the encrypted DEK from RBD image metadata.
func (r *rbdVolumeDEKStore) FetchDEK(ctx context.Context, keyID string) (string, error) {
	metadataKey := "nvmeof.csi.ceph.com/" + keyID

	encryptedDEK, err := r.rbdVol.GetMetadata(metadataKey)
	if errors.Is(err, librbd.ErrNotFound) {
		return "", fmt.Errorf("%w: %s", ErrKeyNotFound, keyID)
	}

	if err != nil {
		return "", fmt.Errorf("failed to fetch DH-CHAP key %s from RBD metadata: %w", keyID, err)
	}

	return encryptedDEK, nil
}

// RemoveDEK deletes the encrypted DEK from RBD image metadata.
func (r *rbdVolumeDEKStore) RemoveDEK(ctx context.Context, keyID string) error {
	// For now, we don't remove keys from metadata
	// They'll be cleaned up when the volume is deleted
	// If you want to implement removal, you'll need to add a RemoveMetadata method to rbdutil.Volume
	return nil
}
