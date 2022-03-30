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

	fscore "github.com/ceph/ceph-csi/internal/cephfs/core"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	// clusterNameKey is the key in OMAP that contains the name of the
	// NFS-cluster. It will be prefixed with the journal configuration.
	clusterNameKey = "nfs.cluster"
)

// NFSVolume presents the API for consumption by the CSI-controller to create,
// modify and delete the NFS-exported CephFS volume. Instances of this struct
// are short lived, they only exist as long as a CSI-procedure is active.
type NFSVolume struct {
	// ctx is the context for this short living volume object
	ctx context.Context

	volumeID   string
	clusterID  string
	mons       string
	fscID      int64
	objectUUID string

	// TODO: drop in favor of a go-ceph connection
	cr        *util.Credentials
	connected bool
	conn      *util.ClusterConnection
}

// NewNFSVolume create a new NFSVolume instance for the currently executing
// CSI-procedure.
func NewNFSVolume(ctx context.Context, volumeID string) (*NFSVolume, error) {
	vi := util.CSIIdentifier{}

	err := vi.DecomposeCSIID(volumeID)
	if err != nil {
		return nil, fmt.Errorf("error decoding volume ID (%s): %w", volumeID, err)
	}

	return &NFSVolume{
		ctx:        ctx,
		volumeID:   volumeID,
		clusterID:  vi.ClusterID,
		fscID:      vi.LocationID,
		objectUUID: vi.ObjectUUID,
		conn:       &util.ClusterConnection{},
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
	if nv.connected {
		return nil
	}

	var err error
	nv.mons, err = util.Mons(util.CsiConfigFile, nv.clusterID)
	if err != nil {
		return fmt.Errorf("failed to get MONs for cluster (%s): %w", nv.clusterID, err)
	}

	err = nv.conn.Connect(nv.mons, cr)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}

	nv.cr = cr
	nv.connected = true

	return nil
}

// Destroy cleans up resources once the NFSVolume instance is not needed
// anymore.
func (nv *NFSVolume) Destroy() {
	if nv.connected {
		nv.conn.Destroy()
		nv.connected = false
	}
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

	err := nv.setNFSCluster(nfsCluster)
	if err != nil {
		return fmt.Errorf("failed to set NFS-cluster: %w", err)
	}

	// TODO: use new go-ceph API, see ceph/ceph-csi#2977
	// new versions of Ceph use a different command, and the go-ceph API
	// also seems to be different :-/
	//
	// run the new command, but fall back to the previous one in case of an
	// error
	cmds := [][]string{
		// ceph nfs export create cephfs --cluster-id <cluster_id>
		//     --pseudo-path <pseudo_path> --fsname <fsname>
		//     [--readonly] [--path=/path/in/cephfs]
		nv.createExportCommand("--cluster-id="+nfsCluster,
			"--fsname="+fs, "--pseudo-path="+nv.GetExportPath(),
			"--path="+path),
		// ceph nfs export create cephfs ${FS} ${NFS} /${EXPORT} ${SUBVOL_PATH}
		nv.createExportCommand(nfsCluster, fs, nv.GetExportPath(), path),
	}

	stderr, err := nv.retryIfInvalid(cmds)
	if err != nil {
		return fmt.Errorf("failed to create export %q in NFS-cluster %q"+
			"(%v): %s", nv, nfsCluster, err, stderr)
	}

	return nil
}

// retryIfInvalid executes the "ceph" command, and falls back to the next cmd
// in case the error is EINVAL.
func (nv *NFSVolume) retryIfInvalid(cmds [][]string) (string, error) {
	var (
		stderr string
		err    error
	)
	for _, cmd := range cmds {
		_, stderr, err = util.ExecCommand(nv.ctx, "ceph", cmd...)
		// in case of an invalid command, fallback to the next one
		if strings.Contains(stderr, "Error EINVAL: invalid command") {
			continue
		}

		// If we get here, either no error, or an unexpected error
		// happened. There is no need to retry an other command.
		break
	}

	return stderr, err
}

