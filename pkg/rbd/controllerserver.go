/*
Copyright 2018 The Kubernetes Authors.

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

package rbd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	oneGB = 1073741824
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.V(3).Infof("invalid create volume req: %v", req)
		return nil, err
	}
	// Check sanity of request Name, Volume Capabilities
	if len(req.Name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}

	volumeNameMutex.LockKey(req.GetName())
	defer volumeNameMutex.UnlockKey(req.GetName())

	// Need to check for already existing volume name, and if found
	// check for the requested capacity and already allocated capacity
	if exVol, err := getRBDVolumeByName(req.GetName()); err == nil {
		// Since err is nil, it means the volume with the same name already exists
		// need to check if the size of exisiting volume is the same as in new
		// request
		if exVol.VolSize >= int64(req.GetCapacityRange().GetRequiredBytes()) {
			// exisiting volume is compatible with new request and should be reused.
			// TODO (sbezverk) Do I need to make sure that RBD volume still exists?
			return &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      exVol.VolID,
					VolumeContext: req.GetParameters(),
					CapacityBytes: int64(exVol.VolSize),
				},
			}, nil
		}
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Volume with the same name: %s but with different size already exist", req.GetName()))
	}

	// TODO (sbezverk) Last check for not exceeding total storage capacity

	rbdVol, err := getRBDVolumeOptions(req.GetParameters())
	if err != nil {
		return nil, err
	}

	// Generating Volume Name and Volume ID, as according to CSI spec they MUST be different
	volName := req.GetName()
	uniqueID := uuid.NewUUID().String()
	if len(volName) == 0 {
		volName = rbdVol.Pool + "-dynamic-pvc-" + uniqueID
	}
	rbdVol.VolName = volName
	volumeID := "csi-rbd-" + uniqueID
	rbdVol.VolID = volumeID
	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(oneGB)
	if req.GetCapacityRange() != nil {
		volSizeBytes = int64(req.GetCapacityRange().GetRequiredBytes())
	}
	rbdVol.VolSize = volSizeBytes
	volSizeGB := int(volSizeBytes / 1024 / 1024 / 1024)

	// Check if there is already RBD image with requested name
	found, _, _ := rbdStatus(rbdVol, rbdVol.UserId, req.GetSecrets())
	if !found {
		// if VolumeContentSource is not nil, this request is for snapshot
		if req.VolumeContentSource != nil {
			snapshot := req.VolumeContentSource.GetSnapshot()
			if snapshot == nil {
				return nil, status.Error(codes.InvalidArgument, "Volume Snapshot cannot be empty")
			}

			snapshotID := snapshot.GetSnapshotId()
			if len(snapshotID) == 0 {
				return nil, status.Error(codes.InvalidArgument, "Volume Snapshot ID cannot be empty")
			}

			rbdSnap := &rbdSnapshot{}
			if err := loadSnapInfo(snapshotID, path.Join(PluginFolder, "controller-snap"), rbdSnap); err != nil {
				return nil, err
			}

			err = restoreSnapshot(rbdVol, rbdSnap, rbdVol.AdminId, req.GetSecrets())
			if err != nil {
				return nil, err
			}
			glog.V(4).Infof("create volume %s from snapshot %s", volName, rbdSnap.SnapName)
		} else {
			if err := createRBDImage(rbdVol, volSizeGB, rbdVol.AdminId, req.GetSecrets()); err != nil {
				if err != nil {
					glog.Warningf("failed to create volume: %v", err)
					return nil, err
				}
			}
			glog.V(4).Infof("create volume %s", volName)
		}
	}

	// Storing volInfo into a persistent file.
	if err := persistVolInfo(volumeID, path.Join(PluginFolder, "controller"), rbdVol); err != nil {
		glog.Warningf("rbd: failed to store volInfo with error: %v", err)
	}
	rbdVolumes[volumeID] = rbdVol
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			VolumeContext: req.GetParameters(),
			CapacityBytes: int64(volSizeBytes),
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.Warningf("invalid delete volume req: %v", req)
		return nil, err
	}
	// For now the image get unconditionally deleted, but here retention policy can be checked
	volumeID := req.GetVolumeId()
	volumeIDMutex.LockKey(volumeID)
	defer volumeIDMutex.UnlockKey(volumeID)
	rbdVol := &rbdVolume{}
	if err := loadVolInfo(volumeID, path.Join(PluginFolder, "controller"), rbdVol); err != nil {
		if os.IsNotExist(errors.Cause(err)) {
			// Must have been deleted already. This is not an error (idempotency!).
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, err
	}
	volName := rbdVol.VolName
	// Deleting rbd image
	glog.V(4).Infof("deleting volume %s", volName)
	if err := deleteRBDImage(rbdVol, rbdVol.AdminId, req.GetSecrets()); err != nil {
		// TODO: can we detect "already deleted" situations here and proceed?
		glog.V(3).Infof("failed to delete rbd image: %s/%s with error: %v", rbdVol.Pool, volName, err)
		return nil, err
	}
	// Removing persistent storage file for the unmapped volume
	if err := deleteVolInfo(volumeID, path.Join(PluginFolder, "controller")); err != nil {
		return nil, err
	}

	delete(rbdVolumes, volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// TODO implement this properly
	/*
		for _, cap := range req.VolumeCapabilities {
			if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
				return &csi.ValidateVolumeCapabilitiesResponse{Supported: false, Message: ""}, nil
			}
		}
	*/
	return &csi.ValidateVolumeCapabilitiesResponse{Message: "ValidateVolumeCapabilities is not implemented for csi v1.0.0"}, nil
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func getControllerServiceCapability(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

// TODO Implement this properly
func (cs *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	var cscs []*csi.ControllerServiceCapability
	supportedCscTypes := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	}
	for _, cscType := range supportedCscTypes {
		cscs = append(cscs, getControllerServiceCapability(cscType))
	}
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cscs,
	}, nil
}

