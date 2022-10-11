/*
Copyright 2022 The Ceph-CSI Authors.

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

package core

import (
	"errors"
	"fmt"
	"strings"

	fsAdmin "github.com/ceph/go-ceph/cephfs/admin"
)

// ErrSubVolSnapMetadataNotSupported is returned when set/get/list/remove
// subvolume snapshot metadata options are not supported.
var ErrSubVolSnapMetadataNotSupported = errors.New("subvolume snapshot metadata operations are not supported")

func (s *snapshotClient) supportsSubVolSnapMetadata() bool {
	newLocalClusterState(s.clusterID)

	return clusterAdditionalInfo[s.clusterID].subVolSnapshotMetadataState != unsupported
}

func (s *snapshotClient) isUnsupportedSubVolSnapMetadata(err error) bool {
	var invalid fsAdmin.NotImplementedError
	if err != nil && errors.As(err, &invalid) {
		// In case the error is other than invalid command return error to
		// the caller.
		clusterAdditionalInfo[s.clusterID].subVolSnapshotMetadataState = unsupported

		return false
	}
	clusterAdditionalInfo[s.clusterID].subVolSnapshotMetadataState = supported

	return true
}

// setSnapshotMetadata sets custom metadata on the subvolume snapshot in a
// volume as a key-value pair.
func (s *snapshotClient) setSnapshotMetadata(key, value string) error {
	if !s.supportsSubVolSnapMetadata() {
		return ErrSubVolSnapMetadataNotSupported
	}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		return err
	}

	err = fsa.SetSnapshotMetadata(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID, key, value)
	if !s.isUnsupportedSubVolSnapMetadata(err) {
		return ErrSubVolSnapMetadataNotSupported
	}

	return err
}

// removeSnapshotMetadata removes custom metadata set on the subvolume
// snapshot in a volume using the metadata key.
func (s *snapshotClient) removeSnapshotMetadata(key string) error {
	if !s.supportsSubVolSnapMetadata() {
		return ErrSubVolSnapMetadataNotSupported
	}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		return err
	}

	err = fsa.RemoveSnapshotMetadata(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID, key)
	if !s.isUnsupportedSubVolSnapMetadata(err) {
		return ErrSubVolSnapMetadataNotSupported
	}

	return err
}

// SetAllSnapshotMetadata set all the metadata from arg parameters on
// subvolume snapshot.
func (s *snapshotClient) SetAllSnapshotMetadata(parameters map[string]string) error {
	if !s.enableMetadata {
		return nil
	}

	for k, v := range parameters {
		err := s.setSnapshotMetadata(k, v)
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on subvolume snapshot %s %s in fs %s: %w",
				k, v, s.SnapshotID, s.VolID, s.FsName, err)
		}
	}

	if s.clusterName != "" {
		err := s.setSnapshotMetadata(clusterNameKey, s.clusterName)
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on subvolume snapshot %s %s in fs %s: %w",
				clusterNameKey, s.clusterName, s.SnapshotID, s.VolID, s.FsName, err)
		}
	}

	return nil
}

// UnsetAllSnapshotMetadata unset all the metadata from arg keys on subvolume
// snapshot.
func (s *snapshotClient) UnsetAllSnapshotMetadata(keys []string) error {
	if !s.enableMetadata {
		return nil
	}

	for _, key := range keys {
		err := s.removeSnapshotMetadata(key)
		// TODO: replace string comparison with errno.
		if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
			return fmt.Errorf("failed to unset metadata key %q on subvolume snapshot %s %s in fs %s: %w",
				key, s.SnapshotID, s.VolID, s.FsName, err)
		}
	}

	err := s.removeSnapshotMetadata(clusterNameKey)
	// TODO: replace string comparison with errno.
	if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
		return fmt.Errorf("failed to unset metadata key %q on subvolume snapshot %s %s in fs %s: %w",
			clusterNameKey, s.SnapshotID, s.VolID, s.FsName, err)
	}

	return nil
}
