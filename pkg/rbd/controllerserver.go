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
	"os/exec"
	"strconv"
	"syscall"

	"github.com/ceph/ceph-csi/pkg/util"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	oneGB = 1073741824
)

// ControllerServer struct of rbd CSI driver with supported methods of CSI
// controller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer
	MetadataStore util.CachePersister
}

var (
	rbdVolumes   = map[string]*rbdVolume{}
	rbdSnapshots = map[string]*rbdSnapshot{}
)

// LoadExDataFromMetadataStore loads the rbd volume and snapshot
// info from metadata store
func (cs *ControllerServer) LoadExDataFromMetadataStore() error {
	vol := &rbdVolume{}
	// nolint
	cs.MetadataStore.ForAll("csi-rbd-vol-", vol, func(identifier string) error {
		rbdVolumes[identifier] = vol
		return nil
	})

	snap := &rbdSnapshot{}
	// nolint
	cs.MetadataStore.ForAll("csi-rbd-(.*)-snap-", snap, func(identifier string) error {
		rbdSnapshots[identifier] = snap
		return nil
	})

	klog.Infof("Loaded %d volumes and %d snapshots from metadata store", len(rbdVolumes), len(rbdSnapshots))
	return nil
}

func (cs *ControllerServer) validateVolumeReq(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.V(3).Infof("invalid create volume req: %v", protosanitizer.StripSecrets(req))
		return err
	}
	// Check sanity of request Name, Volume Capabilities
	if len(req.Name) == 0 {
		return status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		return status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}
	return nil
}

func parseVolCreateRequest(req *csi.CreateVolumeRequest) (*rbdVolume, error) {
	// TODO (sbezverk) Last check for not exceeding total storage capacity

	rbdVol, err := getRBDVolumeOptions(req.GetParameters())
	if err != nil {
		return nil, err
	}

	// Generating Volume Name and Volume ID, as according to CSI spec they MUST be different
	volName := req.GetName()
	uniqueID := uuid.NewUUID().String()
	rbdVol.VolName = volName
	volumeID := "csi-rbd-vol-" + uniqueID
	rbdVol.VolID = volumeID
	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(oneGB)
	if req.GetCapacityRange() != nil {
		volSizeBytes = req.GetCapacityRange().GetRequiredBytes()
	}
	rbdVol.VolSize = volSizeBytes

	return rbdVol, nil
}

func storeVolumeMetadata(vol *rbdVolume, cp util.CachePersister) error {
	if err := cp.Create(vol.VolID, vol); err != nil {
		klog.Errorf("failed to store metadata for volume %s: %v", vol.VolID, err)
		return err
	}

	return nil
}

// CreateVolume creates the volume in backend and store the volume metadata
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	if err := cs.validateVolumeReq(req); err != nil {
		return nil, err
	}
	volumeNameMutex.LockKey(req.GetName())
	defer func() {
		if err := volumeNameMutex.UnlockKey(req.GetName()); err != nil {
			klog.Warningf("failed to unlock mutex volume:%s %v", req.GetName(), err)
		}
	}()

	// Need to check for already existing volume name, and if found
	// check for the requested capacity and already allocated capacity
	if exVol, err := getRBDVolumeByName(req.GetName()); err == nil {
		// Since err is nil, it means the volume with the same name already exists
		// need to check if the size of existing volume is the same as in new
		// request
		if exVol.VolSize >= req.GetCapacityRange().GetRequiredBytes() {
			// existing volume is compatible with new request and should be reused.

			if err = storeVolumeMetadata(exVol, cs.MetadataStore); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			// TODO (sbezverk) Do I need to make sure that RBD volume still exists?
			return &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      exVol.VolID,
					CapacityBytes: exVol.VolSize,
					VolumeContext: req.GetParameters(),
				},
			}, nil
		}
		return nil, status.Errorf(codes.AlreadyExists, "Volume with the same name: %s but with different size already exist", req.GetName())
	}

	rbdVol, err := parseVolCreateRequest(req)
	if err != nil {
		return nil, err
	}

	volSizeGB := int(rbdVol.VolSize / 1024 / 1024 / 1024)

	// Check if there is already RBD image with requested name
	err = cs.checkRBDStatus(rbdVol, req, volSizeGB)
	if err != nil {
		return nil, err
	}

	rbdVolumes[rbdVol.VolID] = rbdVol

	if err = storeVolumeMetadata(rbdVol, cs.MetadataStore); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      rbdVol.VolID,
			CapacityBytes: rbdVol.VolSize,
			VolumeContext: req.GetParameters(),
		},
	}, nil
}

