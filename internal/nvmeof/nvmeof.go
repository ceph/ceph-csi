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

package nvmeof

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"syscall"

	pb "github.com/ceph/ceph-nvmeof/lib/go/nvmeof"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// GatewayAddress holds the address and port of the NVMe-oF gateway.
// This is used to connect to the gateway server.
// It is also used to specify the address and port of listeners.
type GatewayAddress struct {
	Address string `json:"address"`
	Port    uint32 `json:"port"`
}

func (ga GatewayAddress) String() string {
	return fmt.Sprintf("%s:%d", ga.Address, ga.Port)
}

// ListenerDetails holds the listener information for a subsystem.
type ListenerDetails struct {
	GatewayAddress
	Hostname string `json:"hostname"`
}

func (ga ListenerDetails) String() string {
	return fmt.Sprintf("%s:%d (%s)", ga.Address, ga.Port, ga.Hostname)
}

// Config holds gateway client configuration.
type GatewayConfig = GatewayAddress

// GatewayClient wraps the gRPC client and connection details.
type GatewayRpcClient struct {
	config    *GatewayConfig
	conn      *grpc.ClientConn
	client    pb.GatewayClient // protobuf client
	maxSerial *big.Int         // Maximum allowed serial number for subsystems.
}

// TODO: Move this to a util package?
const (
	MaxAllowedSubsystemSerialNumber = 99999999999999
	RandomNumberOffset              = 2 // skip reserved identifiers 0 and 1
)

// NewGatewayRpcClient creates a new gateway client.
func NewGatewayRpcClient(config *GatewayConfig) (*GatewayRpcClient, error) {
	client := &GatewayRpcClient{
		config:    config,
		maxSerial: big.NewInt(MaxAllowedSubsystemSerialNumber - RandomNumberOffset),
	}

	err := client.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gateway: %w", err)
	}

	return client, nil
}

// Destroy closes the gateway connection.
func (c *GatewayRpcClient) Destroy() error {
	var err error

	// Close connection if it exists
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}
	// Clear client reference
	c.client = nil
	// Clear config to prevent reuse
	c.config = nil

	return err
}

// CreateNamespace creates a namespace in a subsystem.
func (gw *GatewayRpcClient) CreateNamespace(ctx context.Context, subsystemNQN, poolName, imageName string,
) (uint32, error) {
	log.DebugLog(ctx, "Creating namespace for RBD %s/%s in subsystem %s",
		poolName, imageName, subsystemNQN)

	req := &pb.NamespaceAddReq{
		SubsystemNqn: subsystemNQN,
		RbdPoolName:  poolName,
		RbdImageName: imageName,
		BlockSize:    4096, // TODO: Make this configurable ??
		// Nsid:              nil,
		// Uuid:              nil,
		// Anagrpid:          nil,
		// CreateImage:       nil,
		// Size:              nil,
		// Force:             nil,
		// NoAutoVisible:     nil, // TODO: if we use only one subsystem for all volumes, we probably want to set this to true
		// TrashImage:        nil,
		// DisableAutoResize: nil,
		// ReadOnly:          nil,
	}

	resp, err := gw.client.NamespaceAdd(ctx, req)
	switch {
	case err != nil:
		return 0, fmt.Errorf("failed to create namespace for %s/%s: %w", poolName, imageName, err)
	case resp.GetStatus() == int32(syscall.EEXIST):
		return gw.findExistingNamespace(ctx, subsystemNQN, poolName, imageName)
	case resp.GetStatus() != 0:
		return 0, fmt.Errorf("gateway NamespaceAdd returned error: %s", resp.GetErrorMessage())
	}
	log.DebugLog(ctx, "Namespace created with NSID: %d", resp.GetNsid())

	return resp.GetNsid(), nil
}

// DeleteNamespace deletes a namespace from a subsystem.
func (gw *GatewayRpcClient) DeleteNamespace(ctx context.Context, subsystemNQN string, namespaceID uint32) error {
	log.DebugLog(ctx, "Deleting namespace %d from subsystem %s", namespaceID, subsystemNQN)

	req := &pb.NamespaceDeleteReq{
		SubsystemNqn: subsystemNQN,
		Nsid:         namespaceID,
	}

	status, err := gw.client.NamespaceDelete(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete namespace %d: %w", namespaceID, err)
	}
	if status.GetStatus() == 0 {
		log.DebugLog(ctx, "Namespace deleted successfully: %d", namespaceID)

		return nil
	}
	if status.GetStatus() == int32(syscall.ENOENT) { // ENOENT
		log.DebugLog(ctx, "Namespace %d already deleted (not found)", namespaceID)

		return nil // Namespace already deleted, no error
	}

	return fmt.Errorf("gateway NamespaceDelete returned error (status=%d): %s",
		status.GetStatus(), status.GetErrorMessage())
}