func (cs *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		glog.Warningf("invalid create snapshot req: %v", req)
		return nil, err
	}

	// Check sanity of request Snapshot Name, Source Volume Id
	if len(req.Name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot Name cannot be empty")
	}
	if len(req.SourceVolumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Source Volume ID cannot be empty")
	}

	snapshotNameMutex.LockKey(req.GetName())
	defer snapshotNameMutex.UnlockKey(req.GetName())

	// Need to check for already existing snapshot name, and if found
	// check for the requested source volume id and already allocated source volume id
	if exSnap, err := getRBDSnapshotByName(req.GetName()); err == nil {
		tp, err := ptypes.TimestampProto(time.Unix(0, exSnap.CreatedAt))
		if err != nil {
			return nil, fmt.Errorf("Failed to covert creation timestamp: %v", err)
		}

		if req.SourceVolumeId == exSnap.SourceVolumeID {
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      exSnap.SizeBytes,
					SnapshotId:     exSnap.SnapID,
					SourceVolumeId: exSnap.SourceVolumeID,
					// TODO fix below properly
					CreationTime: tp,
					ReadyToUse:   true,
				},
			}, nil
		}
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Snapshot with the same name: %s but with different source volume id already exist", req.GetName()))
	}

	rbdSnap, err := getRBDSnapshotOptions(req.GetParameters())
	if err != nil {
		return nil, err
	}

	// Generating Snapshot Name and Snapshot ID, as according to CSI spec they MUST be different
	snapName := req.GetName()
	uniqueID := uuid.NewUUID().String()
	rbdVolume, err := getRBDVolumeByID(req.GetSourceVolumeId())
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Source Volume ID %s cannot found", req.GetSourceVolumeId()))
	}
	if !hasSnapshotFeature(rbdVolume.ImageFeatures) {
		return nil, fmt.Errorf("Volume(%s) has not snapshot feature(layering)", req.GetSourceVolumeId())
	}

	rbdSnap.VolName = rbdVolume.VolName
	rbdSnap.SnapName = snapName
	snapshotID := "csi-rbd-" + rbdVolume.VolName + "-snap-" + uniqueID
	rbdSnap.SnapID = snapshotID
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.SizeBytes = rbdVolume.VolSize

	err = createSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets())
	// if we already have the snapshot, return the snapshot
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() == int(syscall.EEXIST) {
					glog.Warningf("Snapshot with the same name: %s, we return this.", req.GetName())
				} else {
					glog.Warningf("failed to create snapshot: %v", err)
					return nil, err
				}
			} else {
				glog.Warningf("failed to create snapshot: %v", err)
				return nil, err
			}
		} else {
			glog.Warningf("failed to create snapshot: %v", err)
			return nil, err
		}
	} else {
		glog.V(4).Infof("create snapshot %s", snapName)
		err = protectSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets())

		if err != nil {
			err = deleteSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets())
			if err != nil {
				return nil, fmt.Errorf("snapshot is created but failed to protect and delete snapshot: %v", err)
			}
			return nil, fmt.Errorf("Snapshot is created but failed to protect snapshot")
		}
	}

	rbdSnap.CreatedAt = time.Now().UnixNano()

	// Storing snapInfo into a persistent file.
	if err := persistSnapInfo(snapshotID, path.Join(PluginFolder, "controller-snap"), rbdSnap); err != nil {
		glog.Warningf("rbd: failed to store snapInfo with error: %v", err)

		// Unprotect snapshot
		err := unprotectSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets())
		if err != nil {
			return nil, status.Error(codes.Unknown, fmt.Sprintf("This Snapshot should be removed but failed to unprotect snapshot: %s/%s with error: %v", rbdSnap.Pool, rbdSnap.SnapName, err))
		}

		// Deleting snapshot
		glog.V(4).Infof("deleting Snaphot %s", rbdSnap.SnapName)
		if err := deleteSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets()); err != nil {
			return nil, status.Error(codes.Unknown, fmt.Sprintf("This Snapshot should be removed but failed to delete snapshot: %s/%s with error: %v", rbdSnap.Pool, rbdSnap.SnapName, err))
		}

		return nil, err
	}
	rbdSnapshots[snapshotID] = rbdSnap
	tp, err := ptypes.TimestampProto(time.Unix(0, rbdSnap.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("Failed to covert creation timestamp: %v", err)
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      rbdSnap.SizeBytes,
			SnapshotId:     snapshotID,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   tp,
			ReadyToUse:     true,
		},
	}, nil
}