func (cs *ControllerServer) checkRBDStatus(rbdVol *rbdVolume, req *csi.CreateVolumeRequest, volSizeGB int) error {
	var err error
	// Check if there is already RBD image with requested name
	found, _, _ := rbdStatus(rbdVol, rbdVol.UserID, req.GetSecrets()) // #nosec
	if !found {
		// if VolumeContentSource is not nil, this request is for snapshot
		if req.VolumeContentSource != nil {
			if err = cs.checkSnapshot(req, rbdVol); err != nil {
				return err
			}
		} else {
			err = createRBDImage(rbdVol, volSizeGB, rbdVol.AdminID, req.GetSecrets())
			if err != nil {
				klog.Warningf("failed to create volume: %v", err)
				return err
			}

			klog.V(4).Infof("create volume %s", rbdVol.VolName)
		}
	}
	return nil
}
func (cs *ControllerServer) checkSnapshot(req *csi.CreateVolumeRequest, rbdVol *rbdVolume) error {
	snapshot := req.VolumeContentSource.GetSnapshot()
	if snapshot == nil {
		return status.Error(codes.InvalidArgument, "Volume Snapshot cannot be empty")
	}

	snapshotID := snapshot.GetSnapshotId()
	if len(snapshotID) == 0 {
		return status.Error(codes.InvalidArgument, "Volume Snapshot ID cannot be empty")
	}

	rbdSnap := &rbdSnapshot{}
	if err := cs.MetadataStore.Get(snapshotID, rbdSnap); err != nil {
		return err
	}

	err := restoreSnapshot(rbdVol, rbdSnap, rbdVol.AdminID, req.GetSecrets())
	if err != nil {
		return err
	}
	klog.V(4).Infof("create volume %s from snapshot %s", req.GetName(), rbdSnap.SnapName)
	return nil
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		klog.Warningf("invalid delete volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}
	// For now the image get unconditionally deleted, but here retention policy can be checked
	volumeID := req.GetVolumeId()
	volumeIDMutex.LockKey(volumeID)

	defer func() {
		if err := volumeIDMutex.UnlockKey(volumeID); err != nil {
			klog.Warningf("failed to unlock mutex volume:%s %v", volumeID, err)
		}
	}()

	rbdVol := &rbdVolume{}
	if err := cs.MetadataStore.Get(volumeID, rbdVol); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			klog.V(3).Infof("metadata for volume %s not found, assuming the volume to be already deleted (%v)", volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}

		return nil, err
	}

	volName := rbdVol.VolName
	// Deleting rbd image
	klog.V(4).Infof("deleting volume %s", volName)
	if err := deleteRBDImage(rbdVol, rbdVol.AdminID, req.GetSecrets()); err != nil {
		// TODO: can we detect "already deleted" situations here and proceed?
		klog.V(3).Infof("failed to delete rbd image: %s/%s with error: %v", rbdVol.Pool, volName, err)
		return nil, err
	}

	if err := cs.MetadataStore.Delete(volumeID); err != nil {
		return nil, err
	}

	delete(rbdVolumes, volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// ListVolumes returns a list of volumes stored in memory
func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {

	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_LIST_VOLUMES); err != nil {
		klog.Warningf("invalid list volume req: %v", req)
		return nil, err
	}

	//validate starting token if present
	if len(req.GetStartingToken()) > 0 {
		i, parseErr := strconv.ParseUint(req.StartingToken, 10, 32)
		if parseErr != nil {
			return nil, status.Errorf(codes.Aborted, "invalid starting token %s", parseErr.Error())
		}
		//check starting Token is greater than list of rbd volumes
		if len(rbdVolumes) < int(i) {
			return nil, status.Errorf(codes.Aborted, "invalid starting token %s", parseErr.Error())
		}
	}

	var entries []*csi.ListVolumesResponse_Entry

	for _, vol := range rbdVolumes {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vol.VolID,
				CapacityBytes: vol.VolSize,
				VolumeContext: extractStoredVolOpt(vol),
			},
		})
	}

	resp := &csi.ListVolumesResponse{
		Entries: entries,
	}

	return resp, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

