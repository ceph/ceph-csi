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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/nvmeof"
	nvmeoferrors "github.com/ceph/ceph-csi/internal/nvmeof/errors"
	"github.com/ceph/ceph-csi/internal/rbd"
	rbdutil "github.com/ceph/ceph-csi/internal/rbd"
	rbddriver "github.com/ceph/ceph-csi/internal/rbd/driver"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"
)

type Server struct {
	csi.UnimplementedControllerServer

	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	volumeLocks *util.IDLocker

	// hostLocks protects against concurrently adding hosts to, and removing hosts from
	// the gateway during ControllerPublishVolume and ControllerUnpublishVolume.
	hostLocks *util.IDLocker

	// subsystemLocks prevents concurrent calls from deleting an "empty but not empty
	// anymore" subsystem (and listeners).
	subsystemLocks *util.IDLocker

	// backendServer handles the RBD requests
	backendServer *rbd.ControllerServer
}

// NewControllerServer initialize a controller server for nvmeof CSI driver.
func NewControllerServer(d *csicommon.CSIDriver) (*Server, error) {
	return &Server{
		volumeLocks:    util.NewIDLocker(),
		hostLocks:      util.NewIDLocker(),
		subsystemLocks: util.NewIDLocker(),
		backendServer:  rbddriver.NewControllerServer(d),
	}, nil
}

// ControllerGetCapabilities uses the RBD backendServer to return the
// capabilities that were set in the Driver.Run() function.
func (cs *Server) ControllerGetCapabilities(
	ctx context.Context,
	req *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	return cs.backendServer.ControllerGetCapabilities(ctx, req)
}

// ValidateControllerServiceRequest uses the RBD backendServer driver field.
// It checks whether the controller service request is valid.
func (cs *Server) ValidateControllerServiceRequest(capability csi.ControllerServiceCapability_RPC_Type) error {
	return cs.backendServer.Driver.ValidateControllerServiceRequest(capability)
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *Server) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return cs.backendServer.ValidateVolumeCapabilities(ctx, req)
}

// CreateVolume creates a new RBD volume and exposes it through NVMe-oF Gateway.
func (cs *Server) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	// TODO - need to check and modify the request to ensure it has the required fields NvmeOF supports (like nfs does)
	// TODO - move all hardcoded strings to constants

	// Step 0: Validate request
	if err := validateCreateVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
	}

	// prevent concurrent requests for the same volume
	if acquired := cs.volumeLocks.TryAcquire(req.GetName()); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, req.GetName())

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.volumeLocks.Release(req.GetName())

	// Step 1: Create RBD volume through backend. if exists, it is ok.
	res, err := cs.backendServer.CreateVolume(ctx, req)
	if err != nil {
		log.ErrorLog(ctx, "failed to create RBD volume: %v", err)

		return nil, err
	}

	backend := res.GetVolume()
	volumeID := backend.GetVolumeId()
	volumeContext := backend.GetVolumeContext()
	defer func() {
		// skip cleanup if there was no error
		if err == nil {
			return
		}

		_, cleanupErr := cs.backendServer.DeleteVolume(
			ctx,
			&csi.DeleteVolumeRequest{
				VolumeId: volumeID,
				Secrets:  req.GetSecrets(),
			},
		)
		if cleanupErr != nil {
			log.ErrorLog(ctx, "failed to cleanup volume %q: %v", volumeID, cleanupErr)
		}
	}()

	rbdImageName := volumeContext["imageName"]
	rbdPoolName := volumeContext["pool"]
	// can be empty. if it was defined in config-map the rbd csi driver would have set it already
	rbdRadosNameSpace := res.GetVolume().GetVolumeContext()["radosNamespace"]

	// Step 2: Setup NVMe-oF resources
	var nvmeofData *nvmeof.NVMeoFVolumeData
	nvmeofData, err = cs.createNVMeoFResources(ctx, req, rbdPoolName, rbdRadosNameSpace, rbdImageName)
	if err != nil {
		log.ErrorLog(ctx, "NVMe-oF resource setup failed for volumeID %s: %v", volumeID, err)

		return nil, status.Errorf(codes.Internal, "NVMe-oF setup failed: %v", err)
	}
	defer func() {
		// Cleanup NVMe-oF resources on subsequent errors.
		// only if there was an error and nvmeofData is not nil, it means resources were created.
		if err != nil && nvmeofData != nil {
			log.DebugLog(ctx, "Cleaning up NVMe-oF resources for volume %q due to error: %v", volumeID, err)
			cleanupErr := cs.cleanupNVMeoFResources(ctx, nvmeofData)
			if cleanupErr != nil {
				log.ErrorLog(ctx, "failed to cleanup NVMe-oF resources for volume  %q: %v", volumeID, cleanupErr)
			}
		}
	}()

	// step 3: Populate volume context for NodeServer
	err = populateVolumeContext(backend, nvmeofData)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to populate volume context: %v", err)
	}

	// Step 4: Store NVMe-oF metadata in the volume context
	err = cs.storeNVMeoFMetadata(ctx, req, volumeID, nvmeofData)
	if err != nil {
		return nil, err // Error already formatted with proper status code
	}

	return &csi.CreateVolumeResponse{Volume: backend}, nil
}

