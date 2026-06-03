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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// DH-CHAP Key Storage Architecture
//
// DH-CHAP keys are stored in a DEKStore (Data Encryption Key Store) using a structured key ID format.
// The DEKStore can be backed by different storage systems:
//   - RBD metadata (for testing) - stored as RBD image metadata key-value pairs
//   - External KMS like Vault (for production) - stored in the KMS system
//
// Key ID Format:
//   <prefix>-<nodeID>-<subsystemHash>
//
// Components:
//   prefix:         Either "nvmeof-dhchap-host" or "nvmeof-dhchap-subsystem"
//   nodeID:         Kubernetes node name (e.g., "worker-node-1")
//   subsystemHash:  First 16 characters of SHA-256 hash of subsystem NQN
//
// Examples:
//   Host key:       nvmeof-dhchap-host-worker-node-1-a1b2c3d4e5f6g7h8
//   Subsystem key:  nvmeof-dhchap-subsystem-worker-node-1-a1b2c3d4e5f6g7h8
//
// Key Value Format:
//   The value stored is the DH-CHAP key in nvme-cli format:
//   DHHC-1:<hash_type>:<base64_encoded_key_with_crc32>:
//
//   Example: DHHC-1:01:tY6/W/5reir8GG6AYcHmRjhEj3l4UMt4GgG9nq+plT0NsIEu:
//
// Storage Flow:
//   1. Generate key using nvme gen-dhchap-key
//   2. Encrypt key using KMS (EncryptDEK)
//   3. Store encrypted key in DEKStore with key ID
//
// Retrieval Flow:
//   1. Fetch encrypted key from DEKStore using key ID
//   2. Decrypt key using KMS (DecryptDEK)
//   3. Return plaintext DH-CHAP key for nvme connect
//

// DH-CHAP modes.
const (
	DHCHAPEmpty              = ""               // when no DH-CHAP is provided
	DHCHAPModeNone           = "none"           // when the provided DH-CHAP is explicitly 'none'
	DHCHAPModeUniDirectional = "unidirectional" // when only host has DH-CHAP
	DHCHAPModeBiDirectional  = "bidirectional"  // when both host and subsystem have DH-CHAP
)

// DH-CHAP specific constants.
// The DH-CHAP key format is:
//
//	DHHC-1:<hash_type>:<base64_encoded_key_with_crc32>:
const (
	DHCHAPHashNone   = 0
	DHCHAPHashSHA256 = 1
	DHCHAPHashSHA384 = 2
	DHCHAPHashSHA512 = 3

	defaultDHCHAPKeySize = 32
	defaultDHCHAPHash    = DHCHAPHashSHA256

	// Key prefixes for DEKStore key IDs.
	keyPrefixDHCHAPHost      = "nvmeof-dhchap-host"
	keyPrefixDHCHAPSubsystem = "nvmeof-dhchap-subsystem"

	// subsystemHashLength is the number of hex characters to use from the SHA-256 hash
	// of the subsystem NQN when building key IDs. 16 chars = 64 bits = sufficient uniqueness.
	subsystemHashLength = 16
)

// DHCHAPKeys holds the DH-CHAP keys for host and subsystem.
type DHCHAPKeys struct {
	HostKey      string // DH-CHAP host key
	SubsystemKey string // DH-CHAP subsystem key
}

// GetOrCreateDHCHAPHostKey retrieves existing key or creates new one if not found.
func GetOrCreateDHCHAPHostKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID,
	subsystemNQN,
	hostNQN string,
) (string, error) {
	// Try to get existing key
	hostKey, err := getDHCHAPHostKey(ctx, skm, nodeID, subsystemNQN)
	if err == nil {
		// Key exists, return it
		return hostKey, nil
	}

	// TODO - when another KMS implementation is added, we should check what kind of error it returns
	// when key is not found and check for that here instead of ErrKeyNotFound.!!!
	// for now , since we have only RBD DEKStore, we can check for ErrKeyNotFound which
	// is returned by RBD DEKStore when key is not found.

	// Only create if truly not found - not on any other error.
	if !errors.Is(err, ErrKeyNotFound) {
		// Real error (KMS down, network issue etc) - don't generate new key
		return "", fmt.Errorf("failed to check existing host key: %w", err)
	}

	// Key doesn't exist, generate new one
	dhchapKey, err := generateAndStoreDHCHAPKey(ctx, skm, keyPrefixDHCHAPHost, nodeID, subsystemNQN, hostNQN)
	if err != nil {
		return "", fmt.Errorf("failed to generate host key: %w", err)
	}

	// Retrieve the newly generated key
	return dhchapKey, nil
}