// ControllerUnpublishVolume returns success response
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ControllerPublishVolume returns success response
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

// CreateSnapshot creates the snapshot in backend and stores metadata
// in store
// nolint: gocyclo
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {

	if err := cs.validateSnapshotReq(req); err != nil {
		return nil, err
	}
	snapshotNameMutex.LockKey(req.GetName())

	defer func() {
		if err := snapshotNameMutex.UnlockKey(req.GetName()); err != nil {
			klog.Warningf("failed to unlock mutex snapshot:%s %v", req.GetName(), err)
		}
	}()

	// Need to check for already existing snapshot name, and if found
	// check for the requested source volume id and already allocated source volume id
	if exSnap, err := getRBDSnapshotByName(req.GetName()); err == nil {
		if req.SourceVolumeId == exSnap.SourceVolumeID {
			if err = storeSnapshotMetadata(exSnap, cs.MetadataStore); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      exSnap.SizeBytes,
					SnapshotId:     exSnap.SnapID,
					SourceVolumeId: exSnap.SourceVolumeID,
					CreationTime: &timestamp.Timestamp{
						Seconds: exSnap.CreatedAt,
					},
					ReadyToUse: true,
				},
			}, nil
		}
		return nil, status.Errorf(codes.AlreadyExists, "Snapshot with the same name: %s but with different source volume id already exist", req.GetName())
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
		return nil, status.Errorf(codes.NotFound, "Source Volume ID %s cannot found", req.GetSourceVolumeId())
	}
	if !hasSnapshotFeature(rbdVolume.ImageFeatures) {
		return nil, fmt.Errorf("volume(%s) has not snapshot feature(layering)", req.GetSourceVolumeId())
	}

	rbdSnap.VolName = rbdVolume.VolName
	rbdSnap.SnapName = snapName
	snapshotID := "csi-rbd-" + rbdVolume.VolName + "-snap-" + uniqueID
	rbdSnap.SnapID = snapshotID
	rbdSnap.SourceVolumeID = req.GetSourceVolumeId()
	rbdSnap.SizeBytes = rbdVolume.VolSize

	err = cs.doSnapshot(rbdSnap, req.GetSecrets())
	// if we already have the snapshot, return the snapshot
	if err != nil {
		return nil, err
	}

	rbdSnap.CreatedAt = ptypes.TimestampNow().GetSeconds()

	rbdSnapshots[snapshotID] = rbdSnap

	if err = storeSnapshotMetadata(rbdSnap, cs.MetadataStore); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      rbdSnap.SizeBytes,
			SnapshotId:     snapshotID,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime: &timestamp.Timestamp{
				Seconds: rbdSnap.CreatedAt,
			},
			ReadyToUse: true,
		},
	}, nil
}

func storeSnapshotMetadata(rbdSnap *rbdSnapshot, cp util.CachePersister) error {
	if err := cp.Create(rbdSnap.SnapID, rbdSnap); err != nil {
		klog.Errorf("failed to store metadata for snapshot %s: %v", rbdSnap.SnapID, err)
		return err
	}

	return nil
}

func (cs *ControllerServer) validateSnapshotReq(req *csi.CreateSnapshotRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Warningf("invalid create snapshot req: %v", protosanitizer.StripSecrets(req))
		return err
	}

	// Check sanity of request Snapshot Name, Source Volume Id
	if len(req.Name) == 0 {
		return status.Error(codes.InvalidArgument, "Snapshot Name cannot be empty")
	}
	if len(req.SourceVolumeId) == 0 {
		return status.Error(codes.InvalidArgument, "Source Volume ID cannot be empty")
	}
	return nil
}

