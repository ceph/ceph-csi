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

	"github.com/container-storage-interface/spec/lib/go/csi"
	// google.golang.org/protobuf/encoding doesn't offer MessageV2().
	"github.com/golang/protobuf/proto" //nolint:staticcheck // See comment above.
	"google.golang.org/protobuf/encoding/protojson"
)

// This file provides functionality to store various mount information
// in a file. It's currently used to restore ceph-fuse mounts.
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
}

// NodeStageMountinfo describes mountinfo of a volume.
type NodeStageMountinfo struct {
	VolumeCapability *csi.VolumeCapability
	Secrets          map[string]string
	MountOptions     []string
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
	}, nil
}

func (r *nodeStageMountinfoRecord) toNodeStageMountinfo() (*NodeStageMountinfo, error) {
	volCapability := &csi.VolumeCapability{}
	if err := protojson.Unmarshal([]byte(r.VolumeCapabilityProtoJSON), proto.MessageV2(volCapability)); err != nil {
		return nil, err
	}

	return &NodeStageMountinfo{
		VolumeCapability: volCapability,
		MountOptions:     r.MountOptions,
		Secrets:          r.Secrets,
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

	// Write the byte slice into file.

	err = os.WriteFile(fmtNodeStageMountinfoFilename(volID), bs, 0o600)
	if os.IsNotExist(err) {
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
	if err := os.Remove(fmtNodeStageMountinfoFilename(volID)); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}
