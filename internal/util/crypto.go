/*
Copyright 2019 The Ceph-CSI Authors.

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

package util

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	mapperFilePrefix     = "luks-rbd-"
	mapperFilePathPrefix = "/dev/mapper"

	// Passphrase size - 20 bytes is 160 bits to satisfy:
	// https://tools.ietf.org/html/rfc6749#section-10.10
	defaultEncryptionPassphraseSize = 20
)

var (
	// ErrDEKStoreNotFound is an error that is returned when the DEKStore
	// has not been configured for the volumeID in the KMS instance.
	ErrDEKStoreNotFound = errors.New("DEKStore not found")

	// ErrDEKStoreNeeded is an indication that gets returned with
	// NewVolumeEncryption when the KMS does not include support for the
	// DEKStore interface.
	ErrDEKStoreNeeded = errors.New("DEKStore required, use " +
		"VolumeEncryption.SetDEKStore()")
)

type VolumeEncryption struct {
	KMS kms.EncryptionKMS

	// dekStore that will be used, this can be the EncryptionKMS or a
	// different object implementing the DEKStore interface.
	dekStore kms.DEKStore

	id string
}

// FetchEncryptionKMSID returns non-empty kmsID if 'encrypted' parameter is evaluated as true.
func FetchEncryptionKMSID(encrypted, kmsID string) (string, error) {
	isEncrypted, err := strconv.ParseBool(encrypted)
	if err != nil {
		return "", fmt.Errorf(
			"invalid value set in 'encrypted': %s (should be \"true\" or \"false\"): %w",
			encrypted, err)
	}
	if !isEncrypted {
		return "", nil
	}

	if kmsID == "" {
		kmsID = kms.DefaultKMSType
	}

	return kmsID, nil
}

type EncryptionType int

const (
	// EncryptionTypeInvalid signals invalid or unsupported configuration.
	EncryptionTypeInvalid EncryptionType = iota
	// EncryptionTypeNone disables encryption.
	EncryptionTypeNone
	// EncryptionTypeBlock enables block encryption.
	EncryptionTypeBlock
	// EncryptionTypeBlock enables file encryption (fscrypt).
	EncryptionTypeFile
)

const (
	encryptionTypeBlockString = "block"
	encryptionTypeFileString  = "file"
)

func ParseEncryptionType(typeStr string) EncryptionType {
	switch typeStr {
	case encryptionTypeBlockString:
		return EncryptionTypeBlock
	case encryptionTypeFileString:
		return EncryptionTypeFile
	case "":
		return EncryptionTypeNone
	default:
		return EncryptionTypeInvalid
	}
}

func EncryptionTypeString(encType EncryptionType) string {
	switch encType {
	case EncryptionTypeBlock:
		return encryptionTypeBlockString
	case EncryptionTypeFile:
		return encryptionTypeFileString
	case EncryptionTypeNone:
		return ""
	case EncryptionTypeInvalid:
		return "INVALID"
	default:
		return "UNKNOWN"
	}
}

// FetchEncryptionType returns encryptionType specified in volOptions.
// If not specified, use fallback. If specified but invalid, return
// invalid.
func FetchEncryptionType(volOptions map[string]string, fallback EncryptionType) EncryptionType {
	encType, ok := volOptions["encryptionType"]
	if !ok {
		return fallback
	}

	if encType == "" {
		return EncryptionTypeInvalid
	}

	return ParseEncryptionType(encType)
}

// NewVolumeEncryption creates a new instance of VolumeEncryption and
// configures the DEKStore. If the KMS does not provide a DEKStore interface,
// the VolumeEncryption will be created *and* a ErrDEKStoreNeeded is returned.
// Callers that receive a ErrDEKStoreNeeded error, should use
// VolumeEncryption.SetDEKStore() to configure an alternative storage for the
// DEKs.
func NewVolumeEncryption(id string, ekms kms.EncryptionKMS) (*VolumeEncryption, error) {
	kmsID := id
	if kmsID == "" {
		// if kmsID is not set, encryption is enabled, and the type is
		// SecretsKMS
		kmsID = kms.DefaultKMSType
	}

	ve := &VolumeEncryption{
		id:  kmsID,
		KMS: ekms,
	}

	if ekms.RequiresDEKStore() == kms.DEKStoreIntegrated {
		dekStore, ok := ekms.(kms.DEKStore)
		if !ok {
			return nil, fmt.Errorf("KMS %T does not implement the "+
				"DEKStore interface", ekms)
		}

		ve.dekStore = dekStore

		return ve, nil
	}

	return ve, ErrDEKStoreNeeded
}

// SetDEKStore sets the DEKStore for this VolumeEncryption instance. It will be
// used when StoreNewCryptoPassphrase() or RemoveDEK() is called.
func (ve *VolumeEncryption) SetDEKStore(dekStore kms.DEKStore) {
	ve.dekStore = dekStore
}

// Destroy frees any resources that the VolumeEncryption instance allocated.
func (ve *VolumeEncryption) Destroy() {
	ve.KMS.Destroy()
}

// RemoveDEK deletes the DEK for a particular volumeID from the DEKStore linked
// with this VolumeEncryption instance.
func (ve *VolumeEncryption) RemoveDEK(volumeID string) error {
	if ve.dekStore == nil {
		return ErrDEKStoreNotFound
	}

	return ve.dekStore.RemoveDEK(volumeID)
}

func (ve *VolumeEncryption) GetID() string {
	return ve.id
}

// StoreCryptoPassphrase takes an unencrypted passphrase, encrypts it and saves
// it in the DEKStore.
func (ve *VolumeEncryption) StoreCryptoPassphrase(volumeID, passphrase string) error {
	encryptedPassphrase, err := ve.KMS.EncryptDEK(volumeID, passphrase)
	if err != nil {
		return fmt.Errorf("failed encrypt the passphrase for %s: %w", volumeID, err)
	}

	err = ve.dekStore.StoreDEK(volumeID, encryptedPassphrase)
	if err != nil {
		return fmt.Errorf("failed to save the passphrase for %s: %w", volumeID, err)
	}

	return nil
}

// StoreNewCryptoPassphrase generates a new passphrase and saves it in the KMS.
func (ve *VolumeEncryption) StoreNewCryptoPassphrase(volumeID string, length int) error {
	passphrase, err := generateNewEncryptionPassphrase(length)
	if err != nil {
		return fmt.Errorf("failed to generate passphrase for %s: %w", volumeID, err)
	}

	return ve.StoreCryptoPassphrase(volumeID, passphrase)
}

// GetCryptoPassphrase Retrieves passphrase to encrypt volume.
func (ve *VolumeEncryption) GetCryptoPassphrase(volumeID string) (string, error) {
	passphrase, err := ve.dekStore.FetchDEK(volumeID)
	if err != nil {
		return "", err
	}

	return ve.KMS.DecryptDEK(volumeID, passphrase)
}

// generateNewEncryptionPassphrase generates a random passphrase for encryption.
func generateNewEncryptionPassphrase(length int) (string, error) {
	bytesPassphrase := make([]byte, length)
	_, err := rand.Read(bytesPassphrase)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(bytesPassphrase), nil
}

// VolumeMapper returns file name and it's path to where encrypted device should be open.
func VolumeMapper(volumeID string) (string, string) {
	mapperFile := mapperFilePrefix + volumeID
	mapperFilePath := path.Join(mapperFilePathPrefix, mapperFile)

	return mapperFile, mapperFilePath
}

// EncryptVolume encrypts provided device with LUKS.
func EncryptVolume(ctx context.Context, devicePath, passphrase string) error {
	log.DebugLog(ctx, "Encrypting device %q	 with LUKS", devicePath)
	_, stdErr, err := LuksFormat(devicePath, passphrase)
	if err != nil || stdErr != "" {
		log.ErrorLog(ctx, "failed to encrypt device %q with LUKS (%v): %s", devicePath, err, stdErr)
	}

	return err
}

// OpenEncryptedVolume opens volume so that it can be used by the client.
func OpenEncryptedVolume(ctx context.Context, devicePath, mapperFile, passphrase string) error {
	log.DebugLog(ctx, "Opening device %q with LUKS on %q", devicePath, mapperFile)
	_, stdErr, err := LuksOpen(devicePath, mapperFile, passphrase)
	if err != nil || stdErr != "" {
		log.ErrorLog(ctx, "failed to open device %q (%v): %s", devicePath, err, stdErr)
	}

	return err
}

// ResizeEncryptedVolume resizes encrypted volume so that it can be used by the client.
func ResizeEncryptedVolume(ctx context.Context, mapperFile string) error {
	log.DebugLog(ctx, "Resizing LUKS device %q", mapperFile)
	_, stdErr, err := LuksResize(mapperFile)
	if err != nil || stdErr != "" {
		log.ErrorLog(ctx, "failed to resize LUKS device %q (%v): %s", mapperFile, err, stdErr)
	}

	return err
}

// CloseEncryptedVolume closes encrypted volume so it can be detached.
func CloseEncryptedVolume(ctx context.Context, mapperFile string) error {
	log.DebugLog(ctx, "Closing LUKS device %q", mapperFile)
	_, stdErr, err := LuksClose(mapperFile)
	if err != nil || stdErr != "" {
		log.ErrorLog(ctx, "failed to close LUKS device %q (%v): %s", mapperFile, err, stdErr)
	}

	return err
}

// IsDeviceOpen determines if encrypted device is already open.
func IsDeviceOpen(ctx context.Context, device string) (bool, error) {
	_, mappedFile, err := DeviceEncryptionStatus(ctx, device)

	return (mappedFile != ""), err
}

// DeviceEncryptionStatus looks to identify if the passed device is a LUKS mapping
// and if so what the device is and the mapper name as used by LUKS.
// If not, just returns the original device and an empty string.
func DeviceEncryptionStatus(ctx context.Context, devicePath string) (string, string, error) {
	if !strings.HasPrefix(devicePath, mapperFilePathPrefix) {
		return devicePath, "", nil
	}
	mapPath := strings.TrimPrefix(devicePath, mapperFilePathPrefix+"/")
	stdout, stdErr, err := LuksStatus(mapPath)
	if err != nil || stdErr != "" {
		log.DebugLog(ctx, "%q is not an active LUKS device (%v): %s", devicePath, err, stdErr)

		return devicePath, "", nil
	}
	lines := strings.Split(stdout, "\n")
	if len(lines) < 1 {
		return "", "", fmt.Errorf("device encryption status returned no stdout for %s", devicePath)
	}
	// The line will look like: "/dev/mapper/xxx is active and is in use."
	if !strings.Contains(lines[0], " is active") {
		// Implies this is not a LUKS device
		return devicePath, "", nil
	}
	for i := 1; i < len(lines); i++ {
		kv := strings.SplitN(strings.TrimSpace(lines[i]), ":", 2)
		if len(kv) < 1 {
			return "", "", fmt.Errorf("device encryption status output for %s is badly formatted: %s",
				devicePath, lines[i])
		}
		if kv[0] == "device" {
			return strings.TrimSpace(kv[1]), mapPath, nil
		}
	}
	// Identified as LUKS, but failed to identify a mapped device
	return "", "", fmt.Errorf("mapped device not found in path %s", devicePath)
}
