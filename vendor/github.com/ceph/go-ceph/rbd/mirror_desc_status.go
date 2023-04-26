//go:build !nautilus && ceph_preview
// +build !nautilus,ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"encoding/json"
	"strings"
)

// MirrorDescriptionReplayStatus contains information pertaining to the status
// of snapshot based RBD image mirroring.
type MirrorDescriptionReplayStatus struct {
	ReplayState              string  `json:"replay_state"`
	RemoteSnapshotTimestamp  int64   `json:"remote_snapshot_timestamp"`
	LocalSnapshotTimestamp   int64   `json:"local_snapshot_timestamp"`
	SyncingSnapshotTimestamp int64   `json:"syncing_snapshot_timestamp"`
	SyncingPercent           int     `json:"syncing_percent"`
	BytesPerSecond           float64 `json:"bytes_per_second"`
	BytesPerSnapshot         float64 `json:"bytes_per_snapshot"`
	LastSnapshotSyncSeconds  int64   `json:"last_snapshot_sync_seconds"`
	LastSnapshotBytes        int64   `json:"last_snapshot_bytes"`
}

// extractDescriptionJSON will extract one string containing a JSON object from
// the description if one can be found.
func (s *SiteMirrorImageStatus) extractDescriptionJSON() (string, error) {
	start := strings.Index(s.Description, "{")
	if start == -1 {
		return "", ErrNotExist
	}
	end := strings.LastIndex(s.Description, "}")
	if end == -1 {
		return "", ErrNotExist
	}
	if start >= end {
		return "", ErrNotExist
	}
	return s.Description[start : end+1], nil
}

// UnmarshalDescriptionJSON parses an embedded JSON string that may be found in
// the description of the SiteMirrorImageStatus. It will store the result in
// the value pointed to by v.  If no embedded JSON string is found an
// ErrNotExist error is returned. An error may also be returned if the contents
// can not be parsed.
func (s *SiteMirrorImageStatus) UnmarshalDescriptionJSON(v interface{}) error {
	desc, err := s.extractDescriptionJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(desc), v)
}

// DescriptionReplayStatus parses a MirrorDescriptionReplayStatus result out of
// the image status description field if available. If the embedded status JSON
// is not found or fails to parse and error will be returned.
func (s *SiteMirrorImageStatus) DescriptionReplayStatus() (
	*MirrorDescriptionReplayStatus, error) {
	// ---
	mdrs := MirrorDescriptionReplayStatus{}
	if err := s.UnmarshalDescriptionJSON(&mdrs); err != nil {
		return nil, err
	}
	return &mdrs, nil
}
