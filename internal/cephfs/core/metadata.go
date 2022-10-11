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

const (
	// clusterNameKey cluster Key, set on cephfs subvolume.
	clusterNameKey = "csi.ceph.com/cluster/name"
)

// ErrSubVolMetadataNotSupported is returned when set/get/list/remove subvolume metadata options are not supported.
var ErrSubVolMetadataNotSupported = errors.New("subvolume metadata operations are not supported")

func (s *subVolumeClient) supportsSubVolMetadata() bool {
	newLocalClusterState(s.clusterID)

	return clusterAdditionalInfo[s.clusterID].subVolMetadataState != unsupported
}

func (s *subVolumeClient) isUnsupportedSubVolMetadata(err error) bool {
	var invalid fsAdmin.NotImplementedError
	if err != nil && errors.As(err, &invalid) {
		// In case the error is other than invalid command return error to the caller.
		clusterAdditionalInfo[s.clusterID].subVolMetadataState = unsupported

		return false
	}
	clusterAdditionalInfo[s.clusterID].subVolMetadataState = supported

	return true
}

// setMetadata sets custom metadata on the subvolume in a volume as a
// key-value pair.
func (s *subVolumeClient) setMetadata(key, value string) error {
	var err error
	if !s.supportsSubVolMetadata() {
		return ErrSubVolMetadataNotSupported
	}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		return err
	}
	err = fsa.SetMetadata(s.FsName, s.SubvolumeGroup, s.VolID, key, value)
	if !s.isUnsupportedSubVolMetadata(err) {
		return ErrSubVolMetadataNotSupported
	}

	return err
}

// removeMetadata removes custom metadata set on the subvolume in a volume
// using the metadata key.
func (s *subVolumeClient) removeMetadata(key string) error {
	var err error
	if !s.supportsSubVolMetadata() {
		return ErrSubVolMetadataNotSupported
	}
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		return err
	}
	err = fsa.RemoveMetadata(s.FsName, s.SubvolumeGroup, s.VolID, key)
	if !s.isUnsupportedSubVolMetadata(err) {
		return ErrSubVolMetadataNotSupported
	}

	return err
}

// SetAllMetadata set all the metadata from arg parameters on Ssubvolume.
func (s *subVolumeClient) SetAllMetadata(parameters map[string]string) error {
	if !s.enableMetadata {
		return nil
	}

	for k, v := range parameters {
		err := s.setMetadata(k, v)
		// If setMetadata is not supported return nil
		if errors.Is(err, ErrSubVolMetadataNotSupported) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on subvolume %v: %w", k, v, s, err)
		}
	}

	if s.clusterName != "" {
		err := s.setMetadata(clusterNameKey, s.clusterName)
		// If setMetadata is not supported return nil
		if errors.Is(err, ErrSubVolMetadataNotSupported) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on subvolume %v: %w",
				clusterNameKey, s.clusterName, s, err)
		}
	}

	return nil
}

// UnsetAllMetadata unset all the metadata from arg keys on subvolume.
func (s *subVolumeClient) UnsetAllMetadata(keys []string) error {
	if !s.enableMetadata {
		return nil
	}

	for _, key := range keys {
		err := s.removeMetadata(key)
		// If setMetadata is not supported return nil
		if errors.Is(err, ErrSubVolMetadataNotSupported) {
			return nil
		}
		// TODO: replace string comparison with errno.
		if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
			return fmt.Errorf("failed to unset metadata key %q on subvolume %v: %w", key, s, err)
		}
	}

	err := s.removeMetadata(clusterNameKey)
	// If setMetadata is not supported return nil
	if errors.Is(err, ErrSubVolMetadataNotSupported) {
		return nil
	}
	// TODO: replace string comparison with errno.
	if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
		return fmt.Errorf("failed to unset metadata key %q on subvolume %v: %w", clusterNameKey, s, err)
	}

	return nil
}
