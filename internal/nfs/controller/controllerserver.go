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
	"errors"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ceph/ceph-csi/internal/cephfs"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/journal"
	nfs "github.com/ceph/ceph-csi/internal/nfs/types"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// Server struct of CEPH CSI driver with supported methods of CSI controller
// server spec.
type Server struct {
	csi.UnimplementedControllerServer

	// backendServer handles the CephFS requests
	backendServer *cephfs.ControllerServer
}

// NewControllerServer initialize a controller server for ceph CSI driver.
func NewControllerServer(d *csicommon.CSIDriver) *Server {
	// global instance of the volume journal, yuck
	store.VolJournal = journal.NewCSIVolumeJournalWithNamespace(d.GetInstanceID(), fsutil.RadosNamespace)
	store.SnapJournal = journal.NewCSISnapshotJournalWithNamespace(d.GetInstanceID(), fsutil.RadosNamespace)

	return &Server{
		backendServer: cephfs.NewControllerServer(d),
	}
}

// ControllerGetCapabilities uses the CephFS backendServer to return the
// capabilities that were set in the Driver.Run() function.
func (cs *Server) ControllerGetCapabilities(
	ctx context.Context,
	req *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	return cs.backendServer.ControllerGetCapabilities(ctx, req)
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *Server) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return cs.backendServer.ValidateVolumeCapabilities(ctx, req)
}

// CreateVolume creates the backing subvolume and on any error cleans up any
// created entities.
func (cs *Server) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	// nfs does not supports shallow snapshots
	req.Parameters["backingSnapshot"] = "false"
	res, err := cs.backendServer.CreateVolume(ctx, req)
	if err != nil {
		return nil, err
	}

	backend := res.GetVolume()

	log.DebugLog(ctx, "CephFS volume created: %s", backend.GetVolumeId())

	secret := req.GetSecrets()
	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		log.ErrorLog(ctx, "failed to retrieve admin credentials: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	nfsVolume, err := nfs.NewNFSVolume(ctx, backend.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = nfsVolume.Connect(cr)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to connect: %v", err)
	}
	defer nfsVolume.Destroy()

	err = nfsVolume.CreateExport(backend)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to create export: %v", err)
	}

	log.DebugLog(ctx, "published NFS-export: %s", nfsVolume)

	// volume has been exported over NFS, set the "share" parameter to
	// allow mounting
	backend.VolumeContext["share"] = nfsVolume.GetExportPath()

	// apply MutableParameters through ControllerModifyVolume()
	if req.GetMutableParameters() != nil {
		log.DebugLog(ctx, "modifying parameters: %v", req.GetMutableParameters())

		_, err = cs.ControllerModifyVolume(ctx,
			&csi.ControllerModifyVolumeRequest{
				VolumeId:          backend.GetVolumeId(),
				Secrets:           req.GetSecrets(),
				MutableParameters: req.GetMutableParameters(),
			})
		if err != nil {
			return nil, err
		}
	}

	return &csi.CreateVolumeResponse{Volume: backend}, nil
}

// DeleteVolume deletes the volume in backend and its reservation.
func (cs *Server) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if err := util.ValidateVolumeID(volumeID, true); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	secret := req.GetSecrets()
	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		log.ErrorLog(ctx, "failed to retrieve admin credentials: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	nfsVolume, err := nfs.NewNFSVolume(ctx, volumeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = nfsVolume.Connect(cr)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to connect: %v", err)
	}
	defer nfsVolume.Destroy()

	err = nfsVolume.DeleteExport()
	// if the export does not exist, continue with deleting the backend volume
	if err != nil && !errors.Is(err, nfs.ErrNotFound) {
		return nil, status.Errorf(codes.InvalidArgument, "failed to delete export: %v", err)
	}

	log.DebugLog(ctx, "NFS-export %q has been deleted", nfsVolume)

	return cs.backendServer.DeleteVolume(ctx, req)
}

// ControllerPublishVolume delegates to the CephFS backend to read the service
// account restriction metadata from the backing CephFS subvolume and pass it
// to the node via publish context.
func (cs *Server) ControllerPublishVolume(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	return cs.backendServer.ControllerPublishVolume(ctx, req)
}

// ControllerUnpublishVolume is a no-op for the NFS CSI driver.
func (cs *Server) ControllerUnpublishVolume(
	ctx context.Context,
	req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ControllerExpandVolume calls the backend (CephFS) procedure to expand the
// volume. There is no interaction with the NFS-server needed to publish the
// new size.
func (cs *Server) ControllerExpandVolume(
	ctx context.Context,
	req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {
	return cs.backendServer.ControllerExpandVolume(ctx, req)
}

// CreateSnapshot calls the backend (CephFS) procedure to create snapshot.
// There is no interaction with the NFS-server needed for snapshot creation.
func (cs *Server) CreateSnapshot(
	ctx context.Context,
	req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {
	return cs.backendServer.CreateSnapshot(ctx, req)
}

// DeleteSnapshot calls the backend (CephFS) procedure to delete snapshot.
// There is no interaction with the NFS-server needed for snapshot creation.
func (cs *Server) DeleteSnapshot(
	ctx context.Context,
	req *csi.DeleteSnapshotRequest,
) (*csi.DeleteSnapshotResponse, error) {
	return cs.backendServer.DeleteSnapshot(ctx, req)
}

// ControllerModifyVolume adjusts parameters after a volume has been created.
// The new parameters from the [mutable_parameters] attribute are stored in
// the [NFSVolume] object (which stores the parameters in the volumes OMAP).
func (cs *Server) ControllerModifyVolume(
	ctx context.Context,
	req *csi.ControllerModifyVolumeRequest,
) (*csi.ControllerModifyVolumeResponse, error) {
	secret := req.GetSecrets()
	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		log.ErrorLog(ctx, "failed to retrieve admin credentials: %v", err)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	nfsVolume, err := nfs.NewNFSVolume(ctx, req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = nfsVolume.Connect(cr)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to connect: %v", err)
	}
	defer nfsVolume.Destroy()

	if servername, ok := req.GetMutableParameters()[nfs.ParameterServer]; ok {
		err := nfsVolume.SetServer(servername)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.ControllerModifyVolumeResponse{}, nil
}