// DeleteVolume removes the volume from the gateaway and deletes the RBD-image from the backend.
func (cs *Server) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if err := util.ValidateVolumeID(volumeID, true); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// prevent concurrent requests for the same volume
	if acquired := cs.volumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.volumeLocks.Release(volumeID)

	// Get NVMe-oF metadata for cleanup
	nvmeofData, err := cs.getNVMeoFMetadata(ctx, req.GetSecrets(), volumeID)
	if err != nil {
		log.DebugLog(ctx, "No NVMe-oF metadata found, skipping NVMe-oF cleanup: %v", err)
	} else {
		// Clean up NVMe-oF resources
		if err := cs.cleanupNVMeoFResources(ctx, nvmeofData); err != nil {
			log.ErrorLog(ctx, "NVMe-oF cleanup failed (continuing with RBD deletion): %v", err)

			return nil, status.Errorf(codes.Internal, "NVMe-oF cleanup failed: %v", err)
		}
	}
	// Delete RBD volume through backend

	return cs.backendServer.DeleteVolume(ctx, req)
}

// ControllerPublishVolume handles the publishing of a volume (run Add host GRPC).
func (cs *Server) ControllerPublishVolume(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	// Validate request
	if err := validatePublishVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
	}

	volumeID := req.GetVolumeId()
	if acquired := cs.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.volumeLocks.Release(volumeID)

	nodeID := req.GetNodeId()
	if acquired := cs.hostLocks.TryAcquire(nodeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.hostLocks.Release(nodeID)

	// Publish NVMe-oF resources
	hostNqn, err := publishResources(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish resources: %v", err)
	}

	// populate publish context
	publishContext := populatePublishContext(req, hostNqn)

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishContext,
	}, nil
}

