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

package types

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ceph/go-ceph/common/admin/nfs"
	"github.com/container-storage-interface/spec/lib/go/csi"

	fscore "github.com/ceph/ceph-csi/internal/cephfs/core"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// clusterNameKey is the key in OMAP that contains the name of the
	// NFS-cluster. It will be prefixed with the journal configuration.
	clusterNameKey = "nfs.cluster"

	// exportNameKey is the key in OMAP that contains a friendly export
	// name, when one was set. It will be prefixed with the journal
	// configuration, same as clusterNameKey.
	exportNameKey = "nfs.exportName"

	// ParameterServer is set in the parameters on volume creation and in
	// the VolumeContext.
	ParameterServer = "server"

	// ParameterClients is set in the parameters on volume creation and
	// configured for the export in the NFS-server. It is not stored in
	// the VolumeContext.
	ParameterClients = "clients"

	// ParameterCluster is set in the parameters on volume creation and in
	// the VolumeContext.
	ParameterCluster = "nfsCluster"

	// ParameterSecTypes is set in the parameters on volume creation and in
	// the VolumeContext.
	ParameterSecTypes = "secTypes"

	// ParameterFriendlyExportNames opts a StorageClass in to naming
	// exports "<namespace>/<pvc-name>" instead of the generated volume
	// ID. Requires the external-provisioner to run with
	// --extra-create-metadata=true.
	ParameterFriendlyExportNames = "friendlyExportNames"
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

	// exportName, when set, replaces volumeID in the export's pseudo-path.
	exportName string

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
// mounting. Falls back to the volumeID when no friendly name was set
// (SetExportName) or resolved (resolveExportName).
func (nv *NFSVolume) GetExportPath() string {
	if nv.exportName != "" {
		return "/" + nv.exportName
	}

	return "/" + nv.volumeID
}

// SetExportName sets a friendly export name and persists it in the CephFS
// journal.
func (nv *NFSVolume) SetExportName(name string) error {
	err := nv.setAttribute(exportNameKey, name)
	if err != nil {
		return fmt.Errorf("failed to store export name %q for %q: %w", name, nv, err)
	}

	nv.exportName = name

	return nil
}

// CreateExport takes the (CephFS) CSI-volume and instructs Ceph Mgr to create
// a new NFS-export for the volume on the Ceph managed NFS-server.
func (nv *NFSVolume) CreateExport(backend *csi.Volume) error {
	if !nv.connected {
		return fmt.Errorf("can not created export for %q: %w", nv, ErrNotConnected)
	}
	vctx := backend.GetVolumeContext()
	fs := vctx["fsName"]
	nfsCluster := vctx[ParameterCluster]
	path := vctx["subvolumePath"]
	secTypes := vctx[ParameterSecTypes]
	clients := vctx[ParameterClients]

	err := nv.setNFSCluster(nfsCluster)
	if err != nil {
		return fmt.Errorf("failed to set NFS-cluster: %w", err)
	}

	nfsa, err := nv.conn.GetNFSAdmin()
	if err != nil {
		return fmt.Errorf("failed to get NFSAdmin: %w", err)
	}

	export := nfs.CephFSExportSpec{
		FileSystemName: fs,
		ClusterID:      nfsCluster,
		PseudoPath:     nv.GetExportPath(),
		Path:           path,
	}

	if secTypes != "" {
		export.SecType = []nfs.SecType{}
		for secType := range strings.SplitSeq(secTypes, ",") {
			export.SecType = append(export.SecType, nfs.SecType(secType))
		}
	}

	if clients != "" {
		export.ClientAddr = strings.Split(clients, ",")
	}

	_, err = nfsa.CreateCephFSExport(export)
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "Export already exists"):
		// Only an idempotent retry if the existing export still points
		// at this volume's subvolume path; a friendly export name isn't
		// guaranteed unique the way a volume-ID derived one is.
		existing, infoErr := nfsa.ExportInfo(nfsCluster, nv.GetExportPath())
		if infoErr != nil {
			return fmt.Errorf("export %q already exists in NFS-cluster %q, but failed to verify "+
				"it belongs to this volume: %w", nv, nfsCluster, infoErr)
		}
		if existing.Path != path {
			return fmt.Errorf("export path %q in NFS-cluster %q is already in use by a different "+
				"volume (subvolume %q, expected %q): %w", nv.GetExportPath(), nfsCluster, existing.Path,
				path, ErrExportNameConflict)
		}

		return nil
	case strings.Contains(err.Error(), "rados: ret=-2"): // try with the old command
		log.ErrorLogMsg("going to fallback to cli, "+
			"go-ceph failed to create export %q in NFS-cluster %q: %v",
			nv, nfsCluster, err)

		break
	default: // any other error
		return fmt.Errorf("exporting %q on NFS-cluster %q failed: %w",
			nv, nfsCluster, err)
	}

	// if we get here, the API call failed, fallback to the old command

	// ceph nfs export create cephfs ${FS} ${NFS} /${EXPORT} ${SUBVOL_PATH}
	cmd := nv.createExportCommand(nfsCluster, fs, nv.GetExportPath(), path)

	_, stderr, err := util.ExecCommand(nv.ctx, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to create export %q in NFS-cluster %q"+
			"(%v): %s", nv, nfsCluster, err, stderr)
	}

	return nil
}