// GetOrCreateDHCHAPSubsystemKey retrieves existing key or creates new one if not found.
func GetOrCreateDHCHAPSubsystemKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
	hostNQN string,
) (string, error) {
	// Try to get existing key
	subsystemKey, err := getDHCHAPSubsystemKey(ctx, skm, nodeID, subsystemNQN)
	if err == nil {
		// Key exists, return it
		return subsystemKey, nil
	}

	// TODO - when another KMS implementation is added, we should check what kind of error it returns
	// when key is not found and check for that here instead of ErrKeyNotFound.!!!
	// for now , since we have only RBD DEKStore, we can check for ErrKeyNotFound which
	// is returned by RBD DEKStore when key is not found.

	// Only create if truly not found - not on any other error.
	if !errors.Is(err, ErrKeyNotFound) {
		// Real error (KMS down, network issue etc) - don't generate new key
		return "", fmt.Errorf("failed to check existing subsystem key: %w", err)
	}

	// Key doesn't exist, generate new one
	dhchapKey, err := generateAndStoreDHCHAPKey(ctx, skm, keyPrefixDHCHAPSubsystem, nodeID, subsystemNQN, hostNQN)
	if err != nil {
		return "", fmt.Errorf("failed to generate subsystem key: %w", err)
	}

	return dhchapKey, nil
}

// RemoveDHCHAPHostKey removes the DH-CHAP host key for the given node and subsystem connection.
func RemoveDHCHAPHostKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
) error {
	keyID := buildDHCHAPKeyID(keyPrefixDHCHAPHost, nodeID, subsystemNQN)

	return skm.RemoveKey(ctx, keyID)
}

// RemoveDHCHAPSubsystemKey removes the DH-CHAP subsystem key for the given node and subsystem connection.
func RemoveDHCHAPSubsystemKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
) error {
	keyID := buildDHCHAPKeyID(keyPrefixDHCHAPSubsystem, nodeID, subsystemNQN)

	return skm.RemoveKey(ctx, keyID)
}

// hashSubsystemNQN creates a short hash of the subsystem NQN for use in key IDs.
// Returns the first 16 characters (64 bits) of the SHA-256 hash as hex string.
//
// Example:
//
//	Input:  nqn.2016-06.io.spdk:cnode1
//	Output: a1b2c3d4e5f6g7h8
func hashSubsystemNQN(subsystemNQN string) string {
	h := sha256.Sum256([]byte(subsystemNQN))

	return hex.EncodeToString(h[:])[:subsystemHashLength]
}

// generateAndStoreDHCHAPKey generates a new DH-CHAP key and stores it in the DEKStore.
//
// Key generation and storage flow:
//  1. Build key ID: <prefix>-<nodeID>-<subsystemHash>
//  2. Generate DH-CHAP key using nvme gen-dhchap-key command
//  3. Encrypt key using KMS
//  4. Store encrypted key in DEKStore with key ID
//
// Example key ID: nvmeof-dhchap-host-worker-node-1-a1b2c3d4e5f6g7h8.
func generateAndStoreDHCHAPKey(
	ctx context.Context,
	skm SecurityKeyManager,
	prefix,
	nodeID,
	subsystemNQN,
	hostNQN string,
) (string, error) {
	keyID := buildDHCHAPKeyID(prefix, nodeID, subsystemNQN)
	dhchapKey, err := generateDHCHAPKey(ctx, defaultDHCHAPKeySize, defaultDHCHAPHash, hostNQN)
	if err != nil {
		return "", fmt.Errorf("failed to generate DH-CHAP key for %s: %w", keyID, err)
	}
	err = skm.StoreKey(ctx, keyID, dhchapKey)
	if err != nil {
		return "", fmt.Errorf("failed to store DH-CHAP key for %s: %w", keyID, err)
	}

	return dhchapKey, nil
}

