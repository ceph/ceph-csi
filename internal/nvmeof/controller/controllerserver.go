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
	"errors"
	"fmt"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/nvmeof"
	"github.com/ceph/ceph-csi/internal/rbd"
	rbdutil "github.com/ceph/ceph-csi/internal/rbd"
	rbddriver "github.com/ceph/ceph-csi/internal/rbd/driver"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

type Server struct {
	csi.UnimplementedControllerServer

	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	volumeLocks *util.VolumeLocks

	// backendServer handles the RBD requests
	backendServer *rbd.ControllerServer
}

// NewControllerServer initialize a controller server for nvmeof CSI driver.
func NewControllerServer(d *csicommon.CSIDriver) (*Server, error) {
	return &Server{
		volumeLocks:   util.NewVolumeLocks(),
		backendServer: rbddriver.NewControllerServer(d),
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
	// TODO - in case of failure, we should clean up the created resources (RBD volume, subsystem, etc.)
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
	rbdImageName := res.GetVolume().GetVolumeContext()["imageName"]
	rbdPoolName := res.GetVolume().GetVolumeContext()["pool"]

	// Step 2: Setup NVMe-oF resources
	nvmeofData, err := createNVMeoFResources(ctx, req, rbdPoolName, rbdImageName)
	if err != nil {
		log.ErrorLog(ctx, "NVMe-oF resource setup failed for volumeID %s: %v", volumeID, err)
		// TODO: Implement cleanup of RBD volume on NVMe-oF failure

		return nil, status.Errorf(codes.Internal, "NVMe-oF setup failed: %v", err)
	}

	// step 3: Populate volume context for NodeServer
	populateVolumeContext(backend, nvmeofData)

	// Step 4: Store NVMe-oF metadata in the volume context
	if err := cs.storeNVMeoFMetadata(ctx, req, volumeID, nvmeofData); err != nil {
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
	if volumeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "empty volume ID in request")
	}

	// prevent concurrent requests for the same volume
	if acquired := cs.volumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.volumeLocks.Release(volumeID)

	// Get NVMe-oF metadata for cleanup
	nvmeofData, err := cs.getNVMeoFMetadata(ctx, req, volumeID)
	if err != nil {
		log.DebugLog(ctx, "No NVMe-oF metadata found, skipping NVMe-oF cleanup: %v", err)
	} else {
		// Clean up NVMe-oF resources
		if err := cleanupNVMeoFResources(ctx, req, nvmeofData); err != nil {
			log.ErrorLog(ctx, "NVMe-oF cleanup failed (continuing with RBD deletion): %v", err)

			return nil, status.Errorf(codes.Internal, "NVMe-oF cleanup failed: %v", err)
		}
	}
	// Delete RBD volume through backend

	return cs.backendServer.DeleteVolume(ctx, req)
}

// validateCreateVolumeRequest validates the incoming request for nvmeof.
// the rest of the parameters are validated by RBD.
func validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	// Validate required parameters
	params := req.GetParameters()
	requiredParams := []string{
		"subsystemNQN", "hostNQN", "nvmeofGatewayAddress", "nvmeofGatewayPort",
		"listenerIpAddress", "listenerPort", "listenerHostname",
	}
	for _, param := range requiredParams {
		if params[param] == "" {
			return fmt.Errorf("missing required parameter: %s", param)
		}
	}

	return nil
}

// ensureSubsystem checks if the subsystem exists, and creates it if not.
func ensureSubsystem(ctx context.Context, gateway *nvmeof.GatewayRpcClient, subsystemNQN string) error {
	exists, err := gateway.SubsystemExists(ctx, subsystemNQN)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	// Create if doesn't exist (controller decision)

	return gateway.CreateSubsystem(ctx, subsystemNQN)
}