// createExportCommand returns the "ceph nfs export create ..." command
// arguments (without "ceph"). The order of the parameters matches old Ceph
// releases, new Ceph releases added --option formats, which can be added  when
// passing the parameters to this function.
func (nv *NFSVolume) createExportCommand(nfsCluster, fs, export, path string) []string {
	return []string{
		"--id", nv.cr.ID,
		"--keyfile=" + nv.cr.KeyFile,
		"-m", nv.mons,
		"nfs",
		"export",
		"create",
		"cephfs",
		fs,
		nfsCluster,
		export,
		path,
	}
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

	// TODO: use new go-ceph API, see ceph/ceph-csi#2977
	// new versions of Ceph use a different command, and the go-ceph API
	// also seems to be different :-/
	//
	// run the new command, but fall back to the previous one in case of an
	// error
	cmds := [][]string{
		// ceph nfs export rm <cluster_id> <pseudo_path>
		nv.deleteExportCommand("rm", nfsCluster),
		// ceph nfs export delete <cluster_id> <pseudo_path>
		nv.deleteExportCommand("delete", nfsCluster),
	}

	stderr, err := nv.retryIfInvalid(cmds)
	if err != nil {
		return fmt.Errorf("failed to delete export %q from NFS-cluster"+
			"%q (%v): %s", nv, nfsCluster, err, stderr)
	}

	return nil
}

// deleteExportCommand returns the "ceph nfs export delete ..." command
// arguments (without "ceph"). Old releases of Ceph expect "delete" as cmd,
// newer releases use "rm".
func (nv *NFSVolume) deleteExportCommand(cmd, nfsCluster string) []string {
	return []string{
		"--id", nv.cr.ID,
		"--keyfile=" + nv.cr.KeyFile,
		"-m", nv.mons,
		"nfs",
		"export",
		cmd,
		nfsCluster,
		nv.GetExportPath(),
	}
}

// getNFSCluster fetches the NFS-cluster name from the CephFS journal.
func (nv *NFSVolume) getNFSCluster() (string, error) {
	if !nv.connected {
		return "", fmt.Errorf("can not get NFS-cluster for %q: not connected", nv)
	}

	fs := fscore.NewFileSystem(nv.conn)
	fsName, err := fs.GetFsName(nv.ctx, nv.fscID)
	if err != nil {
		return "", fmt.Errorf("failed to get filesystem name for ID %x: %w", nv.fscID, err)
	}

	mdPool, err := fs.GetMetadataPool(nv.ctx, fsName)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata pool for %q: %w", fsName, err)
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := store.VolJournal.Connect(nv.mons, fsutil.RadosNamespace, nv.cr)
	if err != nil {
		return "", fmt.Errorf("failed to connect to journal: %w", err)
	}
	defer j.Destroy()

	clusterName, err := j.FetchAttribute(nv.ctx, mdPool, nv.objectUUID, clusterNameKey)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster name: %w", err)
	}

	return clusterName, nil
}

// setNFSCluster stores the NFS-cluster name in the CephFS journal.
func (nv *NFSVolume) setNFSCluster(clusterName string) error {
	if !nv.connected {
		return fmt.Errorf("can not set NFS-cluster for %q: not connected", nv)
	}

	fs := fscore.NewFileSystem(nv.conn)
	fsName, err := fs.GetFsName(nv.ctx, nv.fscID)
	if err != nil {
		return fmt.Errorf("failed to get filesystem name for ID %x: %w", nv.fscID, err)
	}

	mdPool, err := fs.GetMetadataPool(nv.ctx, fsName)
	if err != nil {
		return fmt.Errorf("failed to get metadata pool for %q: %w", fsName, err)
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := store.VolJournal.Connect(nv.mons, fsutil.RadosNamespace, nv.cr)
	if err != nil {
		return fmt.Errorf("failed to connect to journal: %w", err)
	}
	defer j.Destroy()

	err = j.StoreAttribute(nv.ctx, mdPool, nv.objectUUID, clusterNameKey, clusterName)
	if err != nil {
		return fmt.Errorf("failed to store cluster name: %w", err)
	}

	return nil
}