func (cs *Server) ControllerUnpublishVolume(
	ctx context.Context,
	req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {
	// Validate request
	if err := util.ValidateControllerUnpublishVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
	}

	volumeID := req.GetVolumeId()
	if acquired := cs.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.volumeLocks.Release(volumeID)

	nodeID := req.GetNodeId()
	if acquired := cs.hostLocks.TryAcquire(nodeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.hostLocks.Release(nodeID)

	// Since ControllerUnpublishVolume doesn't receive volume context,
	// we need to retrieve it from the volume metadata stored during CreateVolume
	secrets := req.GetSecrets()
	if secrets == nil {
		secretName, secretNamespace, err := util.GetControllerPublishSecretRef(req.GetVolumeId(), util.RBDType)
		if err != nil {
			log.WarningLog(ctx, "controller publish secret not found: %v", err)

			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}

		secrets, err = k8s.GetSecret(secretName, secretNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get controller publish secret from k8s: %w", err)
		}
	}

	nvmeofData, err := cs.getNVMeoFMetadata(ctx, secrets, volumeID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get NVMe-oF metadata for volumeID %s: %v", volumeID, err)

		return nil, nvmeoferrors.ToGRPCError(err)
	}

	// Unpublish NVMe-oF resources
	if err := unpublishResources(ctx, nvmeofData, nodeID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unpublish resources: %v", err)
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ControllerModifyVolume modifies the volume's QoS parameters.
func (cs *Server) ControllerModifyVolume(
	ctx context.Context,
	req *csi.ControllerModifyVolumeRequest,
) (*csi.ControllerModifyVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	params := req.GetMutableParameters()
	if err := util.ValidateVolumeID(volumeID, true); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Step 1: Acquire volume lock
	if acquired := cs.volumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.volumeLocks.Release(volumeID)

	// Step 2: Parse QoS parameters from mutable_parameters
	hasRBDQoS := rbd.HasQoSParams(params)
	if hasRBDQoS {
		log.ErrorLog(ctx, "Cannot set RBD QoS parameters on NVMe-oF volumes")

		return nil, status.Error(codes.InvalidArgument, "cannot set RBD QoS parameters on NVMe-oF volumes")
	}
	nvmeofQoS, err := parseQoSParameters(params)
	if err != nil {
		log.ErrorLog(ctx, "failed to parse NVMe-oF QoS parameters: %v", err)

		return nil, status.Errorf(codes.InvalidArgument, "failed to parse QoS parameters: %v", err)
	}
	if nvmeofQoS != nil {
		return cs.modifyNVMeoFQoS(ctx, req, nvmeofQoS)
	}

	return &csi.ControllerModifyVolumeResponse{}, nil
}

func (cs *Server) ControllerExpandVolume(
	ctx context.Context,
	req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {
	err := cs.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME)
	if err != nil {
		log.ErrorLog(ctx, "invalid expand volume req: %v", err)

		return nil, err
	}
	// expand volume is handled by rbd backend server
	return cs.backendServer.ControllerExpandVolume(ctx, req)
}

// CreateSnapshot forwards the snapshot creation to the backend RBD driver. Because Snapshots do not need to
// be available in the NVMe-oF gateway, there are no further actions needed.
func (cs *Server) CreateSnapshot(
	ctx context.Context,
	req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {
	err := cs.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT)
	if err != nil {
		log.ErrorLog(ctx, "invalid create snapshot req: %v", err)

		return nil, err
	}

	// create snapshot is handled by rbd backend server
	return cs.backendServer.CreateSnapshot(ctx, req)
}

// DeleteSnapshot forwards the snapshot deletion to the backend RBD driver. Because Snapshots are not
// available in the NVMe-oF gateway, there are no further actions needed.
func (cs *Server) DeleteSnapshot(
	ctx context.Context,
	req *csi.DeleteSnapshotRequest,
) (*csi.DeleteSnapshotResponse, error) {
	err := cs.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT)
	if err != nil {
		log.ErrorLog(ctx, "invalid delete snapshot req: %v", err)

		return nil, err
	}

	// delete snapshot is handled by rbd backend server
	return cs.backendServer.DeleteSnapshot(ctx, req)
}

// validateCreateVolumeRequest validates the incoming request for nvmeof.
// the rest of the parameters are validated by RBD.
func validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	// Validate required parameters
	params := req.GetParameters()
	requiredParams := []string{
		"subsystemNQN", "nvmeofGatewayAddress", "nvmeofGatewayPort",
	}
	for _, param := range requiredParams {
		if params[param] == "" {
			return fmt.Errorf("missing required parameter: %s", param)
		}
	}
	// Validate listeners JSON if provided
	listeners, err := parseListeners(params["listeners"])
	if err != nil {
		return fmt.Errorf("invalid listeners parameter: %w", err)
	}
	// Validate network mask if provided
	err = validateNetworkMask(params["networkMask"])
	if err != nil {
		return fmt.Errorf("invalid network mask parameter: %w", err)
	}
	networkMask := params["networkMask"]
	// Must have EITHER listeners XOR networkMask
	if len(listeners) == 0 && networkMask == "" {
		return errors.New("must specify either 'listeners' xor 'networkMask', but got neither")
	}
	if len(listeners) > 0 && networkMask != "" {
		return errors.New("must specify either 'listeners' xor 'networkMask',but got both")
	}
	// Validate QoS parameters - cannot mix RBD and NVMe-oF QoS
	mutableParams := req.GetMutableParameters()

	// check for RBD QoS parameters in both params and mutableParams
	if hasRBDQoS := rbd.HasQoSParams(params); hasRBDQoS {
		return errors.New("setting RBD QoS parameters on NVMe-oF volumes is not supported")
	}
	if hasRBDQoS := rbd.HasQoSParams(mutableParams); hasRBDQoS {
		return errors.New("setting RBD QoS parameters on NVMe-oF volumes is not supported")
	}

	// It take the mutableParams value from the volumeAttributesClassName in the PersistentVolumeClaim yaml.
	_, err = parseQoSParameters(mutableParams)
	if err != nil {
		return fmt.Errorf("invalid NVMe-oF QoS parameters: %w", err)
	}

	return nil
}

// validatePublishVolumeRequest validates the incoming request for publishing a volume.
func validatePublishVolumeRequest(req *csi.ControllerPublishVolumeRequest) error {
	volumeContext := req.GetVolumeContext()
	requiredParams := []string{
		"subsystemNQN", "nvmeofGatewayAddress", "nvmeofGatewayPort",
	}
	for _, param := range requiredParams {
		if volumeContext[param] == "" {
			return fmt.Errorf("missing required parameter: %s", param)
		}
	}

	return util.ValidateControllerPublishVolumeRequest(req)
}

// parseListeners parses the listeners JSON parameter and validates its contents.
// it possible to have zero listeners if networkMask is provided,
// because in that case the gateway will automatically create listeners
// for all interfaces in the specified network mask.
// Also it possible to have listeners with empty address or port,
// in that case the gateway will listen on all
// interfaces (0.0.0.0) and use the default port (4420).
// example of listeners JSON:
// [
//
//	{
//	  "hostname": "gateway-1",
//	  "address": "192.168.234.123",
//	  "port": 4420
//	},
//	{
//	  "hostname": "gateway-2.ceph.example.net"
//	}
//
// ].
func parseListeners(listenersJSON string) ([]nvmeof.ListenerDetails, error) {
	if listenersJSON == "" { // No "listeners" entry was provided
		return []nvmeof.ListenerDetails{}, nil
	}
	var listeners []nvmeof.ListenerDetails
	if err := json.Unmarshal([]byte(listenersJSON), &listeners); err != nil {
		return nil, fmt.Errorf("failed to parse listeners JSON: %w", err)
	}

	if len(listeners) == 0 { // At least one listener is required
		return nil, errors.New("at least one listener must be specified")
	}

	// Validate each listener
	// Listener address can be empty. it will set to default 0.0.0.0
	// Port can be empty (will use default - 4420).
	for i, listener := range listeners {
		if listener.Hostname == "" {
			return nil, fmt.Errorf("listener %d: missing required fields (hostname)", i)
		}
	}

	return listeners, nil
}

// Validate network mask CIDR format.
func validateNetworkMask(networkMask string) error {
	if networkMask == "" {
		return nil
	}

	_, _, err := net.ParseCIDR(networkMask)
	if err != nil {
		return fmt.Errorf("invalid network mask CIDR format: %w", err)
	}

	return nil
}

// parseQoSParameters extracts and parses QoS parameters from the given map.
func parseQoSParameters(params map[string]string) (*nvmeof.NVMeoFQosVolume, error) {
	qos := &nvmeof.NVMeoFQosVolume{}
	hasAnyQoS := false

	parseParam := func(key, name string, dest **uint64) error {
		if val, exists := params[key]; exists && val != "" {
			parsed, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid %s: %w", name, err)
			}
			*dest = &parsed
			hasAnyQoS = true
		}

		return nil
	}

	if err := parseParam(nvmeof.RwIosPerSecond, nvmeof.RwIosPerSecond, &qos.RwIosPerSecond); err != nil {
		return nil, err
	}
	if err := parseParam(nvmeof.RwMbytesPerSecond, nvmeof.RwMbytesPerSecond, &qos.RwMbytesPerSecond); err != nil {
		return nil, err
	}
	if err := parseParam(nvmeof.RMbytesPerSecond, nvmeof.RMbytesPerSecond, &qos.RMbytesPerSecond); err != nil {
		return nil, err
	}
	if err := parseParam(nvmeof.WMbytesPerSecond, nvmeof.WMbytesPerSecond, &qos.WMbytesPerSecond); err != nil {
		return nil, err
	}

	if !hasAnyQoS {
		return nil, nil
	}

	return qos, nil
}

