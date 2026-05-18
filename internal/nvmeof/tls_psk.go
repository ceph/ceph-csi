/*
Copyright 2026 The Ceph-CSI Authors.

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
	"errors"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
)

// TLS-PSK Key Storage Architecture
//
// TLS-PSK keys are stored in a DEKStore (Data Encryption Key Store) using a structured key ID format.
// The DEKStore can be backed by different storage systems:
//   - RBD metadata (for testing) - stored as RBD image metadata key-value pairs
//   - External KMS like Vault (for production) - stored in the KMS system
//
// Key ID Format:
//   <prefix>-<nodeID>-<subsystemHash>
//
// Components:
//   prefix:         "nvmeof-tls-psk"
//   nodeID:         Kubernetes node name (e.g., "worker-node-1")
//   subsystemHash:  First 16 characters of SHA-256 hash of subsystem NQN
//
// Examples:
//   TLS-PSK key:    nvmeof-tls-psk-worker-node-1-a1b2c3d4e5f6g7h8
//
// Key Value Format:
//   The value stored is the TLS-PSK in NVMe PSK Interchange format:
//   NVMeTLSkey-1:<hash_type>:<base64_encoded_key_with_crc>:
//
//   Example: NVMeTLSkey-1:01:lLyldXckJcsT8tGnxG00BsR2kHsK/92ygXcRbYXTC8jekqdm:
//
// Storage Flow:
//   1. Generate PSK using nvme gen-tls-key command
//   2. Encrypt key using KMS (EncryptDEK)
//   3. Store encrypted key in DEKStore with key ID
//
// Retrieval Flow:
//   1. Fetch encrypted key from DEKStore using key ID
//   2. Decrypt key using KMS (DecryptDEK)
//   3. Return plaintext TLS-PSK for nvme connect
//

// TLS-PSK modes.
const (
	TLSPSKEmpty   = ""        // when no TLS-PSK is provided
	TLSPSKNone    = "none"    // when TLS-PSK is explicitly disabled
	TLSPSKEnabled = "enabled" // when TLS-PSK is enabled
)

// TLS-PSK specific constants.
const (
	// Key prefix for DEKStore key IDs.
	keyPrefixTLSPSK = "nvmeof-tls-psk"
)

// GetOrCreateTLSPSKKey retrieves existing PSK or creates new one if not found.
// This function follows the same pattern as GetOrCreateDHCHAPHostKey.
func GetOrCreateTLSPSKKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID,
	subsystemNQN string,
) (string, error) {
	// Try to get existing key
	pskKey, err := GetTLSPSKKey(ctx, skm, nodeID, subsystemNQN)
	if err == nil {
		// Key exists, return it
		return pskKey, nil
	}

	// TODO: When another KMS implementation is added, check what kind of error it
	// returns when a key is not found and handle that here instead of relying on
	// ErrKeyNotFound. For now, since we only have the RBD DEKStore, we can check
	// for ErrKeyNotFound, which is returned by the RBD DEKStore when a key is not found.

	// Only create if truly not found - not on any other error.
	if !errors.Is(err, ErrKeyNotFound) {
		// Real error (KMS down, network issue etc) - don't generate new key
		return "", fmt.Errorf("failed to check existing TLS-PSK key: %w", err)
	}

	// Key doesn't exist, generate new one
	pskKey, err = generateAndStoreTLSPSKKey(ctx, skm, nodeID, subsystemNQN)
	if err != nil {
		return "", fmt.Errorf("failed to generate TLS-PSK key: %w", err)
	}

	return pskKey, nil
}

// RemoveTLSPSKKey removes the TLS-PSK key for the given node and subsystem connection.
func RemoveTLSPSKKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
) error {
	keyID := buildTLSPSKKeyID(nodeID, subsystemNQN)

	return skm.RemoveKey(ctx, keyID)
}

// generateAndStoreTLSPSKKey generates a new TLS-PSK key and stores it in the DEKStore.
//
// Key generation and storage flow:
//  1. Build key ID: nvmeof-tls-psk-<nodeID>-<subsystemHash>
//  2. Generate TLS-PSK using nvme gen-tls-key command
//  3. Encrypt key using KMS
//  4. Store encrypted key in DEKStore with key ID
//
// Example key ID: nvmeof-tls-psk-worker-node-1-a1b2c3d4e5f6g7h8.
func generateAndStoreTLSPSKKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID,
	subsystemNQN string,
) (string, error) {
	keyID := buildTLSPSKKeyID(nodeID, subsystemNQN)
	pskKey, err := generateTLSPSKKey(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to generate TLS-PSK key for %s: %w", keyID, err)
	}
	err = skm.StoreKey(ctx, keyID, pskKey)
	if err != nil {
		return "", fmt.Errorf("failed to store TLS-PSK key for %s: %w", keyID, err)
	}

	return pskKey, nil
}

// generateTLSPSKKey generates a TLS-PSK key using nvme-cli gen-tls-key command.
// This generates a TLS-PSK in NVMe PSK Interchange format:
// NVMeTLSkey-1:<hash_type>:<base64_encoded_key_with_crc>:
//
// The format includes:
//   - NVMeTLSkey-1: Protocol identifier
//   - hash_type: 01 (SHA-256), 02 (SHA-384)
//   - base64_encoded_key_with_crc: Base64 of (key_bytes + crc_checksum)
//
// Example output: NVMeTLSkey-1:01:lLyldXckJcsT8tGnxG00BsR2kHsK/92ygXcRbYXTC8jekqdm: .
func generateTLSPSKKey(ctx context.Context) (string, error) {
	args := []string{
		"gen-tls-key",
	}

	stdout, stderr, err := util.ExecCommandWithTimeout(ctx, connectTimeout, "nvme", args...)
	if err != nil {
		// Command doesn't exist or failed
		return "", fmt.Errorf("nvme gen-tls-key command failed: %w (stderr: %s)", err, stderr)
	}

	// Parse output - nvme gen-tls-key outputs the key in format:
	// NVMeTLSkey-1:01:base64key:
	key := strings.TrimSpace(stdout)
	if key == "" {
		return "", errors.New("generated TLS-PSK key is empty")
	}

	return key, nil
}

// buildTLSPSKKeyID constructs a unique key ID for DEKStore storage.
//
// Format: nvmeof-tls-psk-<nodeID>-<subsystemHash>
//
// Components:
//   - prefix: "nvmeof-tls-psk"
//   - nodeID: Kubernetes node name
//   - subsystemHash: First 16 chars of SHA-256 hash of subsystem NQN
//
// Examples:
//
//	buildTLSPSKKeyID("worker-1", "nqn.2016-06.io.spdk:cnode1")
//	 "nvmeof-tls-psk-worker-1-a1b2c3d4e5f6g7h8"
func buildTLSPSKKeyID(
	nodeID string,
	subsystemNQN string,
) string {
	subsystemHash := hashSubsystemNQN(subsystemNQN)

	return fmt.Sprintf("%s-%s-%s", keyPrefixTLSPSK, nodeID, subsystemHash)
}

// GetTLSPSKKey retrieves the TLS-PSK key from the DEKStore (get-only, no creation).
// Returns ErrKeyNotFound if the key doesn't exist.
// This should be used on the node side to prevent PSK regeneration mismatches.
func GetTLSPSKKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
) (string, error) {
	keyID := buildTLSPSKKeyID(nodeID, subsystemNQN)

	return skm.GetKey(ctx, keyID)
}