// GetUUIDBySubsystemAndNameSpaceID get the uuid of namespace by given subsystem and ns-id.
func (gw *GatewayRpcClient) GetUUIDBySubsystemAndNameSpaceID(
	ctx context.Context,
	subsystemNQN string,
	namespaceID uint32,
) (string, error) {
	req := &pb.ListNamespacesReq{
		Subsystem: subsystemNQN,
		Nsid:      &namespaceID,
	}
	resp, err := gw.client.ListNamespaces(ctx, req)
	if err != nil {
		return "", err
	}
	// Check if response is valid
	if resp == nil {
		return "", fmt.Errorf("received nil response from subsystem %s nsid %d", subsystemNQN, namespaceID)
	}

	// Check if any namespaces found
	if len(resp.GetNamespaces()) == 0 {
		return "", fmt.Errorf("no namespaces found for subsystem %s nsid %d", subsystemNQN, namespaceID)
	}
	// extra validation
	for _, ns := range resp.GetNamespaces() {
		if ns.GetNsid() == namespaceID {
			return ns.GetUuid(), nil
		}
	}

	return "", fmt.Errorf("namespace with nsid %d not found", namespaceID)
}

// CreateSubsystem creates an NVMe-oF subsystem on the gateway.
func (gw *GatewayRpcClient) CreateSubsystem(ctx context.Context, subsystemNQN string) error {
	log.DebugLog(ctx, "Creating NVMe subsystem: %s on gateway %s",
		subsystemNQN, gw.config)

	serialNumber, err := gw.generateSerialNumber()
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	req := &pb.CreateSubsystemReq{
		SubsystemNqn:  subsystemNQN,
		SerialNumber:  serialNumber,     // Generate serial from NQN
		EnableHa:      true,             // Enable HA (seems to be expected always true)
		NoGroupAppend: proto.Bool(true), // Do not append gateway group name to the NQN
		// MaxNamespaces: nil,   // Use gateway default
		// DhchapKey: nil,       // No authentication
		// KeyEncrypted: nil,    // No encryption
	}

	status, err := gw.client.CreateSubsystem(ctx, req)
	switch {
	case err != nil:
		return fmt.Errorf("failed to create subsystem %s: %w", subsystemNQN, err)
	case status.GetStatus() == int32(syscall.EEXIST):
		// subsystem was already created
		return nil
	case status.GetStatus() != 0:
		return fmt.Errorf("gateway CreateSubsystem returned error: %s", status.GetErrorMessage())
	}

	log.DebugLog(ctx, "Subsystem created successfully: %s", status.GetNqn())

	return nil
}

// DeleteSubsystem deletes an NVMe-oF subsystem.
func (gw *GatewayRpcClient) DeleteSubsystem(ctx context.Context, subsystemNQN string) error {
	log.DebugLog(ctx, "Deleting NVMe subsystem: %s", subsystemNQN)

	req := &pb.DeleteSubsystemReq{
		SubsystemNqn: subsystemNQN,
	}

	status, err := gw.client.DeleteSubsystem(ctx, req)
	switch {
	case err != nil:
		return fmt.Errorf("failed to delete subsystem %s: %w", subsystemNQN, err)
	case status.GetStatus() == int32(syscall.ENOENT):
		// the subsystem was removed already
		return nil
	case status.GetStatus() != 0:
		return fmt.Errorf("gateway DeleteSubsystem returned error: %s", status.GetErrorMessage())
	}

	log.DebugLog(ctx, "Subsystem deleted successfully: %s", subsystemNQN)

	return nil
}

func (gw *GatewayRpcClient) SubsystemExists(ctx context.Context, subsystemNQN string) (bool, error) {
	log.DebugLog(ctx, "Checking if subsystem %s exists on gateway %s", subsystemNQN, gw.config)

	req := &pb.ListSubsystemsReq{
		SubsystemNqn: nil,
	}
	resp, err := gw.client.ListSubsystems(ctx, req)
	if err != nil {
		return false, fmt.Errorf("failed to list subsystems: %w", err)
	}
	if resp.GetStatus() != 0 {
		return false, fmt.Errorf("gateway ListSubsystems returned error (status=%d): %s",
			resp.GetStatus(), resp.GetErrorMessage())
	}

	for _, sub := range resp.GetSubsystems() {
		if sub.GetNqn() == subsystemNQN {
			log.DebugLog(ctx, "Subsystem %s exists", subsystemNQN)

			return true, nil
		}
	}
	log.DebugLog(ctx, "Subsystem %s does not exist", subsystemNQN)

	return false, nil
}