// modifyNVMeoFQoS handles NVMe-oF gateway QoS modification.
func (cs *Server) modifyNVMeoFQoS(
	ctx context.Context,
	req *csi.ControllerModifyVolumeRequest,
	qos *nvmeof.NVMeoFQosVolume,
) (*csi.ControllerModifyVolumeResponse, error) {
	volumeID := req.GetVolumeId()

	// Step 1: Get secrets

	// Since ControllerModifyVolume doesn't receive volume context and dont have option to take secrets
	// because there is no "csi.storage.k8s.io/controller-modify-secret-name" field in the SC !,
	// the full solution for it is to use GetControllerExpandSecretRef but there is no such function yet.
	// TODO: change the call to GetControllerExpandSecretRef once it is implemented.
	secrets := req.GetSecrets()
	if secrets == nil {
		secretName, secretNamespace, err := util.GetControllerPublishSecretRef(volumeID, util.RBDType)
		if err != nil {
			log.ErrorLog(ctx, "Failed to get secret reference: %v", err)

			return nil, status.Errorf(codes.Internal, "failed to get secret reference: %v", err)
		}

		secrets, err = k8s.GetSecret(secretName, secretNamespace)
		if err != nil {
			log.ErrorLog(ctx, "Failed to get secret from k8s: %v", err)

			return nil, status.Errorf(codes.Internal, "failed to get secret: %v", err)
		}
	}

	// Step 2: Get NVMe-oF metadata
	nvmeofData, err := cs.getNVMeoFMetadata(ctx, secrets, volumeID)
	if err != nil {
		log.ErrorLog(ctx, "Failed to get NVMe-oF metadata: %v", err)

		return nil, nvmeoferrors.ToGRPCError(err)
	}

	// Step 3: Connect to gateway
	config := &nvmeof.GatewayConfig{
		Address: nvmeofData.GatewayManagementInfo.Address,
		Port:    nvmeofData.GatewayManagementInfo.Port,
	}
	gateway, err := connectGateway(ctx, config)
	if err != nil {
		log.ErrorLog(ctx, "Gateway connection failed: %v", err)

		return nil, status.Errorf(codes.Unavailable, "gateway connection failed: %v", err)
	}
	defer func() {
		if closeErr := gateway.Destroy(); closeErr != nil {
			log.ErrorLog(ctx, "Failed to close gateway connection: %v", closeErr)
		}
	}()

	// Step 4: Apply NVMe-oF QoS via gateway
	log.DebugLog(ctx, "Setting QoS for subsystem=%s, nsid=%d", nvmeofData.SubsystemNQN, nvmeofData.NamespaceID)

	err = gateway.SetQoSLimitsForNamespace(ctx, nvmeofData.SubsystemNQN, nvmeofData.NamespaceID, *qos)
	if err != nil {
		// Check if error is EEXIST (RBD QoS already set)
		if errors.Is(err, nvmeoferrors.ErrRbdQoSExists) {
			log.ErrorLog(ctx, "RBD QoS already configured on volume")

			return nil, status.Error(codes.InvalidArgument,
				"RBD QoS already configured on this volume, cannot set NVMe-oF gateway QoS")
		}

		log.ErrorLog(ctx, "Failed to set QoS limits: %v", err)

		return nil, status.Errorf(codes.Internal, "failed to set QoS limits: %v", err)
	}

	log.DebugLog(ctx, "Successfully modified NVMe-oF QoS for volume %s", volumeID)

	return &csi.ControllerModifyVolumeResponse{}, nil
}

// ensureSubsystem checks if the subsystem exists, and creates it if not.
// then creates the listener.
func ensureSubsystem(
	ctx context.Context,
	gateway *nvmeof.GatewayRpcClient,
	subsystemNQN,
	networkMask string,
	listeners []nvmeof.ListenerDetails,
) error {
	exists, err := gateway.SubsystemExists(ctx, subsystemNQN)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	// Create if doesn't exist (controller decision)
	err = gateway.CreateSubsystem(ctx, subsystemNQN, networkMask)
	if err != nil {
		return err
	}

	// if networkMask is not provided, listeners are not created automatically by gateway,
	// should create them manually one by one.
	if networkMask == "" {
		// Create all listeners
		for i, listener := range listeners {
			log.DebugLog(ctx, "Creating listener %d: %s", i, listener.String())
			if err := gateway.CreateListener(ctx, subsystemNQN, listener); err != nil {
				return fmt.Errorf("failed to create listener %d (%s): %w", i, listener.String(), err)
			}
		}
	}

	return nil
}

// cleanupEmptySubsystem checks if the subsystem is empty (no namespaces), if so,
// first deletes the listener and then deletes it.
func cleanupEmptySubsystem(
	ctx context.Context,
	gateway *nvmeof.GatewayRpcClient,
	subsystemNQN string,
	listeners []nvmeof.ListenerDetails,
) error {
	if subsystemNQN == "" {
		return nil
	}
	exists, err := gateway.SubsystemExists(ctx, subsystemNQN)
	if err != nil {
		return err
	}
	if !exists { // In case the subsystem already was deleted, return no error
		log.DebugLog(ctx, "Subsystem %s does not exists", subsystemNQN)

		return nil
	}
	// Check if subsystem has any remaining namespaces
	namespaces, err := gateway.ListNamespaces(ctx, subsystemNQN)
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}
	if len(namespaces.GetNamespaces()) != 0 {
		log.DebugLog(ctx, "Subsystem %s still has %d namespaces, keeping",
			subsystemNQN, len(namespaces.GetNamespaces()))

		return nil
	}

	// subsystem is empty delete listener first
	for i, listener := range listeners {
		if err := gateway.DeleteListener(ctx, subsystemNQN, listener); err != nil {
			return fmt.Errorf("failed to delete listener %d (%s) for subsystem %s: %w",
				i, listener.String(), subsystemNQN, err)
		}
	}

	log.DebugLog(ctx, "Subsystem %s is empty, deleting", subsystemNQN)
	err = gateway.DeleteSubsystem(ctx, subsystemNQN)
	if err != nil {
		return fmt.Errorf("failed to delete empty subsystem: %w", err)
	}
	log.DebugLog(ctx, "Empty subsystem %s deleted", subsystemNQN)

	return nil
}

