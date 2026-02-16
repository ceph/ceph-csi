/*
Copyright 2025 The Ceph-CSI Authors.

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

package nodeserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/nvmeof"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// NodeServer struct of ceph nvmeof driver with supported methods of CSI
// node server spec.
type NodeServer struct {
	csicommon.DefaultNodeServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error.
	volumeLocks *util.IDLocker

	initiator nvmeof.NVMeInitiator

	// securityKeys manages DH-CHAP and PSK\TLS keys
	securityKeys nvmeof.SecurityKeyManager

	// node ID of this node server
	nodeID string
}

// ConnectionInfo holds NVMe-oF connection details.
type NvmeConnectionInfo struct {
	SubsystemNQN  string                   `json:"subsystemNQN"`
	NamespaceID   uint32                   `json:"namespaceID"`
	NamespaceUUID string                   `json:"namespaceUUID"`
	Listeners     []nvmeof.ListenerDetails `json:"listeners"`
	HostNQN       string                   `json:"hostNQN,omitempty"`
	Transport     string                   `json:"transport"`
	DhchapMode    string                   `json:"dhchapMode,omitempty"`
}

// stageTransaction struct represents the state a transaction was when it either completed
// or failed
// this transaction state can be used to rollback the transaction.
type stageTransaction struct {
	// isStagePathCreated represents whether the mount path to stage the volume on was created or not
	isStagePathCreated bool
	// isMounted represents if the volume was mounted or not
	isMounted bool
	// devicePath represents the path where nvmeof device is mapped
	devicePath string
}

const (
	nvmeofSubsystemNQN  = "SubsystemNQN"
	nvmeofNamespaceID   = "NamespaceID"
	nvmeofNamespaceUUID = "NamespaceUUID"
	nvmeofListeners     = "listeners"
	nvmeofHostNQN       = "HostNQN"
	defaultTransport    = "tcp"
	nvmeofdhchapMode    = "dhchapMode"
	authenticationKMSID = "authenticationKMSID"
)

// NewNodeServer initialize a node server for ceph CSI driver.
func NewNodeServer(
	d *csicommon.CSIDriver,
	nodeID,
	t string,
) (*NodeServer, error) {
	// Create NVMe initiator
	nvmeInitiator := nvmeof.NewNVMeInitiator()
	ns := &NodeServer{
		DefaultNodeServer: *csicommon.NewDefaultNodeServer(d, t, "", map[string]string{}, map[string]string{}),
		initiator:         nvmeInitiator,
		volumeLocks:       util.NewIDLocker(),
		securityKeys:      nil, // Initialize lazily when needed
		nodeID:            nodeID,
	}

	// Load nvme kernel modules
	if err := nvmeInitiator.LoadKernelModules(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load NVMe kernel modules: %w", err)
	}

	return ns, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (ns *NodeServer) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

// NodeStageVolume mounts the volume to a staging path on the node.
func (ns *NodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}
	volumeID := req.GetVolumeId()
	if acquired := ns.volumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer ns.volumeLocks.Release(volumeID)

	volumeContext := req.GetVolumeContext()
	stagingTargetPath := getStagingTargetPath(req)

	// Check if stagingPath is already mounted
	isNotMnt, err := isNotMountPoint(ns.Mounter, stagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else if !isNotMnt {
		log.DebugLog(ctx, "nvmeof: volume %s is already mounted to %s, skipping", volumeID, stagingTargetPath)

		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Parse volume context
	connectionInfo, err := ns.getNvmeConnection(volumeContext, req.GetPublishContext())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume context: %v", err)
	}
	// Get authentication KMS ID from volume context.
	// can be empty! it will take default KMS in that case
	authKMSID := volumeContext[authenticationKMSID]

	// perform the actual staging and if this fails, have undoStagingTransaction
	// cleans up for us
	txn, err := ns.stageTransaction(ctx, req, connectionInfo, authKMSID)
	defer func() {
		if err != nil {
			ns.undoStagingTransaction(ctx, req, txn)
		}
	}()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "nvmeof: successfully staged volume %s to stagingTargetPath %s",
		volumeID, stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodePublishVolume mounts the volume mounted to the staging target path to the target
// path.
func (ns *NodeServer) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	err := util.ValidateNodePublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	// Validate that the pod's service account is allowed to mount this volume
	err = util.ValidateServiceAccountRestriction(ctx,
		req.GetPublishContext()[util.PublishContextServiceAccount],
		req.GetVolumeContext()[util.VolumeContextServiceAccountKey],
		req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	targetPath := req.GetTargetPath()
	stagingPath := req.GetStagingTargetPath()
	volID := req.GetVolumeId()
	isBlock := req.GetVolumeCapability().GetBlock() != nil

	// Add volume ID to staging path (this is where NodeStage mounted it)
	stagingPath = stagingPath + "/" + volID

	if acquired := ns.volumeLocks.TryAcquire(targetPath); !acquired {
		log.ErrorLog(ctx, util.TargetPathOperationAlreadyExistsFmt, targetPath)

		return nil, status.Errorf(codes.Aborted, util.TargetPathOperationAlreadyExistsFmt, targetPath)
	}
	defer ns.volumeLocks.Release(targetPath)

	// Check if that target path exists properly
	isMnt, err := ns.createTargetMountPath(ctx, targetPath, isBlock)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Publish Path
	err = ns.mountVolume(ctx, stagingPath, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "nvmeof: successfully mounted stagingPath %s to targetPath %s", stagingPath, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path.
func (ns *NodeServer) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	err := util.ValidateNodeUnpublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}
	targetPath := req.GetTargetPath()

	if acquired := ns.volumeLocks.TryAcquire(targetPath); !acquired {
		log.ErrorLog(ctx, util.TargetPathOperationAlreadyExistsFmt, targetPath)

		return nil, status.Errorf(codes.Aborted, util.TargetPathOperationAlreadyExistsFmt, targetPath)
	}
	defer ns.volumeLocks.Release(targetPath)

	isMnt, err := ns.Mounter.IsMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// targetPath has already been deleted
			log.DebugLog(ctx, "targetPath: %s has already been deleted", targetPath)

			return &csi.NodeUnpublishVolumeResponse{}, nil
		}

		return nil, status.Error(codes.NotFound, err.Error())
	}
	if !isMnt {
		if err = os.Remove(targetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err = ns.Mounter.Unmount(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = os.Remove(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "nvmeof: successfully unbound volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path.
func (ns *NodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnstageVolumeRequest(req); err != nil {
		return nil, err
	}

	volumeID := req.GetVolumeId()
	if acquired := ns.volumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer ns.volumeLocks.Release(volumeID)

	stagingTargetPath := getStagingTargetPath(req)

	isMnt, err := ns.Mounter.IsMountPoint(stagingTargetPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		// Continue on ENOENT errors as we may still have the nvmeof device mapped
		isMnt = false
	}
	if isMnt {
		// Unmounting the image
		err = ns.Mounter.Unmount(stagingTargetPath)
		if err != nil {
			log.ExtendedLog(ctx, "failed to unmount stagingPath: %s with error: %v", stagingTargetPath, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
		log.DebugLog(ctx, "successfully unmounted volume (%s) from staging path (%s)",
			req.GetVolumeId(), stagingTargetPath)
	}

	if err = os.Remove(stagingTargetPath); err != nil {
		// Any error is critical as Staging path is expected to be empty by Kubernetes, it otherwise
		// keeps invoking Unstage. Hence any errors removing files within this path is a critical
		// error
		if !os.IsNotExist(err) {
			log.ErrorLog(ctx, "failed to remove staging target path (%s): (%v)", stagingTargetPath, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	log.DebugLog(ctx, "successfully removed staging path (%s)", stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeExpandVolume resizes nvmeof volumes (namespace).
func (ns *NodeServer) NodeExpandVolume(
	ctx context.Context,
	req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if err := util.ValidateVolumeID(volumeID, true); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Block mode - nothing to do
	if req.GetVolumeCapability() != nil && req.GetVolumeCapability().GetBlock() != nil {
		log.DebugLog(ctx, "nvmeof: block mode volume, no filesystem resize needed for %s", volumeID)

		return &csi.NodeExpandVolumeResponse{}, nil
	}

	// Get staging path
	volumePath := req.GetStagingTargetPath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path must be provided")
	}

	if acquired := ns.volumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer ns.volumeLocks.Release(volumeID)

	mountPath := volumePath + "/" + volumeID

	// Find device from mount (no metadata needed!)
	devicePath, err := ns.getDeviceFromMount(ctx, mountPath)
	if err != nil {
		log.ErrorLog(ctx, "failed to find device for mount %s: %v", mountPath, err)

		return nil, status.Errorf(codes.Internal, "failed to find device: %v", err)
	}

	log.DebugLog(ctx, "nvmeof: resizing filesystem on device %s at mount path %s", devicePath, mountPath)

	resizer := mount.NewResizeFs(utilexec.New())
	var ok bool
	ok, err = resizer.Resize(devicePath, mountPath)
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"nvmeof: resize failed on path %s, error: %v", req.GetVolumePath(), err)
	}
	log.DebugLog(ctx, "nvmeof: successfully resized filesystem for volume %s", volumeID)

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *NodeServer) mountVolume(ctx context.Context, stagingPath string, req *csi.NodePublishVolumeRequest) error {
	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	mountOptions := []string{"bind"}
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	targetPath := req.GetTargetPath()

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	log.DebugLog(ctx, "target %v\nisBlock %v\nfstype %v\nstagingPath %v\nreadonly %v\nmountflags %v\n",
		targetPath, isBlock, fsType, stagingPath, readOnly, mountOptions)

	if readOnly {
		mountOptions = append(mountOptions, "ro")
	}
	if err := util.Mount(ns.Mounter, stagingPath, targetPath, fsType, mountOptions); err != nil {
		return fmt.Errorf("failed to mount volume %s at targetPath %s: %w", req.GetVolumeId(), targetPath, err)
	}

	return nil
}

// stageTransaction.
func (ns *NodeServer) stageTransaction(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	connectionInfo *NvmeConnectionInfo,
	authKMSID string,
) (*stageTransaction, error) {
	transaction := &stageTransaction{}

	var err error

	// perform the actual staging
	devicePath, err := ns.connectToSubsystem(ctx, req.GetVolumeId(), connectionInfo, req.GetSecrets(), authKMSID)
	if err != nil {
		return transaction, err
	}

	transaction.devicePath = devicePath

	stagingTargetPath := getStagingTargetPath(req)
	isBlock := req.GetVolumeCapability().GetBlock() != nil

	err = ns.createStageMountPoint(ctx, stagingTargetPath, isBlock)
	if err != nil {
		return transaction, err
	}
	transaction.isStagePathCreated = true

	// Mount the device to staging path
	err = ns.mountVolumeToStagePath(ctx, req, stagingTargetPath, devicePath)
	if err != nil {
		return transaction, err
	}
	transaction.isMounted = true

	return transaction, nil
}

func (ns *NodeServer) undoStagingTransaction(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	transaction *stageTransaction,
) {
	var err error

	stagingTargetPath := getStagingTargetPath(req)
	if transaction.isMounted {
		err = ns.Mounter.Unmount(stagingTargetPath)
		if err != nil {
			log.ErrorLog(ctx, "failed to unmount stagingtargetPath: %s with error: %v", stagingTargetPath, err)

			return
		}
	}

	// remove the file/directory created on staging path
	if transaction.isStagePathCreated {
		err = os.Remove(stagingTargetPath)
		if err != nil {
			log.ErrorLog(ctx, "failed to remove stagingtargetPath: %s with error: %v", stagingTargetPath, err)
			// continue on failure to disconnect the image
		}
	}
}

// createTargetMountPath check if the mountPath already has something mounted
// on it. If not, the directory (for a filesystem volume) will be created.
//
// This function returns 'true' in case there is something mounted on the given
// path already, 'false' when the path exists, but nothing is mounted there.
func (ns *NodeServer) createTargetMountPath(ctx context.Context, mountPath string, isBlock bool) (bool, error) {
	isMnt, err := ns.Mounter.IsMountPoint(mountPath)
	if err == nil {
		return isMnt, nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("path %q exists, but detecting it as mount point failed: %w", mountPath, err)
	}

	// filesystem volume needs a directory
	if !isBlock {
		// Create a mountpath directory
		if err = util.CreateMountPoint(mountPath); err != nil {
			return false, fmt.Errorf("failed to create mount path %q: %w", mountPath, err)
		}

		return false, nil
	}

	// block volume checks
	// #nosec
	pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0o750)
	if err != nil {
		log.ErrorLog(ctx, "Failed to create mountPath:%s with error: %v", mountPath, err)

		return false, fmt.Errorf("failed to create mount file %q: %w", mountPath, err)
	}
	if err = pathFile.Close(); err != nil {
		log.ErrorLog(ctx, "Failed to close mountPath:%s with error: %v", mountPath, err)

		return false, fmt.Errorf("failed to close mount file %q: %w", mountPath, err)
	}

	return false, nil
}

// isNotMountPoint checks whether MountPoint does not exist and
// also discards error indicating mountPoint does not exist.
func isNotMountPoint(mounter mount.Interface, stagingTargetPath string) (bool, error) {
	isMnt, err := mounter.IsMountPoint(stagingTargetPath)
	if os.IsNotExist(err) {
		err = nil
	}

	return !isMnt, err
}

// getNvmeConnection extracts and validates NVMe-oF connection parameters from volume context and publish context.
// It returns connection information including subsystem NQN, namespace details, listeners,
// and host NQN, or an error if validation fails.
func (ns *NodeServer) getNvmeConnection(volumeContext, publishContext map[string]string) (*NvmeConnectionInfo, error) {
	subsystemNQN, ok := volumeContext[nvmeofSubsystemNQN]
	if !ok || subsystemNQN == "" {
		return nil, errors.New("missing subsystem NQN in volume context")
	}

	namespaceIDStr, ok := volumeContext[nvmeofNamespaceID]
	if !ok || namespaceIDStr == "" {
		return nil, errors.New("missing namespace ID in volume context")
	}

	namespaceID, err := strconv.ParseUint(namespaceIDStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace ID: %w", err)
	}

	namespaceUUID, ok := volumeContext[nvmeofNamespaceUUID]
	if !ok || namespaceUUID == "" {
		return nil, errors.New("missing namespace UUID in volume context")
	}

	// Parse listeners from JSON
	listenersJSON, ok := volumeContext[nvmeofListeners]
	if !ok || listenersJSON == "" {
		return nil, errors.New("missing listeners in volume context")
	}

	var listeners []nvmeof.ListenerDetails
	if err := json.Unmarshal([]byte(listenersJSON), &listeners); err != nil {
		return nil, fmt.Errorf("failed to parse listeners JSON: %w", err)
	}

	if len(listeners) == 0 {
		return nil, errors.New("no listeners found in volume context")
	}

	hostNQN, ok := publishContext[nvmeofHostNQN]
	if !ok || hostNQN == "" {
		return nil, errors.New("missing host NQN in publish context")
	}
	dhchapMode, ok := volumeContext[nvmeofdhchapMode]
	if !ok || dhchapMode == "" || dhchapMode == "none" {
		dhchapMode = ""
	}

	return &NvmeConnectionInfo{
		SubsystemNQN:  subsystemNQN,
		NamespaceID:   uint32(namespaceID),
		NamespaceUUID: namespaceUUID,
		Listeners:     listeners,
		HostNQN:       hostNQN,
		Transport:     defaultTransport,
		DhchapMode:    dhchapMode,
	}, nil
}

// connectToSubsystem connects to the NVMe-oF subsystem and returns device path.
func (ns *NodeServer) connectToSubsystem(
	ctx context.Context,
	volumeID string,
	info *NvmeConnectionInfo,
	secrets map[string]string,
	authKMSID string,
) (string, error) {
	// Create connect request
	connectReq := &nvmeof.ConnectRequest{
		SubsystemNQN: info.SubsystemNQN,
		Transport:    info.Transport,
		HostNQN:      info.HostNQN,
	}

	// Setup DH-CHAP authentication if required
	if info.DhchapMode != nvmeof.DHCHAPEmpty && info.DhchapMode != nvmeof.DHCHAPModeNone {
		if err := ns.setupDHCHAPAuth(ctx, volumeID, info, secrets, authKMSID, connectReq); err != nil {
			return "", err
		}
	}

	// Resolve listener IP addresses
	validListeners, err := nvmeof.ResolveListeners(ctx, info.Listeners)
	if err != nil {
		return "", fmt.Errorf("failed to setup listeners: %w", err)
	}

	connectReq.Listeners = validListeners
	// Connect to subsystem
	_, err = ns.initiator.ConnectSubsystem(ctx, connectReq)
	if err != nil {
		return "", fmt.Errorf("failed to connect to subsystem: %w", err)
	}

	// Get namespace device path by uuid
	devicePath, err := ns.initiator.GetNamespaceDeviceByUUID(ctx, info.NamespaceUUID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get namespace uuid %s: %v", info.NamespaceUUID, err)

		return "", fmt.Errorf("failed to get namespace device: %w", err)
	}

	return devicePath, nil
}

// createStageMountPoint creates a file for block volumes or directory for filesystem volumes.
func (ns *NodeServer) createStageMountPoint(ctx context.Context, mountPath string, isBlock bool) error {
	if isBlock {
		// Create file for block volume bind mount
		// #nosec:G304, intentionally creating file mountPath, not a security issue
		pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			log.ErrorLog(ctx, "failed to create mountPath:%s with error: %v", mountPath, err)

			return fmt.Errorf("failed to create mount file %q: %w", mountPath, err)
		}
		if err = pathFile.Close(); err != nil {
			log.ErrorLog(ctx, "failed to close mountPath:%s with error: %v", mountPath, err)

			return fmt.Errorf("failed to close mount file %q: %w", mountPath, err)
		}

		return nil
	}
	// Create directory for filesystem mount
	err := os.Mkdir(mountPath, 0o750)
	if err != nil {
		if !os.IsExist(err) {
			log.ErrorLog(ctx, "failed to create mountPath:%s with error: %v", mountPath, err)

			return fmt.Errorf("failed to create mount path %q: %w", mountPath, err)
		}
	}

	return nil
}

// mountVolumeToStagePath mounts the device to the staging path.
func (ns *NodeServer) mountVolumeToStagePath(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	stagingPath, devicePath string,
) error {
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.Mounter, Exec: utilexec.New()}
	volumeCap := req.GetVolumeCapability()
	isBlock := req.GetVolumeCapability().GetBlock() != nil

	// Start with base mount options
	mountOptions := []string{}

	// Add user-specified mount flags and handle read-only
	mountOptions = csicommon.ConstructMountOptions(mountOptions, volumeCap)

	if isBlock {
		// For block volumes - bind mount
		log.DebugLog(ctx, "nvmeof: bind mounting block device %s to %s", devicePath, stagingPath)
		mountOptions = append(mountOptions, "bind")
		err := diskMounter.MountSensitiveWithoutSystemd(devicePath, stagingPath, "", mountOptions, nil)
		if err != nil {
			log.ErrorLog(ctx, "failed to bind mount device %s to %s: %v", devicePath, stagingPath, err)
		}

		return err
	}
	// For filesystem volumes - format and mount
	fsType := volumeCap.GetMount().GetFsType()
	if fsType == "" {
		fsType = "ext4"
	}

	// Add filesystem-specific default options
	if fsType == "xfs" {
		mountOptions = append(mountOptions, "nouuid")
	}
	mountOptions = append(mountOptions, "_netdev")
	log.DebugLog(ctx, "nvmeof: mounting device %s to %s with fsType %s", devicePath, stagingPath, fsType)
	err := diskMounter.FormatAndMount(devicePath, stagingPath, fsType, mountOptions)
	if err != nil {
		log.ErrorLog(ctx,
			"failed to mount device path (%s) to staging path (%s) for volume "+
				"(%s) error: %s Check dmesg logs if required.",
			devicePath,
			stagingPath,
			req.GetVolumeId(),
			err)
	}

	return err
}

// getStagingTargetPath concats either NodeStageVolumeRequest's or
// NodeUnstageVolumeRequest's target path with the volumeID.
func getStagingTargetPath(req interface{}) string {
	switch vr := req.(type) {
	case *csi.NodeStageVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	case *csi.NodeUnstageVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	}

	return ""
}

// getDeviceFromMount finds the device path for a given mount path.
func (ns *NodeServer) getDeviceFromMount(ctx context.Context, mountPath string) (string, error) {
	mountPoints, err := ns.Mounter.List()
	if err != nil {
		return "", fmt.Errorf("failed to list mounts: %w", err)
	}

	for _, mp := range mountPoints {
		if mp.Path == mountPath {
			log.DebugLog(ctx, "found device %s for mount path %s", mp.Device, mountPath)

			return mp.Device, nil
		}
	}

	return "", fmt.Errorf("no mount found for path %s", mountPath)
}