func (cs *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		glog.Warningf("invalid delete snapshot req: %v", req)
		return nil, err
	}

	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot ID cannot be empty")
	}
	snapshotIDMutex.LockKey(snapshotID)
	defer snapshotIDMutex.UnlockKey(snapshotID)

	rbdSnap := &rbdSnapshot{}
	if err := loadSnapInfo(snapshotID, path.Join(PluginFolder, "controller-snap"), rbdSnap); err != nil {
		return nil, err
	}

	// Unprotect snapshot
	err := unprotectSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("failed to unprotect snapshot: %s/%s with error: %v", rbdSnap.Pool, rbdSnap.SnapName, err))
	}

	// Deleting snapshot
	glog.V(4).Infof("deleting Snaphot %s", rbdSnap.SnapName)
	if err := deleteSnapshot(rbdSnap, rbdSnap.AdminId, req.GetSecrets()); err != nil {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("failed to delete snapshot: %s/%s with error: %v", rbdSnap.Pool, rbdSnap.SnapName, err))
	}

	// Removing persistent storage file for the unmapped snapshot
	if err := deleteSnapInfo(snapshotID, path.Join(PluginFolder, "controller-snap")); err != nil {
		return nil, err
	}

	delete(rbdSnapshots, snapshotID)

	return &csi.DeleteSnapshotResponse{}, nil
}

func (cs *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS); err != nil {
		glog.Warningf("invalid list snapshot req: %v", req)
		return nil, err
	}

	sourceVolumeID := req.GetSourceVolumeId()

	// TODO (sngchlko) list with token
	// TODO (#94) protect concurrent access to global data structures

	// list only a specific snapshot which has snapshot ID
	if snapshotID := req.GetSnapshotId(); len(snapshotID) != 0 {
		if rbdSnap, ok := rbdSnapshots[snapshotID]; ok {
			// if source volume ID also set, check source volume id on the cache.
			if len(sourceVolumeID) != 0 && rbdSnap.SourceVolumeID != sourceVolumeID {
				return nil, status.Error(codes.Unknown, fmt.Sprintf("Requested Source Volume ID %s is different from %s", sourceVolumeID, rbdSnap.SourceVolumeID))
			}
			tp, err := ptypes.TimestampProto(time.Unix(0, rbdSnap.CreatedAt))
			if err != nil {
				return nil, fmt.Errorf("Failed to covert creation timestamp: %v", err)
			}

			return &csi.ListSnapshotsResponse{
				Entries: []*csi.ListSnapshotsResponse_Entry{
					{
						Snapshot: &csi.Snapshot{
							SizeBytes:      rbdSnap.SizeBytes,
							SnapshotId:     rbdSnap.SnapID,
							SourceVolumeId: rbdSnap.SourceVolumeID,
							CreationTime:   tp,
							ReadyToUse:     true,
						},
					},
				},
			}, nil
		}
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Snapshot ID %s cannot found", snapshotID))
	}

	entries := []*csi.ListSnapshotsResponse_Entry{}
	for _, rbdSnap := range rbdSnapshots {
		// if source volume ID also set, check source volume id on the cache.
		if len(sourceVolumeID) != 0 && rbdSnap.SourceVolumeID != sourceVolumeID {
			continue
		}
		tp, err := ptypes.TimestampProto(time.Unix(0, rbdSnap.CreatedAt))
		if err != nil {
			return nil, fmt.Errorf("Failed to covert creation timestamp: %v", err)
		}

		entries = append(entries, &csi.ListSnapshotsResponse_Entry{
			Snapshot: &csi.Snapshot{
				SizeBytes:      rbdSnap.SizeBytes,
				SnapshotId:     rbdSnap.SnapID,
				SourceVolumeId: rbdSnap.SourceVolumeID,
				CreationTime:   tp,
				ReadyToUse:     true,
			},
		})
	}

	return &csi.ListSnapshotsResponse{
		Entries: entries,
	}, nil
}