// createNVMeoFResources sets up the NVMe-oF resources for the given RBD volume.
func (cs *Server) createNVMeoFResources(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
	rbdPoolName,
	rbdRadosNameSpace,
	rbdImageName string,
) (*nvmeof.NVMeoFVolumeData, error) {
	// Step 1: Extract parameters (already validated)
	params := req.GetParameters()

	networkMask := params["networkMask"]
	nvmeofGatewayPortStr := params["nvmeofGatewayPort"]
	nvmeofGatewayPort, err := strconv.ParseUint(nvmeofGatewayPortStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid nvmeofGatewayPort %s: %w", nvmeofGatewayPortStr, err)
	}
	nvmeofData := &nvmeof.NVMeoFVolumeData{
		SubsystemNQN:  params["subsystemNQN"],
		NamespaceID:   0,   // will be set after namespace creation,
		NamespaceUUID: "",  // will be set after namespace creation
		ListenerInfo:  nil, // will be set after listener creation or retrieval
		GatewayManagementInfo: nvmeof.GatewayConfig{
			Address: params["nvmeofGatewayAddress"],
			Port:    uint32(nvmeofGatewayPort),
		},
	}

	// setup listeners (if provided, otherwise it will be set by gateway based on network mask)
	err = setupDefaultListenersValues(params["listeners"], nvmeofData)
	if err != nil {
		return nil, err
	}

	// extract Qos parameters if any
	mutableParams := req.GetMutableParameters()
	// It take the mutableParams value from the volumeAttributesClassName in the PersistentVolumeClaim yaml.
	// We already verified in the validateCreateVolumeRequest that there is no RBD QoS
	nvmeofQoS, err := parseQoSParameters(mutableParams)
	if err != nil {
		log.ErrorLog(ctx, "failed to parse NVMe-oF QoS parameters: %v", err)

		return nil, fmt.Errorf("failed to parse QoS parameters: %w", err)
	}

	// Step 2: Connect to gateway
	config, err := getGatewayConfigFromRequest(params)
	if err != nil {
		return nil, err
	}
	gateway, err := connectGateway(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("gateway connection failed: %w", err)
	}
	defer func() {
		if closeErr := gateway.Destroy(); closeErr != nil {
			log.ErrorLog(ctx, "Warning: failed to close gateway connection: %v", closeErr)
		}
	}()

	// TODO: replace util.VolumeOperationAlreadyExistsFmt
	if acquired := cs.subsystemLocks.TryAcquire(nvmeofData.SubsystemNQN); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, nvmeofData.SubsystemNQN)

		return nil, fmt.Errorf(util.VolumeOperationAlreadyExistsFmt, nvmeofData.SubsystemNQN)
	}
	defer cs.subsystemLocks.Release(nvmeofData.SubsystemNQN)

	// Step 3: Ensure subsystem exists (and listener)
	if err := ensureSubsystem(ctx, gateway, nvmeofData.SubsystemNQN, networkMask, nvmeofData.ListenerInfo); err != nil {
		return nvmeofData, fmt.Errorf("subsystem setup failed: %w", err)
	}

	log.DebugLog(ctx, "subsystem %s and Listener %s for the subsystem were created", nvmeofData.SubsystemNQN,
		nvmeofData.ListenerInfo)

	// Step 4: Create namespace and set its uuid
	nsid, err := gateway.CreateNamespace(ctx, nvmeofData.SubsystemNQN, rbdPoolName, rbdRadosNameSpace, rbdImageName)
	if err != nil {
		return nvmeofData, fmt.Errorf("namespace creation failed: %w", err)
	}
	log.DebugLog(ctx, "Namespace created: %s/%s with NSID: %d", rbdPoolName, rbdImageName, nsid)
	nvmeofData.NamespaceID = nsid

	// Step 5: Set QoS limits if any
	if nvmeofQoS != nil {
		log.DebugLog(ctx, "Setting QoS limits: %s", nvmeofQoS)
		if err := gateway.SetQoSLimitsForNamespace(ctx, nvmeofData.SubsystemNQN, nvmeofData.NamespaceID,
			*nvmeofQoS); err != nil {
			return nvmeofData, fmt.Errorf("setting QoS limits failed: %w", err)
		}
	}

	// Step 6: If using auto-listeners, query them back for storing in metadata
	if networkMask != "" {
		autoListeners, err := gateway.ListListeners(ctx, nvmeofData.SubsystemNQN)
		if err != nil {
			return nvmeofData, fmt.Errorf("failed to list auto-created listeners: %w", err)
		}
		nvmeofData.ListenerInfo = nvmeof.ConvertListenersFromProto(autoListeners.GetListeners())
		log.DebugLog(ctx, "Retrieved %d auto-created listeners", len(nvmeofData.ListenerInfo))
	}

	uuid, err := gateway.GetUUIDBySubsystemAndNameSpaceID(ctx, nvmeofData.SubsystemNQN, nvmeofData.NamespaceID)
	if err != nil {
		return nvmeofData, fmt.Errorf("get namespace uuid failed: %w", err)
	}
	nvmeofData.NamespaceUUID = uuid

	return nvmeofData, nil
}

