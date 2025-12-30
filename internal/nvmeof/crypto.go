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

	"github.com/ceph/ceph-csi/internal/kms"
)

var (
	// ErrDEKStoreNotSet is returned when attempting to use StoreKey/GetKey/RemoveKey
	// before configuring a DEKStore via SetDEKStore(). This indicates a programming
	// error - the caller must set the DEKStore before performing key operations.
	ErrDEKStoreNotSet = errors.New("DEKStore not found for security keys")

	// ErrDEKStoreNeeded is returned by InitSecurityKeyManager when the KMS backend
	// requires external storage (like metadata KMS). The caller must configure a
	// DEKStore by calling SetDEKStore() before using StoreKey/GetKey/RemoveKey.
	// This is expected behavior for certain KMS types and not an error condition.
	ErrDEKStoreNeeded = errors.New("DEKStore required, use SetDEKStore()")

	// ErrKeyNotFound is returned when a requested key is not found in the DEKStore.
	ErrKeyNotFound = errors.New("key not found in DEKStore")
)

const (
	// NVMeOFSecurityOwner is the identifier used when requesting KMS instances
	// for NVMe-oF authentication key management.
	NVMeOFSecurityOwner = "nvmeof-system"
)

// SecurityKeyManager defines the interface for managing NVMe-oF security keys.
// It provides encrypted storage and retrieval of authentication keys (like DH-CHAP keys)
// using a KMS (Key Management System) for encryption and a DEKStore for persistence.
type SecurityKeyManager interface {
	// StoreKey encrypts and stores a plaintext key.
	StoreKey(ctx context.Context, keyID string, plainKey string) error
	// GetKey retrieves and decrypts a stored key.
	GetKey(ctx context.Context, keyID string) (string, error)
	// RemoveKey deletes a stored key.
	RemoveKey(ctx context.Context, keyID string) error
	// SetDEKStore sets the storage backend for encrypted keys.
	// Required for KMS implementations that don't have integrated storage (like metadata KMS).
	SetDEKStore(dekStore kms.DEKStore)
	// GetID returns the KMS identifier.
	GetID() string
	// Destroy releases resources held by the manager.
	Destroy()
}

// SecurityKeyManager manages NVMe-oF authentication keys
// Similar to VolumeEncryption, but for authentication instead of disk encryption.
type securityKeyManager struct {
	// KMS instance for key operations
	kms kms.EncryptionKMS

	// Where to store encrypted keys (if KMS doesn't store internally)
	dekStore kms.DEKStore

	// KMS identifier
	id string
}

// InitSecurityKeyManager creates a new SecurityKeyManager instance.
// If kmsID is empty, defaults to "metadata" KMS which stores keys in RBD image metadata.
// Returns ErrDEKStoreNeeded if the KMS requires external storage (caller must call SetDEKStore).
func InitSecurityKeyManager(
	ctx context.Context,
	kmsID string, // From volume context
	credentials map[string]string, // From K8s Secret
) (SecurityKeyManager, error) {
	if kmsID == "" {
		// Use metadata KMS for testing (stores in RBD metadata)
		kmsID = "metadata"
	}
	kmsInstance, err := kms.GetKMS(NVMeOFSecurityOwner, kmsID, credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to get KMS: %w", err)
	}

	securityKeys, err := newSecurityKeyManager(kmsID, kmsInstance)
	if errors.Is(err, ErrDEKStoreNeeded) {
		return securityKeys, err
	} else if err != nil {
		return nil, err
	}

	return securityKeys, nil
}

// newSecurityKeyManager creates a new securityKeyManager instance.
// If KMS has integrated storage (like Vault), it's configured automatically.
// Otherwise, returns ErrDEKStoreNeeded and caller must provide DEKStore via SetDEKStore.
func newSecurityKeyManager(
	kmsID string,
	ekms kms.EncryptionKMS,
) (SecurityKeyManager, error) {
	skm := &securityKeyManager{
		id:  kmsID,
		kms: ekms,
	}

	// Check if KMS stores keys internally (like Vault)
	if ekms.RequiresDEKStore() == kms.DEKStoreIntegrated {
		// KMS implements DEKStore, use it
		dekStore, ok := ekms.(kms.DEKStore)
		if !ok {
			return nil, fmt.Errorf("KMS %T does not implement DEKStore", ekms)
		}
		skm.dekStore = dekStore

		return skm, nil
	}

	// KMS needs external storage (metadata KMS)
	return skm, ErrDEKStoreNeeded
}

func (skm *securityKeyManager) SetDEKStore(dekStore kms.DEKStore) {
	skm.dekStore = dekStore
}

func (skm *securityKeyManager) GetID() string {
	return skm.id
}

// Destroy frees any resources that the SecurityKeyManager instance allocated.
func (skm *securityKeyManager) Destroy() {
	skm.kms.Destroy()
}

// StoreKey encrypts the plaintext key using KMS and stores it in the DEKStore.
func (skm *securityKeyManager) StoreKey(
	ctx context.Context,
	keyID string,
	plainKey string,
) error {
	if skm.dekStore == nil {
		return ErrDEKStoreNotSet
	}
	// Step 1: Encrypt key with KMS
	encryptedKey, err := skm.kms.EncryptDEK(ctx, keyID, plainKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt key for %s: %w", keyID, err)
	}

	// Step 2: Store encrypted key in DEKStore
	err = skm.dekStore.StoreDEK(ctx, keyID, encryptedKey)
	if err != nil {
		return fmt.Errorf("failed to save key for %s: %w", keyID, err)
	}

	return nil
}

// GetKey retrieves the encrypted key from DEKStore and decrypts it using KMS.
func (skm *securityKeyManager) GetKey(
	ctx context.Context,
	keyID string,
) (string, error) {
	if skm.dekStore == nil {
		return "", ErrDEKStoreNotSet
	}
	// Step 1: Fetch encrypted key from DEKStore
	encryptedKey, err := skm.dekStore.FetchDEK(ctx, keyID)

	if errors.Is(err, ErrKeyNotFound) {
		return "", fmt.Errorf("%w: %s", ErrKeyNotFound, keyID)
	}

	if err != nil {
		return "", fmt.Errorf("failed to fetch key %s: %w", keyID, err)
	}

	// Step 2: Decrypt key with KMS
	plainKey, err := skm.kms.DecryptDEK(ctx, keyID, encryptedKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt key %s: %w", keyID, err)
	}

	return plainKey, nil
}

// RemoveKey deletes the key from the DEKStore.
func (skm *securityKeyManager) RemoveKey(
	ctx context.Context,
	keyID string,
) error {
	if skm.dekStore == nil {
		return ErrDEKStoreNotSet
	}

	return skm.dekStore.RemoveDEK(ctx, keyID)
}