// AddHost adds a host to a subsystem (allows access).
func (gw *GatewayRpcClient) AddHost(ctx context.Context, subsystemNQN, hostNQN string) error {
	log.DebugLog(ctx, "Adding host %s to subsystem %s on gateway %s",
		hostNQN, subsystemNQN, gw.config)

	req := &pb.AddHostReq{
		SubsystemNqn: subsystemNQN,
		HostNqn:      hostNQN,
		// Psk: nil,          // No pre-shared key
		// DhchapKey: nil,    // No DH-CHAP authentication
		// PskEncrypted: nil, // No PSK encryption
		// KeyEncrypted: nil, // No key encryption
	}

	resp, err := gw.client.AddHost(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to add host %s to subsystem %s: %w", hostNQN, subsystemNQN, err)
	}
	if resp.GetStatus() == 0 {
		log.DebugLog(ctx, "Host added successfully: %s to subsystem %s", hostNQN, subsystemNQN)

		return nil
	}
	if resp.GetStatus() == int32(syscall.EEXIST) { // EEXIST
		log.DebugLog(ctx, "Host %s already added to subsystem %s", hostNQN, subsystemNQN)

		return nil // Host already added, no error
	}

	return fmt.Errorf("gateway AddHost returned error (status=%d): %s", resp.GetStatus(), resp.GetErrorMessage())
}

func (gw *GatewayRpcClient) CreateListener(ctx context.Context, subsystemNQN string, listenerInfo ListenerDetails,
) error {
	log.DebugLog(ctx, "Adding listener %s to subsystem %s", listenerInfo.Address, subsystemNQN)
	adrfam := pb.AddressFamily_ipv4
	req := &pb.CreateListenerReq{
		Nqn:      subsystemNQN,
		HostName: listenerInfo.Hostname,
		Traddr:   listenerInfo.Address,
		Trsvcid:  &listenerInfo.Port,
		Adrfam:   &adrfam, // Assuming IPv4, can be configurable // TODO - make it configurable
		// Secure  // false, // Assuming no security for now   // TODO - make it configurable?
		// VerifyHostName // false, // Assuming no hostname verification for now // TODO - make it configurable
	}

	resp, err := gw.client.CreateListener(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to add listener %s to subsystem %s: %w", listenerInfo.Address, subsystemNQN, err)
	}
	switch resp.GetStatus() {
	case int32(syscall.EEXIST):
		log.DebugLog(ctx, "Listener %s already created for subsystem %s", listenerInfo.Address, subsystemNQN)

		return nil // Listener already created, no error
	case int32(syscall.EREMOTE): // Handle the stashed listener case
		log.DebugLog(ctx, "Listener %s stashed for subsystem %s (will be active when %s gateway comes up)",
			listenerInfo.Address, subsystemNQN, listenerInfo.Hostname)

		return nil // Treat as success
	case 0:
		// break
	default: // resp.GetStatus() != 0
		return fmt.Errorf("gateway AddListener returned error (status=%d): %s", resp.GetStatus(), resp.GetErrorMessage())
	}
	log.DebugLog(ctx, "Listener added successfully: %s to subsystem %s", listenerInfo.Address, subsystemNQN)

	return nil
}

// DeleteListener removes a listener from a subsystem.
func (gw *GatewayRpcClient) DeleteListener(ctx context.Context, subsystemNQN string, listenerInfo ListenerDetails,
) error {
	log.DebugLog(ctx, "Deleting listener %s from subsystem %s", listenerInfo.Address, subsystemNQN)
	adrfam := pb.AddressFamily_ipv4
	// Add this because without this, the gw return
	// "Can't verify there are no active connections for this address" instead of "Listener not found"
	force := true
	req := &pb.DeleteListenerReq{
		Nqn:      subsystemNQN,
		HostName: listenerInfo.Hostname,
		Traddr:   listenerInfo.Address,
		Trsvcid:  &listenerInfo.Port,
		Adrfam:   &adrfam, // Assuming IPv4, can be configurable // TODO - make it configurable
		Force:    &force,
	}

	resp, err := gw.client.DeleteListener(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete listener %s from subsystem %s: %w", listenerInfo.Address, subsystemNQN, err)
	}
	if resp.GetStatus() == 0 {
		log.DebugLog(ctx, "Listener deleted successfully: %s from subsystem %s", listenerInfo.Address, subsystemNQN)

		return nil
	}
	if resp.GetStatus() == int32(syscall.ENOENT) { // ENOENT
		log.DebugLog(ctx, "Listener %s already deleted from subsystem %s (not found)", listenerInfo.Address, subsystemNQN)

		return nil // Listener already deleted, no error
	}

	return fmt.Errorf("gateway DeleteListener returned error (status=%d): %s", resp.GetStatus(), resp.GetErrorMessage())
}

