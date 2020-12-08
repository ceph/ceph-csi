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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"crypto/rand"
)

const (
	mapperFilePrefix     = "luks-rbd-"
	mapperFilePathPrefix = "/dev/mapper"

	// Encryption passphrase location in K8s secrets
	encryptionPassphraseKey = "encryptionPassphrase"
	kmsTypeKey              = "encryptionKMSType"

	// Default KMS type
	defaultKMSType = "default"

	// kmsConfigPath is the location of the vault config file
	kmsConfigPath = "/etc/ceph-csi-encryption-kms-config/config.json"

	// Passphrase size - 20 bytes is 160 bits to satisfy:
	// https://tools.ietf.org/html/rfc6749#section-10.10
	encryptionPassphraseSize = 20
)

// EncryptionKMS provides external Key Management System for encryption
// passphrases storage.
type EncryptionKMS interface {
	GetPassphrase(key string) (string, error)
	SavePassphrase(key, value string) error
	DeletePassphrase(key string) error
	GetID() string
}

// MissingPassphrase is an error instructing to generate new passphrase.
type MissingPassphrase struct {
	error
}

// SecretsKMS is default KMS implementation that means no KMS is in use.
type SecretsKMS struct {
	passphrase string
}

func initSecretsKMS(secrets map[string]string) (EncryptionKMS, error) {
	passphraseValue, ok := secrets[encryptionPassphraseKey]
	if !ok {
		return nil, errors.New("missing encryption passphrase in secrets")
	}
	return SecretsKMS{passphrase: passphraseValue}, nil
}

// GetPassphrase returns passphrase from Kubernetes secrets.
func (kms SecretsKMS) GetPassphrase(key string) (string, error) {
	return kms.passphrase, nil
}

// SavePassphrase is not implemented.
func (kms SecretsKMS) SavePassphrase(key, value string) error {
	return fmt.Errorf("save new passphrase is not implemented for Kubernetes secrets")
}

// DeletePassphrase is doing nothing as no new passphrases are saved with
// SecretsKMS.
func (kms SecretsKMS) DeletePassphrase(key string) error {
	return nil
}

// GetID is returning ID representing default KMS `default`.
func (kms SecretsKMS) GetID() string {
	return defaultKMSType
}

// GetKMS returns an instance of Key Management System.
func GetKMS(kmsID string, secrets map[string]string) (EncryptionKMS, error) {
	if kmsID == "" || kmsID == defaultKMSType {
		return initSecretsKMS(secrets)
	}

	// #nosec
	content, err := ioutil.ReadFile(kmsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read kms configuration from %s: %s",
			kmsConfigPath, err)
	}

	var config map[string]interface{}
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kms configuration: %s", err)
	}

	kmsConfigData, ok := config[kmsID].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing encryption KMS configuration with %s", kmsID)
	}
	kmsConfig := make(map[string]string)
	for key, value := range kmsConfigData {
		kmsConfig[key], ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("broken KMS config: '%s' for '%s' is not a string",
				value, key)
		}
	}

	kmsType, ok := kmsConfig[kmsTypeKey]
	if !ok {
		return nil, fmt.Errorf("encryption KMS configuration for %s is missing KMS type", kmsID)
	}

	if kmsType == "vault" {
		return InitVaultKMS(kmsID, kmsConfig, secrets)
	}
	return nil, fmt.Errorf("unknown encryption KMS type %s", kmsType)
}

// GetCryptoPassphrase Retrieves passphrase to encrypt volume.
func GetCryptoPassphrase(ctx context.Context, volumeID string, kms EncryptionKMS) (string, error) {
	passphrase, err := kms.GetPassphrase(volumeID)
	if err == nil {
		return passphrase, nil
	}
	if _, ok := err.(MissingPassphrase); ok {
		DebugLog(ctx, "Encryption passphrase is missing for %s. Generating a new one",
			volumeID)
		passphrase, err = generateNewEncryptionPassphrase()
		if err != nil {
			return "", fmt.Errorf("failed to generate passphrase for %s: %w", volumeID, err)
		}
		err = kms.SavePassphrase(volumeID, passphrase)
		if err != nil {
			return "", fmt.Errorf("failed to save the passphrase for %s: %w", volumeID, err)
		}
		return passphrase, nil
	}
	ErrorLog(ctx, "failed to get encryption passphrase for %s: %s", volumeID, err)
	return "", err
}

// generateNewEncryptionPassphrase generates a random passphrase for encryption.
func generateNewEncryptionPassphrase() (string, error) {
	bytesPassphrase := make([]byte, encryptionPassphraseSize)
	_, err := rand.Read(bytesPassphrase)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytesPassphrase), nil
}

// VolumeMapper returns file name and it's path to where encrypted device should be open.
func VolumeMapper(volumeID string) (mapperFile, mapperFilePath string) {
	mapperFile = mapperFilePrefix + volumeID
	mapperFilePath = path.Join(mapperFilePathPrefix, mapperFile)
	return mapperFile, mapperFilePath
}

// EncryptVolume encrypts provided device with LUKS.
func EncryptVolume(ctx context.Context, devicePath, passphrase string) error {
	DebugLog(ctx, "Encrypting device %s with LUKS", devicePath)
	if _, _, err := LuksFormat(devicePath, passphrase); err != nil {
		return fmt.Errorf("failed to encrypt device %s with LUKS: %w", devicePath, err)
	}
	return nil
}

// OpenEncryptedVolume opens volume so that it can be used by the client.
func OpenEncryptedVolume(ctx context.Context, devicePath, mapperFile, passphrase string) error {
	DebugLog(ctx, "Opening device %s with LUKS on %s", devicePath, mapperFile)
	_, _, err := LuksOpen(devicePath, mapperFile, passphrase)
	return err
}

// CloseEncryptedVolume closes encrypted volume so it can be detached.
func CloseEncryptedVolume(ctx context.Context, mapperFile string) error {
	DebugLog(ctx, "Closing LUKS device %s", mapperFile)
	_, _, err := LuksClose(mapperFile)
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
func DeviceEncryptionStatus(ctx context.Context, devicePath string) (mappedDevice, mapper string, err error) {
	if !strings.HasPrefix(devicePath, mapperFilePathPrefix) {
		return devicePath, "", nil
	}
	mapPath := strings.TrimPrefix(devicePath, mapperFilePathPrefix+"/")
	stdout, _, err := LuksStatus(mapPath)
	if err != nil {
		DebugLog(ctx, "device %s is not an active LUKS device: %v", devicePath, err)
		return devicePath, "", nil
	}
	lines := strings.Split(string(stdout), "\n")
	if len(lines) < 1 {
		return "", "", fmt.Errorf("device encryption status returned no stdout for %s", devicePath)
	}
	if !strings.HasSuffix(lines[0], " is active.") {
		// Implies this is not a LUKS device
		return devicePath, "", nil
	}
	for i := 1; i < len(lines); i++ {
		kv := strings.SplitN(strings.TrimSpace(lines[i]), ":", 2)
		if len(kv) < 1 {
			return "", "", fmt.Errorf("device encryption status output for %s is badly formatted: %s",
				devicePath, lines[i])
		}
		if strings.Compare(kv[0], "device") == 0 {
			return strings.TrimSpace(kv[1]), mapPath, nil
		}
	}
	// Identified as LUKS, but failed to identify a mapped device
	return "", "", fmt.Errorf("mapped device not found in path %s", devicePath)
}