// cleanupEmptySubsystem removes the subsystem if it exists and has no namespaces.
// This function is idempotent and safe to call even if the subsystem doesn't exist
// or has active namespaces.
func (cs *Server) cleanupNVMeoFResources(
	ctx context.Context,
	nvmeofData *nvmeof.NVMeoFVolumeData,
) error {
	// Step 1: Connect to gateway using stored management address
	gateway, err := connectGateway(ctx, &nvmeof.GatewayConfig{
		Address: nvmeofData.GatewayManagementInfo.Address,
		Port:    nvmeofData.GatewayManagementInfo.Port,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to gateway for cleanup: %w", err)
	}
	defer func() {
		if closeErr := gateway.Destroy(); closeErr != nil {
			log.ErrorLog(ctx, "Warning: failed to close gateway connection: %v", closeErr)
		}
	}()

	if acquired := cs.subsystemLocks.TryAcquire(nvmeofData.SubsystemNQN); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, nvmeofData.SubsystemNQN)

		return fmt.Errorf(util.VolumeOperationAlreadyExistsFmt, nvmeofData.SubsystemNQN)
	}
	defer cs.subsystemLocks.Release(nvmeofData.SubsystemNQN)

	// Step 2: Delete namespace
	// just in case namespace was created. NsID=0 means it was never created.
	// it is not possible to have a namespace with ID 0.
	if nvmeofData.NamespaceID > 0 {
		log.DebugLog(ctx, "Deleting namespace %d for subsystem %s", nvmeofData.NamespaceID, nvmeofData.SubsystemNQN)
		if err := gateway.DeleteNamespace(ctx, nvmeofData.SubsystemNQN, nvmeofData.NamespaceID); err != nil {
			return fmt.Errorf("failed to delete namespace %d for subsystem %s: %w",
				nvmeofData.NamespaceID, nvmeofData.SubsystemNQN, err)
		}
		log.DebugLog(ctx, "Namespace %d deleted for subsystem %s", nvmeofData.NamespaceID, nvmeofData.SubsystemNQN)
	} else {
		log.DebugLog(ctx, "No namespace ID found in NVMe-oF metadata, skipping namespace deletion")
	}

	// Step 3: Cleanup empty subsystem
	if err := cleanupEmptySubsystem(ctx, gateway, nvmeofData.SubsystemNQN, nvmeofData.ListenerInfo); err != nil {
		return fmt.Errorf("failed to cleanup empty subsystem %s: %w", nvmeofData.SubsystemNQN, err)
	}

	return nil
}