// RemoveHost removes a host from a subsystem.
func (gw *GatewayRpcClient) RemoveHost(ctx context.Context, subsystemNQN, hostNQN string) error {
	log.DebugLog(ctx, "Removing host %s from subsystem %s", hostNQN, subsystemNQN)

	req := &pb.RemoveHostReq{
		SubsystemNqn: subsystemNQN,
		HostNqn:      hostNQN,
	}

	resp, err := gw.client.RemoveHost(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to remove host %s from subsystem %s: %w", hostNQN, subsystemNQN, err)
	}
	if resp.GetStatus() == 0 {
		log.DebugLog(ctx, "Host %s removed successfully from subsystem %s", hostNQN, subsystemNQN)

		return nil
	}
	if resp.GetStatus() == int32(syscall.ENOENT) { // ENOENT
		log.DebugLog(ctx, "Host %s already removed from subsystem %s (not found)", hostNQN, subsystemNQN)

		return nil
	}

	return fmt.Errorf("gateway RemoveHost returned error (status=%d): %s", resp.GetStatus(), resp.GetErrorMessage())
}

// List namespaces in a subsystem.
func (gw *GatewayRpcClient) ListNamespaces(ctx context.Context, subsystemNQN string) (*pb.NamespacesInfo, error) {
	log.DebugLog(ctx, "Listing namespaces in subsystem %s", subsystemNQN)

	req := &pb.ListNamespacesReq{
		Subsystem: subsystemNQN,
	}

	resp, err := gw.client.ListNamespaces(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces in subsystem %s: %w", subsystemNQN, err)
	}
	if resp.GetStatus() != 0 {
		return nil, fmt.Errorf("gateway ListNamespaces returned error: %s", resp.GetErrorMessage())
	}

	log.DebugLog(ctx, "Listed namespaces in subsystem %s successfully", subsystemNQN)

	return resp, nil
}

// Connect to Gateway gRPC server.
func (c *GatewayRpcClient) connect() error {
	// Create connection using new gRPC API
	conn, err := grpc.NewClient(c.config.String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create Gateway gRPC client: %w", err)
	}
	c.conn = conn
	c.client = pb.NewGatewayClient(conn)

	return nil
}

// TODO- maybe move to util-nvmeof package?
func (c *GatewayRpcClient) generateSerialNumber() (string, error) {
	n, err := rand.Int(rand.Reader, c.maxSerial)
	if err != nil {
		return "", fmt.Errorf("failed to generate random serial: %w", err)
	}

	serial := n.Int64() + RandomNumberOffset

	return fmt.Sprintf("Ceph%d", serial), nil
}

// findExistingNamespace searches for a namespace matching the given pool and image names
// Returns the NSID if found, or an error if not found or on failure.
func (gw *GatewayRpcClient) findExistingNamespace(
	ctx context.Context,
	subsystemNQN,
	poolName,
	imageName string,
) (uint32, error) {
	r, err := gw.client.ListNamespaces(ctx, &pb.ListNamespacesReq{Subsystem: subsystemNQN})
	if err != nil {
		return 0, fmt.Errorf("failed to get namespaces in subsystem %s: %w", subsystemNQN, err)
	}

	if r.GetStatus() != 0 {
		return 0, fmt.Errorf("could not get namespaces in subsystem %s (status=%d): %s",
			subsystemNQN, r.GetStatus(), r.GetErrorMessage())
	}

	if len(r.GetNamespaces()) == 0 {
		return 0, fmt.Errorf("no existing namespaces in subsystem %s", subsystemNQN)
	}

	for _, ns := range r.GetNamespaces() {
		if ns.GetRbdPoolName() == poolName && ns.GetRbdImageName() == imageName {
			return ns.GetNsid(), nil
		}
	}

	return 0, fmt.Errorf("could not find existing namespace for %s/%s in subsystem %s",
		poolName, imageName, subsystemNQN)
}
