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

package cephfs

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"

	"github.com/ceph/ceph-csi/internal/cephfs/store"
)

func TestBuildCreateVolumeResponse_WithRadosNamespace(t *testing.T) {
	t.Parallel()
	const ns = "csi-vol-4c742621-090a-484d-a33b"
	req := &csi.CreateVolumeRequest{Parameters: map[string]string{}}
	volOptions := &store.VolumeOptions{
		PoolNamespace: ns,
		RootPath:      "/volumes/csi/csi-vol-test/uuid",
	}
	vID := &store.VolumeIdentifier{FsSubvolName: "csi-vol-test", VolumeID: "vol-0001"}

	resp := buildCreateVolumeResponse(req, volOptions, vID)

	require.Equal(t, ns, resp.GetVolume().GetVolumeContext()["radosNamespace"])
}

func TestBuildCreateVolumeResponse_WithoutRadosNamespace(t *testing.T) {
	t.Parallel()
	req := &csi.CreateVolumeRequest{Parameters: map[string]string{}}
	volOptions := &store.VolumeOptions{
		RootPath: "/volumes/csi/csi-vol-test/uuid",
	}
	vID := &store.VolumeIdentifier{FsSubvolName: "csi-vol-test", VolumeID: "vol-0001"}

	resp := buildCreateVolumeResponse(req, volOptions, vID)

	_, present := resp.GetVolume().GetVolumeContext()["radosNamespace"]
	require.False(t, present, "radosNamespace must be absent when PoolNamespace is empty")
}

func TestBuildCreateVolumeResponse_SubvolNameAndPath(t *testing.T) {
	t.Parallel()
	req := &csi.CreateVolumeRequest{Parameters: map[string]string{}}
	volOptions := &store.VolumeOptions{
		RootPath: "/volumes/csi/csi-vol-test/uuid",
	}
	vID := &store.VolumeIdentifier{FsSubvolName: "csi-vol-test", VolumeID: "vol-0001"}

	resp := buildCreateVolumeResponse(req, volOptions, vID)

	vc := resp.GetVolume().GetVolumeContext()
	require.Equal(t, "csi-vol-test", vc["subvolumeName"])
	require.Equal(t, "/volumes/csi/csi-vol-test/uuid", vc["subvolumePath"])
}