// DeleteExport removes the NFS-export from the Ceph managed NFS-server.
func (nv *NFSVolume) DeleteExport() error {
	if !nv.connected {
		return fmt.Errorf("can not delete export for %q: not connected", nv)
	}

	if err := nv.resolveExportName(); err != nil {
		return err
	}

	nfsCluster, err := nv.getNFSCluster()
	if err != nil {
		return fmt.Errorf("failed to identify NFS cluster: %w", err)
	}

	nfsa, err := nv.conn.GetNFSAdmin()
	if err != nil {
		return fmt.Errorf("failed to get NFSAdmin: %w", err)
	}

	err = nfsa.RemoveExport(nfsCluster, nv.GetExportPath())
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "API call not implemented"): // try with the old command
		break
	case strings.Contains(err.Error(), "Export does not exist"):
		return ErrExportNotFound
	default: // any other error
		return fmt.Errorf("failed to remove %q from NFS-cluster %q: "+
			"%w", nv, nfsCluster, err)
	}

	// if we get here, the API call failed, fallback to the old command

	// ceph nfs export delete <cluster_id> <pseudo_path>
	cmd := nv.deleteExportCommand("delete", nfsCluster)

	_, stderr, err := util.ExecCommand(nv.ctx, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to delete export %q from NFS-cluster"+
			"%q (%v): %s", nv, nfsCluster, err, stderr)
	}

	return nil
}

// SetServer stores the NFS-server name in the CephFS journal.
func (nv *NFSVolume) SetServer(server string) error {
	return nv.setAttribute(ParameterServer, server)
}

// GetServer fetches the NFS-server name from the CephFS journal.
func (nv *NFSVolume) GetServer() (string, error) {
	return nv.getAttribute(ParameterServer)
}

// SetClients updates the NFS-clients list in the NFS export.
func (nv *NFSVolume) SetClients(clients string) error {
	if !nv.connected {
		return fmt.Errorf("can not set clients for %q: %w", nv, ErrNotConnected)
	}

	if err := nv.resolveExportName(); err != nil {
		return err
	}

	nfsCluster, err := nv.getNFSCluster()
	if err != nil {
		return fmt.Errorf("failed to identify NFS cluster: %w", err)
	}

	nfsa, err := nv.conn.GetNFSAdmin()
	if err != nil {
		return fmt.Errorf("failed to get NFSAdmin: %w", err)
	}

	// Fetch current export info
	exportInfo, err := nfsa.ExportInfo(nfsCluster, nv.GetExportPath())
	if err != nil {
		return fmt.Errorf("failed to get export info for %q: %w", nv.GetExportPath(), err)
	}

	// Update the export with new clients list
	if clients != "" {
		clientAddrs := strings.Split(clients, ",")
		exportInfo.Clients = []nfs.ClientInfo{
			{
				Addresses:  clientAddrs,
				AccessType: "rw",
				Squash:     nfs.NoneSquash,
			},
		}
	}

	err = nfsa.ApplyExportInfo(nfsCluster, exportInfo)
	if err != nil {
		return fmt.Errorf("failed to update export %q with new clients: %w", nv.GetExportPath(), err)
	}

	return nil
}