// generateDHCHAPKey generates a DH-CHAP key using the nvme-cli gen-dhchap-key command.
// This generates a DH-CHAP key in the format: DHHC-1:<hash_type>:<base64_encoded_key_with_crc32>:
// According to NVMe-oF spec: NVM Express® Base Specification, Revision 2.3.
//
// The key format includes:
//   - DHHC-1: Protocol identifier
//   - hash_type: 01 (SHA-256), 02 (SHA-384), or 03 (SHA-512)
//   - base64_encoded_key_with_crc32: Base64 of (key_bytes + crc32_checksum)
//
// Example output: DHHC-1:01:tY6/W/5reir8GG6AYcHmRjhEj3l4UMt4GgG9nq+plT0NsIEu: .
func generateDHCHAPKey(ctx context.Context, keyLength, hashAlgorithm int, hostNQN string) (string, error) {
	// Validate
	if hashAlgorithm < DHCHAPHashNone || hashAlgorithm > DHCHAPHashSHA512 {
		return "", fmt.Errorf("invalid hash algorithm: %d", hashAlgorithm)
	}

	// Validate key length for specific hash
	switch hashAlgorithm {
	case DHCHAPHashSHA256:
		if keyLength != 32 {
			return "", fmt.Errorf("SHA-256 requires 32 byte key, got %d", keyLength)
		}
	case DHCHAPHashSHA384:
		if keyLength != 48 {
			return "", fmt.Errorf("SHA-384 requires 48 byte key, got %d", keyLength)
		}
	case DHCHAPHashSHA512:
		if keyLength != 64 {
			return "", fmt.Errorf("SHA-512 requires 64 byte key, got %d", keyLength)
		}
	}

	// Generate random key bytes
	keyBytes := make([]byte, keyLength)
	_, err := rand.Read(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Build nvme gen-dhchap-key command
	args := []string{
		"gen-dhchap-key",
		"-n", hostNQN,
		"-l", strconv.Itoa(keyLength),
		"-m", strconv.Itoa(hashAlgorithm),
	}

	stdout, stderr, err := util.ExecCommandWithTimeout(ctx, connectTimeout, "nvme", args...)
	// Execute connection
	if err != nil {
		log.ErrorLog(ctx, "failed to generate DH-CHAP key: %s: %s", err.Error(), stderr)

		return "", fmt.Errorf("nvme gen-dhchap-key command failed: %w", err)
	}

	// Parse output - nvme gen-dhchap-key outputs the key in format:
	// DHHC-1:01:base64key:
	key := strings.TrimSpace(stdout)
	if key == "" {
		return "", errors.New("generated DH-CHAP key is empty")
	}

	return key, nil
}

// buildDHCHAPKeyID constructs a unique key ID for DEKStore storage.
//
// Format: <prefix>-<nodeID>-<subsystemHash>
//
// Components:
//   - prefix: "nvmeof-dhchap-host" or "nvmeof-dhchap-subsystem"
//   - nodeID: Kubernetes node name
//   - subsystemHash: First 16 chars of SHA-256 hash of subsystem NQN
//
// Examples:
//
//	buildDHCHAPKeyID("nvmeof-dhchap-host", "worker-1", "nqn.2016-06.io.spdk:cnode1")
//	 "nvmeof-dhchap-host-worker-1-a1b2c3d4e5f6g7h8"
func buildDHCHAPKeyID(
	prefix string,
	nodeID string,
	subsystemNQN string,
) string {
	subsystemHash := hashSubsystemNQN(subsystemNQN)

	return fmt.Sprintf("%s-%s-%s", prefix, nodeID, subsystemHash)
}

func getDHCHAPHostKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
) (string, error) {
	keyID := buildDHCHAPKeyID(keyPrefixDHCHAPHost, nodeID, subsystemNQN)
	// Returns DH-CHAP formatted string: "DHHC-1:01:..."

	return skm.GetKey(ctx, keyID)
}

func getDHCHAPSubsystemKey(
	ctx context.Context,
	skm SecurityKeyManager,
	nodeID string,
	subsystemNQN string,
) (string, error) {
	keyID := buildDHCHAPKeyID(keyPrefixDHCHAPSubsystem, nodeID, subsystemNQN)

	return skm.GetKey(ctx, keyID)
}