// publishResources publishes the HostNQN to be allowed to see the Volume.
func publishResources(ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (string, error) {
	nodeID := req.GetNodeId()
	hostNQN, err := getHostNQNFromNodeID(nodeID)
	if err != nil {
		return "", status.Errorf(codes.InvalidArgument, "invalid nodeID format: %v", err)
	}
	// Get volume context from the volume (contains subsystem info from CreateVolume)
	volumeContext := req.GetVolumeContext()
	subsystemNQN := volumeContext[vcSubsystemNQN]
	gatewayAddr := volumeContext[vcGatewayAddress]
	gatewayPortStr := volumeContext[vcGatewayPort]

	// Convert gateway port from string to uint32
	gatewayPort, err := strconv.ParseUint(gatewayPortStr, 10, 32)
	if err != nil {
		return "", fmt.Errorf("invalid gateway port %s: %w", gatewayPortStr, err)
	}
	// Connect to gateway and add host
	config := &nvmeof.GatewayConfig{
		Address: gatewayAddr,
		Port:    uint32(gatewayPort),
	}
	gateway, err := connectGateway(ctx, config)
	if err != nil {
		return "", fmt.Errorf("gateway connection failed: %w", err)
	}
	defer func() {
		if closeErr := gateway.Destroy(); closeErr != nil {
			log.ErrorLog(ctx, "Warning: failed to close gateway connection: %v", closeErr)
		}
	}()

	// Add host to subsystem
	if err := gateway.AddHost(ctx, subsystemNQN, hostNQN); err != nil {
		return "", fmt.Errorf("failed to add host %s: %w", hostNQN, err)
	}

	log.DebugLog(ctx, "Host %s successfully added to subsystem %s", hostNQN, subsystemNQN)

	return hostNQN, nil
}

// unpublishResources removes the host from the NVMe-oF subsystem.
func unpublishResources(ctx context.Context, data *nvmeof.NVMeoFVolumeData, nodeID string) error {
	// Extract host NQN from nodeID
	hostNQN, err := getHostNQNFromNodeID(nodeID)
	if err != nil {
		return fmt.Errorf("invalid nodeID format: %w", err)
	}
	subsystemNQN := data.SubsystemNQN
	gatewayAddr := data.GatewayManagementInfo.Address
	gatewayPort := data.GatewayManagementInfo.Port

	// Connect to gateway and add host
	config := &nvmeof.GatewayConfig{
		Address: gatewayAddr,
		Port:    gatewayPort,
	}
	gateway, err := connectGateway(ctx, config)
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer func() {
		if closeErr := gateway.Destroy(); closeErr != nil {
			log.ErrorLog(ctx, "Warning: failed to close gateway connection: %v", closeErr)
		}
	}()

	// check if there are more namespaces in this subsystem with this host, if so, do not remove the host.
	namespaces, err := gateway.ListNamespaces(ctx, subsystemNQN)
	if err != nil {
		return fmt.Errorf("failed to list namespaces for subsystem %s: %w", subsystemNQN, err)
	}
	for _, ns := range namespaces.GetNamespaces() {
		for _, host := range ns.GetHosts() {
			if host == hostNQN {
				log.DebugLog(ctx, "Host %s is still using namespace %s, not removing", hostNQN, ns.GetNsid())

				return nil
			}
		}
	}
	// Remove host from subsystem
	if err := gateway.RemoveHost(ctx, subsystemNQN, hostNQN); err != nil {
		return fmt.Errorf("failed to remove host %s from subsystem %s: %w",
			hostNQN, subsystemNQN, err)
	}
	log.DebugLog(ctx, "Host %s removed from subsystem %s", hostNQN, subsystemNQN)

	return nil
}

// getHostNQNFromNodeID constructs the host NQN from the nodeID. just concatenate the prefix
// with the nodeID. the hostnqn format is nqn.<YOUR-DATE>.<YOUR-REVERSE-DOMAIN>:<user-part>,
// we use the default prefix (nqn.2014-08.org.nvmexpress:)
// and append the nodeID as user-part. Also there is no need to add an UUID as user-part,
// as the nodeID is already unique.
func getHostNQNFromNodeID(nodeID string) (string, error) {
	const prefix = "nqn.2014-08.org.nvmexpress:"
	if nodeID == "" {
		return "", fmt.Errorf("invalid nodeID format: %s", nodeID)
	}

	return prefix + nodeID, nil
}

// VolumeContext metadata keys.
const (
	// NVMe-oF resource info.
	vcSubsystemNQN  = "SubsystemNQN"
	vcNamespaceID   = "NamespaceID"
	vcNamespaceUUID = "NamespaceUUID"
	vcHostNQN       = "HostNQN"

	// Multiple listeners stored as JSON.
	vcListeners = "listeners"

	// Gateway management info.
	vcGatewayAddress = "GatewayAddress"
	vcGatewayPort    = "GatewayPort"
)

// toRBDMetadataKey converts clean volume context key to prefixed RBD metadata key.
func toRBDMetadataKey(vcKey string) string {
	return ".rbd.nvmeof." + vcKey
}

// populateVolumeContext adds NVMe-oF information to volume context for NodeServer.
func populateVolumeContext(volume *csi.Volume, data *nvmeof.NVMeoFVolumeData) error {
	if volume.VolumeContext == nil {
		volume.VolumeContext = make(map[string]string)
	}
	gatewayManagementInfoPortStr := strconv.FormatUint(uint64(data.GatewayManagementInfo.Port), 10)

	volume.VolumeContext[vcSubsystemNQN] = data.SubsystemNQN
	volume.VolumeContext[vcNamespaceID] = strconv.FormatUint(uint64(data.NamespaceID), 10)
	volume.VolumeContext[vcNamespaceUUID] = data.NamespaceUUID
	volume.VolumeContext[vcGatewayAddress] = data.GatewayManagementInfo.Address
	volume.VolumeContext[vcGatewayPort] = gatewayManagementInfoPortStr

	// Store listeners as JSON
	listenersJSON, err := json.Marshal(data.ListenerInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal listener info: %w", err)
	}
	volume.VolumeContext[vcListeners] = string(listenersJSON)

	return nil
}

// populatePublishContext creates a publish context for the volume.
func populatePublishContext(req *csi.ControllerPublishVolumeRequest, hostNqn string) map[string]string {
	publishContext := make(map[string]string)
	for key, value := range req.GetVolumeContext() {
		publishContext[key] = value
	}
	publishContext[vcHostNQN] = hostNqn

	return publishContext
}

// storeNVMeoFMetadata stores all NVMe-oF data in RBD volume metadata for cleanup operations.
func (cs *Server) storeNVMeoFMetadata(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
	volumeID string,
	nvmeofData *nvmeof.NVMeoFVolumeData,
) error {
	// Create RBD manager
	mgr := rbdutil.NewManager(cs.backendServer.Driver.GetInstanceID(), nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	// Get RBD volume
	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return status.Errorf(codes.Aborted, "failed to find volume with ID %q: %v", volumeID, err)
	}
	defer rbdVol.Destroy(ctx)

	gatewayManagementInfoPortStr := strconv.FormatUint(uint64(nvmeofData.GatewayManagementInfo.Port), 10)

	// Serialize listeners to JSON
	listenersJSON, err := json.Marshal(nvmeofData.ListenerInfo)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to serialize listeners: %v", err)
	}

	// Prepare all metadata entries
	metadata := map[string]string{
		// NVMe-oF resource info
		toRBDMetadataKey(vcSubsystemNQN):  nvmeofData.SubsystemNQN,
		toRBDMetadataKey(vcNamespaceID):   strconv.FormatUint(uint64(nvmeofData.NamespaceID), 10),
		toRBDMetadataKey(vcNamespaceUUID): nvmeofData.NamespaceUUID,

		// Listeners as JSON
		toRBDMetadataKey(vcListeners): string(listenersJSON),

		// Gateway management info
		toRBDMetadataKey(vcGatewayAddress): nvmeofData.GatewayManagementInfo.Address,
		toRBDMetadataKey(vcGatewayPort):    gatewayManagementInfoPortStr,
	}

	// Store all metadata entries
	for key, value := range metadata {
		if value == "" {
			log.WarningLog(ctx, "Skipping empty metadata value for key: %s", key)

			continue
		}

		if err := rbdVol.SetMetadata(key, value); err != nil {
			log.ErrorLog(ctx, "Failed to store %s metadata: %v", key, err)

			return status.Errorf(codes.Internal, "failed to store %s metadata: %v", key, err)
		}
	}

	log.DebugLog(ctx, "All NVMe-oF metadata stored successfully for volume: %s", volumeID)

	return nil
}

