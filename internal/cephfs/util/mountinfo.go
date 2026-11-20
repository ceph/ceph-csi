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

package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// This file provides functionality to store various mount information
// in a file. It is used to restore CephFS staging mounts.
// Mount info is stored in `/csi/mountinfo`.

const (
	mountinfoDir = "/csi/mountinfo"
)

// nodeStageMountinfoRecord describes a single
// record of mountinfo of a staged volume.
// encoding/json-friendly format.
// Only for internal use for marshaling and unmarshaling.
type nodeStageMountinfoRecord struct {
	VolumeCapabilityProtoJSON string            `json:",omitempty"`
	MountOptions              []string          `json:",omitempty"`
	Secrets                   map[string]string `json:",omitempty"`
	StagingTargetPath         string            `json:",omitempty"`
	VolumeContext             map[string]string `json:",omitempty"`
}

// NodeStageMountinfo describes mountinfo of a volume.
type NodeStageMountinfo struct {
	VolumeCapability *csi.VolumeCapability
	Secrets          map[string]string
	MountOptions     []string
	// StagingTargetPath and VolumeContext identify a kernel recovery record.
	StagingTargetPath string
	VolumeContext     map[string]string
}

func fmtNodeStageMountinfoFilename(volID VolumeID) string {
	return path.Join(mountinfoDir, fmt.Sprintf("nodestage-%s.json", volID))
}

func (mi *NodeStageMountinfo) toNodeStageMountinfoRecord() (*nodeStageMountinfoRecord, error) {
	bs, err := protojson.Marshal(proto.MessageV2(mi.VolumeCapability))
	if err != nil {
		return nil, err
	}

	return &nodeStageMountinfoRecord{
		VolumeCapabilityProtoJSON: string(bs),
		MountOptions:              mi.MountOptions,
		Secrets:                   mi.Secrets,
		StagingTargetPath:         mi.StagingTargetPath,
		VolumeContext:             mi.VolumeContext,
	}, nil
}

func (r *nodeStageMountinfoRecord) toNodeStageMountinfo() (*NodeStageMountinfo, error) {
	volCapability := &csi.VolumeCapability{}
	if err := protojson.Unmarshal([]byte(r.VolumeCapabilityProtoJSON), proto.MessageV2(volCapability)); err != nil {
		return nil, err
	}

	return &NodeStageMountinfo{
		VolumeCapability:  volCapability,
		MountOptions:      r.MountOptions,
		Secrets:           r.Secrets,
		StagingTargetPath: r.StagingTargetPath,
		VolumeContext:     r.VolumeContext,
	}, nil
}

// WriteNodeStageMountinfo writes mount info to a file.
func WriteNodeStageMountinfo(volID VolumeID, mi *NodeStageMountinfo) error {
	// Write NodeStageMountinfo into JSON-formatted byte slice.

	r, err := mi.toNodeStageMountinfoRecord()
	if err != nil {
		return err
	}

	bs, err := json.Marshal(r)
	if err != nil {
		return err
	}

	err = writeMountinfo(fmtNodeStageMountinfoFilename(volID), bs)
	if os.IsNotExist(err) && mi.StagingTargetPath == "" {
		// Preserve optional mountinfo directory handling for legacy FUSE records.
		return nil
	}

	return err
}

// GetNodeStageMountinfo tries to retrieve NodeStageMountinfoRecord for `volID`.
// If it doesn't exist, `(nil, nil)` is returned.
func GetNodeStageMountinfo(volID VolumeID) (*NodeStageMountinfo, error) {
	// Read the file.

	bs, err := os.ReadFile(fmtNodeStageMountinfoFilename(volID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	// Unmarshall JSON-formatted byte slice into NodeStageMountinfo struct.

	r := &nodeStageMountinfoRecord{}
	if err = json.Unmarshal(bs, r); err != nil {
		return nil, err
	}

	mi, err := r.toNodeStageMountinfo()
	if err != nil {
		return nil, err
	}

	return mi, err
}

// RemoveNodeStageMountinfo tries to remove NodeStageMountinfo for `volID`.
// If no such record exists for `volID`, it's considered success too.
func RemoveNodeStageMountinfo(volID VolumeID) error {
	return removeMountinfo(fmtNodeStageMountinfoFilename(volID))
}

func removeMountinfo(filename string) error {
	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	// A process crash can leave a temporary file containing credentials.
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "."+filepath.Base(filename)+".tmp-") {
			if err = os.Remove(filepath.Join(filepath.Dir(filename), entry.Name())); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err = os.Remove(filename); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

// writeMountinfo replaces a record without exposing partially written credentials.
func writeMountinfo(filename string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(filename), "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) //nolint:errcheck // Unstage also cleans up abandoned temporary files.
	defer f.Close()           //nolint:errcheck // The close before rename is checked; also close after failed writes.
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	return os.Rename(f.Name(), filename)
}
