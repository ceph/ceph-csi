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

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// NFSVolume presents the API for consumption by the CSI-controller to create,
// modify and delete the NFS-exported CephFS volume. Instances of this struct
// are short lived, they only exist as long as a CSI-procedure is active.
type NFSVolume struct {
	// ctx is the context for this short living volume object
	ctx context.Context

	volumeID  string
	clusterID string
	mons      string

	// TODO: drop in favor of a go-ceph connection
	connected bool
	cr        *util.Credentials
}

// NewNFSVolume create a new NFSVolume instance for the currently executing
// CSI-procedure.
func NewNFSVolume(ctx context.Context, volumeID string) (*NFSVolume, error) {
	// TODO: validate volume.VolumeContext parameters
	vi := util.CSIIdentifier{}

	err := vi.DecomposeCSIID(volumeID)
	if err != nil {
		return nil, fmt.Errorf("error decoding volume ID (%s): %w", volumeID, err)
	}

	return &NFSVolume{
		ctx:      ctx,
		volumeID: volumeID,
	}, nil
}

// String returns a simple/short representation of the NFSVolume.
func (nv *NFSVolume) String() string {
	return nv.volumeID
}

// Connect fetches cluster connection details (like MONs) and connects to the
// Ceph cluster. This uses go-ceph, so after Connect(), Destroy() should be
// called to cleanup resources.
func (nv *NFSVolume) Connect(cr *util.Credentials) error {
	nv.cr = cr

	vi := util.CSIIdentifier{}

	err := vi.DecomposeCSIID(nv.volumeID)
	if err != nil {
		return fmt.Errorf("error decoding volume ID (%s): %w", nv.volumeID, err)
	}

	nv.clusterID = vi.ClusterID
	nv.mons, err = util.Mons(util.CsiConfigFile, vi.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to get MONs for cluster (%s): %w", vi.ClusterID, err)
	}

	nv.connected = true

	return nil
}

// GetExportPath returns the path on the NFS-server that can be used for
// mounting.
func (nv *NFSVolume) GetExportPath() string {
	return "/" + nv.volumeID
}

// CreateExport takes the (CephFS) CSI-volume and instructs Ceph Mgr to create
// a new NFS-export for the volume on the Ceph managed NFS-server.
func (nv *NFSVolume) CreateExport(backend *csi.Volume) error {
	if !nv.connected {
		return fmt.Errorf("can not created export for %q: not connected", nv)
	}

	fs := backend.VolumeContext["fsName"]
	nfsCluster := backend.VolumeContext["nfsCluster"]
	path := backend.VolumeContext["subvolumePath"]

	// ceph nfs export create cephfs ${FS} ${NFS} /${EXPORT} ${SUBVOL_PATH}
	args := []string{
		"--id", nv.cr.ID,
		"--keyfile=" + nv.cr.KeyFile,
		"-m", nv.mons,
		"nfs",
		"export",
		"create",
		"cephfs",
		fs,
		nfsCluster,
		nv.GetExportPath(),
		path,
	}

	// TODO: use new go-ceph API
	_, stderr, err := util.ExecCommand(nv.ctx, "ceph", args...)
	if err != nil {
		return fmt.Errorf("executing ceph export command failed (%w): %s", err, stderr)
	}

	return nil
}

// TODO: store the NFSCluster ("CephNFS" name) in the journal?
func (nv *NFSVolume) getNFSCluster() (string, error) {
	if !nv.connected {
		return "", fmt.Errorf("can not get the NFSCluster for %q: not connected", nv)
	}

	// ceph nfs cluster ls
	// FIXME: with a single CephNFS, it only returns a single like
	args := []string{
		"--id", nv.cr.ID,
		"--keyfile=" + nv.cr.KeyFile,
		"-m", nv.mons,
		"nfs",
		"cluster",
		"ls",
	}

	nfsCluster, _, err := util.ExecCommand(nv.ctx, "ceph", args...)
	if err != nil {
		return "", fmt.Errorf("executing ceph export command failed: %w", err)
	}

	return strings.TrimSpace(nfsCluster), nil
}

// DeleteExport removes the NFS-export from the Ceph managed NFS-server.
func (nv *NFSVolume) DeleteExport() error {
	if !nv.connected {
		return fmt.Errorf("can not delete export for %q: not connected", nv)
	}

	nfsCluster, err := nv.getNFSCluster()
	if err != nil {
		return fmt.Errorf("failed to identify NFS cluster: %w", err)
	}

	// ceph nfs export rm <cluster_id> <pseudo_path>
	args := []string{
		"--id", nv.cr.ID,
		"--keyfile=" + nv.cr.KeyFile,
		"-m", nv.mons,
		"nfs",
		"export",
		"delete",
		nfsCluster,
		nv.GetExportPath(),
	}

	// TODO: use new go-ceph API
	_, stderr, err := util.ExecCommand(nv.ctx, "ceph", args...)
	if err != nil {
		return fmt.Errorf("executing ceph export command failed (%w): %s", err, stderr)
	}

	return nil
}