// getNVMeoFMetadata retrieves all NVMe-oF data from RBD volume metadata.
func (cs *Server) getNVMeoFMetadata(
	ctx context.Context,
	secrets map[string]string,
	volumeID string,
) (*nvmeof.NVMeoFVolumeData, error) {
	// Create RBD manager
	mgr := rbdutil.NewManager(cs.backendServer.Driver.GetInstanceID(), nil, secrets)
	defer mgr.Destroy(ctx)

	// Get RBD volume
	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to find volume with ID %q: %w",
			nvmeoferrors.ErrMetadataNotFound, volumeID, err)
	}
	defer rbdVol.Destroy(ctx)

	// Retrieve all metadata including gateway management info
	metadata := make(map[string]string)

	// Required metadata keys
	requiredKeys := []string{
		toRBDMetadataKey(vcSubsystemNQN),
		toRBDMetadataKey(vcNamespaceID),
		toRBDMetadataKey(vcNamespaceUUID),
		toRBDMetadataKey(vcListeners),
		toRBDMetadataKey(vcGatewayAddress),
		toRBDMetadataKey(vcGatewayPort),
	}

	// Retrieve all metadata values
	for _, key := range requiredKeys {
		value, err := rbdVol.GetMetadata(key)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get %s: %w",
				nvmeoferrors.ErrMetadataNotFound, key, err)
		}
		if value == "" {
			return nil, fmt.Errorf("%w: metadata %s is empty",
				nvmeoferrors.ErrMetadataNotFound, key)
		}
		metadata[key] = value
	}

	// Parse namespace ID
	nsid, err := strconv.ParseUint(metadata[toRBDMetadataKey(vcNamespaceID)], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid namespace ID: %w",
			nvmeoferrors.ErrMetadataCorrupted, err)
	}

	gatewayPort, err := strconv.ParseUint(metadata[toRBDMetadataKey(vcGatewayPort)], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid gateway port: %w",
			nvmeoferrors.ErrMetadataCorrupted, err)
	}

	// Parse listeners from JSON
	var listeners []nvmeof.ListenerDetails
	if err := json.Unmarshal([]byte(metadata[toRBDMetadataKey(vcListeners)]), &listeners); err != nil {
		return nil, fmt.Errorf("%w: failed to parse listeners JSON: %w",
			nvmeoferrors.ErrMetadataCorrupted, err)
	}

	// Construct NVMe-oF volume data
	nvmeofData := &nvmeof.NVMeoFVolumeData{
		SubsystemNQN:  metadata[toRBDMetadataKey(vcSubsystemNQN)],
		NamespaceID:   uint32(nsid),
		NamespaceUUID: metadata[toRBDMetadataKey(vcNamespaceUUID)],
		ListenerInfo:  listeners,
		// Store gateway management info separately
		GatewayManagementInfo: nvmeof.GatewayConfig{
			Address: metadata[toRBDMetadataKey(vcGatewayAddress)],
			Port:    uint32(gatewayPort),
		},
	}

	return nvmeofData, nil
}

// getGatewayConfigFromRequest extracts gateway configuration from request parameters.
func getGatewayConfigFromRequest(params map[string]string) (*nvmeof.GatewayConfig, error) {
	address := params["nvmeofGatewayAddress"]
	if address == "" {
		return nil, errors.New("nvmeofGatewayAddress parameter is required")
	}

	portStr := params["nvmeofGatewayPort"]

	if portStr == "" {
		return nil, errors.New("nvmeofGatewayPort parameter is required")
	}

	// Convert string to proper port type
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port %s: %w", portStr, err)
	}

	return &nvmeof.GatewayConfig{
		Address: address,
		Port:    uint32(port),
	}, nil
}

// connectGateway creates and connects a gateway client for this operation.
func connectGateway(ctx context.Context, config *nvmeof.GatewayConfig) (*nvmeof.GatewayRpcClient, error) {
	gateway, err := nvmeof.NewGatewayRpcClient(config)
	if err != nil {
		log.DebugLog(ctx, "failed to connect to management gateway: %s", config)

		return nil, fmt.Errorf("failed to connect to management gateway %s: %w",
			config, err)
	}
	log.DebugLog(ctx, "Connected to the gateway %s", config)

	return gateway, nil
}

// setupDefaultListeners validates and sets up default values for NVMe-oF listeners.
// if listeners are provided, it ensures they are fully populated with
// default values if needed (port and address).
func setupDefaultListenersValues(listenersJSON string, info *nvmeof.NVMeoFVolumeData) error {
	// Parse listeners from JSON
	listeners, err := parseListeners(listenersJSON)
	if err != nil {
		return fmt.Errorf("failed to parse listeners: %w", err)
	}

	// ensure listeners are fully populated with default values if needed (port and address)
	// before storing in metadata and creating subsystem/listeners
	for i := range listeners {
		if listeners[i].Port == 0 {
			listeners[i].Port = 4420
		}
		// if address is empty, set it to default 0.0.0.0
		if listeners[i].Address == "" {
			listeners[i].Address = "0.0.0.0"
		}
	}
	info.ListenerInfo = listeners

	return nil
}