// resolveExportName loads a previously persisted friendly export name
// (SetExportName) from the CephFS journal into nv.exportName.
func (nv *NFSVolume) resolveExportName() error {
	name, err := nv.getAttribute(exportNameKey)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("failed to get export name for %q: %w", nv, err)
	}

	nv.exportName = name

	return nil
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

// getAttribute fetches the attribute with the given key from the CephFS journal.
func (nv *NFSVolume) getAttribute(key string) (string, error) {
	if !nv.connected {
		return "", fmt.Errorf("can not get NFS-cluster for %q: %w", nv, ErrNotConnected)
	}

	fs := fscore.NewFileSystem(nv.conn)
	fsName, err := fs.GetFsName(nv.ctx, nv.fscID)
	if err != nil && errors.Is(err, util.ErrPoolNotFound) {
		return "", fmt.Errorf("%w for ID %x: %w", ErrFilesystemNotFound, nv.fscID, err)
	} else if err != nil {
		return "", fmt.Errorf("failed to get filesystem name for ID %x: %w", nv.fscID, err)
	}

	mdPool, err := fs.GetMetadataPool(nv.ctx, fsName)
	if err != nil && errors.Is(err, util.ErrPoolNotFound) {
		return "", fmt.Errorf("metadata pool for %q %w: %w", fsName, ErrNotFound, err)
	} else if err != nil {
		return "", fmt.Errorf("failed to get metadata pool for %q: %w", fsName, err)
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := store.VolJournal.Connect(nv.mons, fsutil.RadosNamespace, nv.cr)
	if err != nil {
		return "", fmt.Errorf("failed to connect to journal: %w", err)
	}
	defer j.Destroy()

	value, err := j.FetchAttribute(nv.ctx, mdPool, nv.objectUUID, key)
	if err != nil && errors.Is(err, util.ErrPoolNotFound) || errors.Is(err, util.ErrKeyNotFound) {
		return "", fmt.Errorf("attribute with key %q for %q %w: %w", key, nv.objectUUID, ErrNotFound, err)
	} else if err != nil {
		return "", fmt.Errorf("failed to get attribute with key %q for %q: %w", key, nv.objectUUID, err)
	}

	return value, nil
}

// setAttribute stores the attribute with the key and value in the CephFS journal.
func (nv *NFSVolume) setAttribute(key, value string) error {
	if !nv.connected {
		return fmt.Errorf("can not set NFS-cluster for %q: %w", nv, ErrNotConnected)
	}

	fs := fscore.NewFileSystem(nv.conn)
	fsName, err := fs.GetFsName(nv.ctx, nv.fscID)
	if err != nil && errors.Is(err, util.ErrPoolNotFound) {
		return fmt.Errorf("%w for ID %x: %w", ErrFilesystemNotFound, nv.fscID, err)
	} else if err != nil {
		return fmt.Errorf("failed to get filesystem name for ID %x: %w", nv.fscID, err)
	}

	mdPool, err := fs.GetMetadataPool(nv.ctx, fsName)
	if err != nil && errors.Is(err, util.ErrPoolNotFound) {
		return fmt.Errorf("metadata pool for %q %w: %w", fsName, ErrNotFound, err)
	} else if err != nil {
		return fmt.Errorf("failed to get metadata pool for %q: %w", fsName, err)
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := store.VolJournal.Connect(nv.mons, fsutil.RadosNamespace, nv.cr)
	if err != nil {
		return fmt.Errorf("failed to connect to journal: %w", err)
	}
	defer j.Destroy()

	err = j.StoreAttribute(nv.ctx, mdPool, nv.objectUUID, key, value)
	if err != nil {
		return fmt.Errorf("failed to store attribute with key %q: %w", key, err)
	}

	return nil
}

// getNFSCluster fetches the NFS-cluster name from the CephFS journal.
func (nv *NFSVolume) getNFSCluster() (string, error) {
	return nv.getAttribute(clusterNameKey)
}

// setNFSCluster stores the NFS-cluster name in the CephFS journal.
func (nv *NFSVolume) setNFSCluster(clusterName string) error {
	return nv.setAttribute(clusterNameKey, clusterName)
}