// cleanupEmptySubsystem checks if the subsystem is empty (no namespaces), if so, deletes it.
func cleanupEmptySubsystem(ctx context.Context, gateway *nvmeof.GatewayRpcClient, subsystemNQN string,
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
	log.DebugLog(ctx, "Subsystem %s is empty, deleting", subsystemNQN)
	err = gateway.DeleteSubsystem(ctx, subsystemNQN)
	if err != nil {
		return fmt.Errorf("failed to delete empty subsystem: %w", err)
	}
	log.DebugLog(ctx, "Empty subsystem %s deleted", subsystemNQN)

	return nil
}

// createNVMeoFResources sets up the NVMe-oF resources for the given RBD volume.
// TODO - need to support multiple listeners.
// TODO - need to fallback cleanup if any step fails.
func createNVMeoFResources(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
	rbdPoolName,
	rbdImageName string,
) (*nvmeof.NVMeoFVolumeData, error) {
	// Step 1: Extract parameters (already validated)
	params := req.GetParameters()
	listenerPortStr := params["listenerPort"]
	listenerPort, err := strconv.ParseUint(listenerPortStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid listenerPort %s: %w", listenerPortStr, err)
	}
	nvmeofGatewayPortStr := params["nvmeofGatewayPort"]
	nvmeofGatewayPort, err := strconv.ParseUint(nvmeofGatewayPortStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid nvmeofGatewayPort %s: %w", nvmeofGatewayPortStr, err)
	}
	nvmeofData := &nvmeof.NVMeoFVolumeData{
		SubsystemNQN:  params["subsystemNQN"],
		NamespaceID:   0,  // will be set after namespace creation,
		NamespaceUUID: "", // will be set after namespace creation
		HostNQN:       params["hostNQN"],
		ListenerInfo: nvmeof.ListenerDetails{
			GatewayAddress: nvmeof.GatewayAddress{
				Address: params["listenerIpAddress"],
				Port:    uint32(listenerPort),
			},
			Hostname: params["listenerHostname"],
		},
		GatewayManagementInfo: nvmeof.GatewayConfig{
			Address: params["nvmeofGatewayAddress"],
			Port:    uint32(nvmeofGatewayPort),
		},
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

	// Step 3: Ensure subsystem exists
	if err := ensureSubsystem(ctx, gateway, nvmeofData.SubsystemNQN); err != nil {
		return nil, fmt.Errorf("subsystem setup failed: %w", err)
	}

	// Step 4: Create listeners
	err = gateway.CreateListener(ctx, nvmeofData.SubsystemNQN, nvmeofData.ListenerInfo)
	if err != nil {
		return nil, fmt.Errorf("listener creation failed: %w", err)
	}
	log.DebugLog(ctx, "Listener created for subsystem %s at %s", nvmeofData.SubsystemNQN,
		nvmeofData.ListenerInfo)

	// Step 5: Create namespace and set its uuid
	nsid, err := gateway.CreateNamespace(ctx, nvmeofData.SubsystemNQN, rbdPoolName, rbdImageName)
	if err != nil {
		return nil, fmt.Errorf("namespace creation failed: %w", err)
	}
	log.DebugLog(ctx, "Namespace created: %s/%s with NSID: %d", rbdPoolName, rbdImageName, nsid)
	nvmeofData.NamespaceID = nsid

	uuid, err := gateway.GetUUIDBySubsystemAndNameSpaceID(ctx, nvmeofData.SubsystemNQN, nvmeofData.NamespaceID)
	if err != nil {
		return nil, fmt.Errorf("get namespace uuid failed: %w", err)
	}
	nvmeofData.NamespaceUUID = uuid

	// Step 6: Add host to subsystem
	if err := gateway.AddHost(ctx, nvmeofData.SubsystemNQN, nvmeofData.HostNQN); err != nil {
		return nil, fmt.Errorf("host addition failed: %w", err)
	}
	log.DebugLog(ctx, "Host added: %s to subsystem %s", nvmeofData.HostNQN, nvmeofData.SubsystemNQN)

	return nvmeofData, nil
}

// cleanupNVMeoFResources cleans up NVMe-oF resources associated with the volume.
// This includes removing the host, listener, namespace, and potentially the subsystem.
func cleanupNVMeoFResources(
	ctx context.Context,
	req *csi.DeleteVolumeRequest,
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

	// TODO: maybe just check before if the subsystem exists, if not ,
	// there is no relevant to continue , will make it simple ?
	// instead of check in the std error if "not found"..

	// Step 2: Remove host from subsystem
	if err := gateway.RemoveHost(ctx, nvmeofData.SubsystemNQN, nvmeofData.HostNQN); err != nil {
		return fmt.Errorf("failed to remove host %s from subsystem %s: %w",
			nvmeofData.HostNQN, nvmeofData.SubsystemNQN, err)
	}
	log.DebugLog(ctx, "Host %s removed from subsystem %s", nvmeofData.HostNQN, nvmeofData.SubsystemNQN)

	// Step 3: Delete namespace
	if err := gateway.DeleteNamespace(ctx, nvmeofData.SubsystemNQN, nvmeofData.NamespaceID); err != nil {
		return fmt.Errorf("failed to delete namespace %d for subsystem %s: %w",
			nvmeofData.NamespaceID, nvmeofData.SubsystemNQN, err)
	}
	log.DebugLog(ctx, "Namespace %d deleted for subsystem %s", nvmeofData.NamespaceID, nvmeofData.SubsystemNQN)

	// Step 4: Delete listener
	err = gateway.DeleteListener(ctx, nvmeofData.SubsystemNQN, nvmeofData.ListenerInfo)
	if err != nil {
		return fmt.Errorf("failed to delete listener for subsystem %s: %w", nvmeofData.SubsystemNQN, err)
	}
	log.DebugLog(ctx, "Listener deleted for subsystem %s at %s", nvmeofData.SubsystemNQN,
		nvmeofData.ListenerInfo)

	// Step 5: Cleanup empty subsystem
	if err := cleanupEmptySubsystem(ctx, gateway, nvmeofData.SubsystemNQN); err != nil {
		return fmt.Errorf("failed to cleanup empty subsystem %s: %w", nvmeofData.SubsystemNQN, err)
	}
	log.DebugLog(ctx, "NVMe-oF resources cleaned up for volume with ID %s", req.GetVolumeId())

	return nil
}

// VolumeContext metadata keys.
const (
	// NVMe-oF resource info.
	vcSubsystemNQN  = "SubsystemNQN"
	vcNamespaceID   = "NamespaceID"
	vcNamespaceUUID = "NamespaceUUID"
	vcHostNQN       = "HostNQN"

	// Listener info.
	vcListenerAddress  = "ListenerAddress"
	vcListenerPort     = "ListenerPort"
	vcListenerHostname = "ListenerHostname"

	// Gateway management info.
	vcGatewayAddress = "GatewayAddress"
	vcGatewayPort    = "GatewayPort"
)

// toRBDMetadataKey converts clean volume context key to prefixed RBD metadata key.
func toRBDMetadataKey(vcKey string) string {
	return ".rbd.nvmeof." + vcKey
}

// populateVolumeContext adds NVMe-oF information to volume context for NodeServer.
func populateVolumeContext(volume *csi.Volume, data *nvmeof.NVMeoFVolumeData) {
	if volume.VolumeContext == nil {
		volume.VolumeContext = make(map[string]string)
	}
	listenerPortStr := strconv.FormatUint(uint64(data.ListenerInfo.Port), 10)
	gatewayManagementInfoPortStr := strconv.FormatUint(uint64(data.GatewayManagementInfo.Port), 10)

	volume.VolumeContext[vcSubsystemNQN] = data.SubsystemNQN
	volume.VolumeContext[vcNamespaceID] = strconv.FormatUint(uint64(data.NamespaceID), 10)
	volume.VolumeContext[vcNamespaceUUID] = data.NamespaceUUID
	volume.VolumeContext[vcHostNQN] = data.HostNQN
	volume.VolumeContext[vcListenerAddress] = data.ListenerInfo.Address
	volume.VolumeContext[vcListenerPort] = listenerPortStr
	volume.VolumeContext[vcListenerHostname] = data.ListenerInfo.Hostname
	volume.VolumeContext[vcGatewayAddress] = data.GatewayManagementInfo.Address
	volume.VolumeContext[vcGatewayPort] = gatewayManagementInfoPortStr
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
	listenerInfoPortStr := strconv.FormatUint(uint64(nvmeofData.ListenerInfo.Port), 10)

	// Prepare all metadata entries
	metadata := map[string]string{
		// NVMe-oF resource info
		toRBDMetadataKey(vcSubsystemNQN):  nvmeofData.SubsystemNQN,
		toRBDMetadataKey(vcNamespaceID):   strconv.FormatUint(uint64(nvmeofData.NamespaceID), 10),
		toRBDMetadataKey(vcNamespaceUUID): nvmeofData.NamespaceUUID,
		toRBDMetadataKey(vcHostNQN):       nvmeofData.HostNQN,

		// Listener info
		toRBDMetadataKey(vcListenerAddress):  nvmeofData.ListenerInfo.Address,
		toRBDMetadataKey(vcListenerPort):     listenerInfoPortStr,
		toRBDMetadataKey(vcListenerHostname): nvmeofData.ListenerInfo.Hostname,

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
	req *csi.DeleteVolumeRequest,
	volumeID string,
) (*nvmeof.NVMeoFVolumeData, error) {
	// Create RBD manager
	mgr := rbdutil.NewManager(cs.backendServer.Driver.GetInstanceID(), nil, req.GetSecrets())
	defer mgr.Destroy(ctx)

	// Get RBD volume
	rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find volume with ID %q: %w", volumeID, err)
	}
	defer rbdVol.Destroy(ctx)

	// Retrieve all metadata including gateway management info
	metadata := make(map[string]string)

	// Required metadata keys
	requiredKeys := []string{
		toRBDMetadataKey(vcSubsystemNQN),
		toRBDMetadataKey(vcNamespaceID),
		toRBDMetadataKey(vcNamespaceUUID),
		toRBDMetadataKey(vcHostNQN),
		toRBDMetadataKey(vcListenerAddress),
		toRBDMetadataKey(vcListenerPort),
		toRBDMetadataKey(vcListenerHostname),
		toRBDMetadataKey(vcGatewayAddress),
		toRBDMetadataKey(vcGatewayPort),
	}

	// Retrieve all metadata values
	for _, key := range requiredKeys {
		value, err := rbdVol.GetMetadata(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get %s: %w", key, err)
		}
		if value == "" {
			return nil, fmt.Errorf("metadata %s is empty", key)
		}
		metadata[key] = value
	}

	// Parse namespace ID
	nsid, err := strconv.ParseUint(metadata[toRBDMetadataKey(vcNamespaceID)], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace ID: %w", err)
	}

	listenerInfoPort, err := strconv.ParseUint(metadata[toRBDMetadataKey(vcListenerPort)], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid listener port: %w", err)
	}
	gatewayPort, err := strconv.ParseUint(metadata[toRBDMetadataKey(vcGatewayPort)], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway port: %w", err)
	}
	// Construct NVMe-oF volume data
	nvmeofData := &nvmeof.NVMeoFVolumeData{
		SubsystemNQN:  metadata[toRBDMetadataKey(vcSubsystemNQN)],
		NamespaceID:   uint32(nsid),
		NamespaceUUID: metadata[toRBDMetadataKey(vcNamespaceUUID)],
		HostNQN:       metadata[toRBDMetadataKey(vcHostNQN)],
		ListenerInfo: nvmeof.ListenerDetails{
			GatewayAddress: nvmeof.GatewayAddress{
				Address: metadata[toRBDMetadataKey(vcListenerAddress)],
				Port:    uint32(listenerInfoPort),
			},
			Hostname: metadata[toRBDMetadataKey(vcListenerHostname)],
		},
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
