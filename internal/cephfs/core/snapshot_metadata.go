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
	"fmt"
	"strings"
)

// setSnapshotMetadata sets custom metadata on the subvolume snapshot in a
// volume as a key-value pair.
func (s *snapshotClient) setSnapshotMetadata(key, value string) error {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		return err
	}

	return fsa.SetSnapshotMetadata(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID, key, value)
}

// removeSnapshotMetadata removes custom metadata set on the subvolume
// snapshot in a volume using the metadata key.
func (s *snapshotClient) removeSnapshotMetadata(key string) error {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		return err
	}

	return fsa.RemoveSnapshotMetadata(s.FsName, s.SubvolumeGroup, s.VolID, s.SnapshotID, key)
}

// SetAllSnapshotMetadata set all the metadata from arg parameters on
// subvolume snapshot.
func (s *snapshotClient) SetAllSnapshotMetadata(parameters map[string]string) error {
	for k, v := range parameters {
		err := s.setSnapshotMetadata(k, v)
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on subvolume snapshot %s %s in fs %s: %w",
				k, v, s.SnapshotID, s.VolID, s.FsName, err)
		}
	}

	return nil
}

// UnsetAllSnapshotMetadata unset all the metadata from arg keys on subvolume
// snapshot.
func (s *snapshotClient) UnsetAllSnapshotMetadata(keys []string) error {
	for _, key := range keys {
		err := s.removeSnapshotMetadata(key)
		// TODO: replace string comparison with errno.
		if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
			return fmt.Errorf("failed to unset metadata key %q on subvolume snapshot %s %s in fs %s: %w",
				key, s.SnapshotID, s.VolID, s.FsName, err)
		}
	}

	return nil
}
