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
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"

	"k8s.io/klog"
)

const (
	mapperFilePrefix     = "luks-rbd-"
	mapperFilePathPrefix = "/dev/mapper"

	// image metadata key for encryption
	encryptionMetaKey = ".rbd.csi.ceph.com/encrypted"

	// Encryption passphrase location in K8s secrets
	encryptionPassphraseKey = "encryptionPassphrase"
)

// VolumeMapper returns file name and it's path to where encrypted device should be open
func VolumeMapper(volumeID string) (mapperFile, mapperFilePath string) {
	mapperFile = mapperFilePrefix + volumeID
	mapperFilePath = path.Join(mapperFilePathPrefix, mapperFile)
	return mapperFile, mapperFilePath
}

// GetCryptoPassphrase Retrieves passphrase to encrypt volume
func GetCryptoPassphrase(secrets map[string]string) (string, error) {
	val, ok := secrets[encryptionPassphraseKey]
	if !ok {
		return "", errors.New("missing encryption passphrase in secrets")
	}
	return val, nil
}

// EncryptVolume encrypts provided device with LUKS
func EncryptVolume(ctx context.Context, devicePath, passphrase string) error {
	klog.V(4).Infof(Log(ctx, "Encrypting device %s with LUKS"), devicePath)
	if _, _, err := LuksFormat(devicePath, passphrase); err != nil {
		return errors.Wrapf(err, "failed to encrypt device %s with LUKS", devicePath)
	}
	return nil
}

// OpenEncryptedVolume opens volume so that it can be used by the client
func OpenEncryptedVolume(ctx context.Context, devicePath, mapperFile, passphrase string) error {
	klog.V(4).Infof(Log(ctx, "Opening device %s with LUKS on %s"), devicePath, mapperFile)
	_, _, err := LuksOpen(devicePath, mapperFile, passphrase)
	return err
}

// CloseEncryptedVolume closes encrypted volume so it can be detached
func CloseEncryptedVolume(ctx context.Context, mapperFile string) error {
	klog.V(4).Infof(Log(ctx, "Closing LUKS device %s"), mapperFile)
	_, _, err := LuksClose(mapperFile)
	return err
}

// IsDeviceOpen determines if encrypted device is already open
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
		klog.V(4).Infof(Log(ctx, "device %s is not an active LUKS device: %v"), devicePath, err)
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

// CheckRbdImageEncrypted verifies if rbd image was encrypted when created
func CheckRbdImageEncrypted(ctx context.Context, cr *Credentials, monitors, imageSpec string) (string, error) {
	value, err := GetImageMeta(ctx, cr, monitors, imageSpec, encryptionMetaKey)
	if err != nil {
		klog.Errorf(Log(ctx, "checking image %s encrypted state metadata failed: %s"), imageSpec, err)
		return "", err
	}

	encrypted := strings.TrimSpace(value)
	klog.V(4).Infof(Log(ctx, "image %s encrypted state metadata reports %q"), imageSpec, encrypted)
	return encrypted, nil
}

// SaveRbdImageEncryptionStatus sets image metadata for encryption status
func SaveRbdImageEncryptionStatus(ctx context.Context, cr *Credentials, monitors, imageSpec, status string) error {
	err := SetImageMeta(ctx, cr, monitors, imageSpec, encryptionMetaKey, status)
	if err != nil {
		err = fmt.Errorf("failed to save image metadata encryption status for %s: %v", imageSpec, err.Error())
		klog.Errorf(Log(ctx, err.Error()))
		return err
	}
	return nil
}
