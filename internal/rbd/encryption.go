/*
Copyright 2021 The Ceph-CSI Authors.

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

package rbd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"

	librbd "github.com/ceph/go-ceph/rbd"
)

// rbdEncryptionState describes the status of the process where the image is
// with respect to being encrypted.
type rbdEncryptionState string

const (
	// rbdImageEncryptionUnknown means the image is not encrypted, or the
	// metadata of the image can not be fetched.
	rbdImageEncryptionUnknown = rbdEncryptionState("")
	// rbdImageEncrypted is set in the image metadata after the image has
	// been formatted with cryptsetup. Future usage of the image should
	// unlock the image before mounting.
	rbdImageEncrypted = rbdEncryptionState("encrypted")
	// rbdImageEncryptionPrepared gets set in the image metadata once the
	// passphrase for the image has been generated and stored in the KMS.
	// When using the image for the first time, it needs to be encrypted
	// with cryptsetup before updating the state to `rbdImageEncrypted`.
	rbdImageEncryptionPrepared = rbdEncryptionState("encryptionPrepared")

	// rbdImageRequiresEncryption has been deprecated, it is used only for
	// volumes that have been created with an old provisioner, were never
	// attached/mounted and now get staged by a new node-plugin
	// TODO: remove this backwards compatibility support
	rbdImageRequiresEncryption = rbdEncryptionState("requiresEncryption")

	// image metadata key for encryption
	encryptionMetaKey = ".rbd.csi.ceph.com/encrypted"
)

// checkRbdImageEncrypted verifies if rbd image was encrypted when created.
func (rv *rbdVolume) checkRbdImageEncrypted(ctx context.Context) (rbdEncryptionState, error) {
	value, err := rv.GetMetadata(encryptionMetaKey)
	if errors.Is(err, librbd.ErrNotFound) {
		util.DebugLog(ctx, "image %s encrypted state not set", rv.String())
		return rbdImageEncryptionUnknown, nil
	} else if err != nil {
		util.ErrorLog(ctx, "checking image %s encrypted state metadata failed: %s", rv.String(), err)
		return rbdImageEncryptionUnknown, err
	}

	encrypted := rbdEncryptionState(strings.TrimSpace(value))
	util.DebugLog(ctx, "image %s encrypted state metadata reports %q", rv.String(), encrypted)
	return encrypted, nil
}

func (rv *rbdVolume) ensureEncryptionMetadataSet(status rbdEncryptionState) error {
	err := rv.SetMetadata(encryptionMetaKey, string(status))
	if err != nil {
		return fmt.Errorf("failed to save encryption status for %s: %w", rv, err)
	}

	return nil
}

// setupEncryption configures the metadata of the RBD image for encryption:
// - the Data-Encryption-Key (DEK) will be generated stored for use by the KMS;
// - the RBD image will be marked to support encryption in its metadata.
func (rv *rbdVolume) setupEncryption(ctx context.Context) error {
	err := util.StoreNewCryptoPassphrase(rv.VolID, rv.KMS)
	if err != nil {
		util.ErrorLog(ctx, "failed to save encryption passphrase for "+
			"image %s: %s", rv.String(), err)
		return err
	}

	err = rv.ensureEncryptionMetadataSet(rbdImageEncryptionPrepared)
	if err != nil {
		util.ErrorLog(ctx, "failed to save encryption status, deleting "+
			"image %s: %s", rv.String(), err)
		return err
	}

	return nil
}

func (rv *rbdVolume) openEncryptedDevice(ctx context.Context, devicePath string) (string, error) {
	passphrase, err := util.GetCryptoPassphrase(rv.VolID, rv.KMS)
	if err != nil {
		util.ErrorLog(ctx, "failed to get passphrase for encrypted device %s: %v",
			rv.String(), err)
		return "", err
	}

	mapperFile, mapperFilePath := util.VolumeMapper(rv.VolID)

	isOpen, err := util.IsDeviceOpen(ctx, mapperFilePath)
	if err != nil {
		util.ErrorLog(ctx, "failed to check device %s encryption status: %s", devicePath, err)
		return devicePath, err
	}
	if isOpen {
		util.DebugLog(ctx, "encrypted device is already open at %s", mapperFilePath)
	} else {
		err = util.OpenEncryptedVolume(ctx, devicePath, mapperFile, passphrase)
		if err != nil {
			util.ErrorLog(ctx, "failed to open device %s: %v",
				rv.String(), err)
			return devicePath, err
		}
	}

	return mapperFilePath, nil
}