func (cs *ControllerServer) doSnapshot(rbdSnap *rbdSnapshot, secret map[string]string) error {
	err := createSnapshot(rbdSnap, rbdSnap.AdminID, secret)
	// if we already have the snapshot, return the snapshot
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() == int(syscall.EEXIST) {
					klog.Warningf("Snapshot with the same name: %s, we return this.", rbdSnap.SnapName)
				} else {
					klog.Warningf("failed to create snapshot: %v", err)
					return err
				}
			} else {
				klog.Warningf("failed to create snapshot: %v", err)
				return err
			}
		} else {
			klog.Warningf("failed to create snapshot: %v", err)
			return err
		}
	} else {
		klog.V(4).Infof("create snapshot %s", rbdSnap.SnapName)
		err = protectSnapshot(rbdSnap, rbdSnap.AdminID, secret)

		if err != nil {
			err = deleteSnapshot(rbdSnap, rbdSnap.AdminID, secret)
			if err != nil {
				return fmt.Errorf("snapshot is created but failed to protect and delete snapshot: %v", err)
			}
			return fmt.Errorf("snapshot is created but failed to protect snapshot")
		}
	}
	return nil
}

// DeleteSnapshot deletes the snapshot in backend and removes the
//snapshot metadata from store
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT); err != nil {
		klog.Warningf("invalid delete snapshot req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot ID cannot be empty")
	}
	snapshotIDMutex.LockKey(snapshotID)

	defer func() {
		if err := snapshotIDMutex.UnlockKey(snapshotID); err != nil {
			klog.Warningf("failed to unlock mutex snapshot:%s %v", snapshotID, err)
		}
	}()

	rbdSnap := &rbdSnapshot{}
	if err := cs.MetadataStore.Get(snapshotID, rbdSnap); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			klog.V(3).Infof("metadata for snapshot %s not found, assuming the snapshot to be already deleted (%v)", snapshotID, err)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		return nil, err
	}

	// Unprotect snapshot
	err := unprotectSnapshot(rbdSnap, rbdSnap.AdminID, req.GetSecrets())
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "failed to unprotect snapshot: %s/%s with error: %v", rbdSnap.Pool, rbdSnap.SnapName, err)
	}

	// Deleting snapshot
	klog.V(4).Infof("deleting Snaphot %s", rbdSnap.SnapName)
	if err := deleteSnapshot(rbdSnap, rbdSnap.AdminID, req.GetSecrets()); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "failed to delete snapshot: %s/%s with error: %v", rbdSnap.Pool, rbdSnap.SnapName, err)
	}

	if err := cs.MetadataStore.Delete(snapshotID); err != nil {
		return nil, err
	}

	delete(rbdSnapshots, snapshotID)

	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots lists the snapshots in the store
func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS); err != nil {
		klog.Warningf("invalid list snapshot req: %v", req)
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
				return nil, status.Errorf(codes.Unknown, "Requested Source Volume ID %s is different from %s", sourceVolumeID, rbdSnap.SourceVolumeID)
			}
			return &csi.ListSnapshotsResponse{
				Entries: []*csi.ListSnapshotsResponse_Entry{
					{
						Snapshot: &csi.Snapshot{
							SizeBytes:      rbdSnap.SizeBytes,
							SnapshotId:     rbdSnap.SnapID,
							SourceVolumeId: rbdSnap.SourceVolumeID,
							CreationTime: &timestamp.Timestamp{
								Seconds: rbdSnap.CreatedAt,
							},
							ReadyToUse: true,
						},
					},
				},
			}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Snapshot ID %s cannot found", snapshotID)

	}

	entries := []*csi.ListSnapshotsResponse_Entry{}
	for _, rbdSnap := range rbdSnapshots {
		// if source volume ID also set, check source volume id on the cache.
		if len(sourceVolumeID) != 0 && rbdSnap.SourceVolumeID != sourceVolumeID {
			continue
		}
		entries = append(entries, &csi.ListSnapshotsResponse_Entry{
			Snapshot: &csi.Snapshot{
				SizeBytes:      rbdSnap.SizeBytes,
				SnapshotId:     rbdSnap.SnapID,
				SourceVolumeId: rbdSnap.SourceVolumeID,
				CreationTime: &timestamp.Timestamp{
					Seconds: rbdSnap.CreatedAt,
				},
				ReadyToUse: true,
			},
		})
	}

	return &csi.ListSnapshotsResponse{
		Entries: entries,
	}, nil
}
