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
	"strconv"
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

	// metadataDEK is the key in the image metadata where the (encrypted)
	// DEK is stored.
	metadataDEK = ".rbd.csi.ceph.com/dek"
)

// checkRbdImageEncrypted verifies if rbd image was encrypted when created.
func (ri *rbdImage) checkRbdImageEncrypted(ctx context.Context) (rbdEncryptionState, error) {
	value, err := ri.GetMetadata(encryptionMetaKey)
	if errors.Is(err, librbd.ErrNotFound) {
		util.DebugLog(ctx, "image %s encrypted state not set", ri.String())
		return rbdImageEncryptionUnknown, nil
	} else if err != nil {
		util.ErrorLog(ctx, "checking image %s encrypted state metadata failed: %s", ri.String(), err)
		return rbdImageEncryptionUnknown, err
	}

	encrypted := rbdEncryptionState(strings.TrimSpace(value))
	util.DebugLog(ctx, "image %s encrypted state metadata reports %q", ri.String(), encrypted)
	return encrypted, nil
}

func (ri *rbdImage) ensureEncryptionMetadataSet(status rbdEncryptionState) error {
	err := ri.SetMetadata(encryptionMetaKey, string(status))
	if err != nil {
		return fmt.Errorf("failed to save encryption status for %s: %w", ri, err)
	}

	return nil
}

// isEncrypted returns `true` if the rbdImage is (or needs to be) encrypted.
func (ri *rbdImage) isEncrypted() bool {
	return ri.encryption != nil
}

// setupEncryption configures the metadata of the RBD image for encryption:
// - the Data-Encryption-Key (DEK) will be generated stored for use by the KMS;
// - the RBD image will be marked to support encryption in its metadata.
func (ri *rbdImage) setupEncryption(ctx context.Context) error {
	err := ri.encryption.StoreNewCryptoPassphrase(ri.VolID)
	if err != nil {
		util.ErrorLog(ctx, "failed to save encryption passphrase for "+
			"image %s: %s", ri.String(), err)
		return err
	}

	err = ri.ensureEncryptionMetadataSet(rbdImageEncryptionPrepared)
	if err != nil {
		util.ErrorLog(ctx, "failed to save encryption status, deleting "+
			"image %s: %s", ri.String(), err)
		return err
	}

	return nil
}

func (ri *rbdImage) encryptDevice(ctx context.Context, devicePath string) error {
	passphrase, err := ri.encryption.GetCryptoPassphrase(ri.VolID)
	if err != nil {
		util.ErrorLog(ctx, "failed to get crypto passphrase for %s: %v",
			ri.String(), err)
		return err
	}

	if err = util.EncryptVolume(ctx, devicePath, passphrase); err != nil {
		err = fmt.Errorf("failed to encrypt volume %s: %w", ri.String(), err)
		util.ErrorLog(ctx, err.Error())
		return err
	}

	err = ri.ensureEncryptionMetadataSet(rbdImageEncrypted)
	if err != nil {
		util.ErrorLog(ctx, err.Error())
		return err
	}

	return nil
}

func (rv *rbdVolume) openEncryptedDevice(ctx context.Context, devicePath string) (string, error) {
	passphrase, err := rv.encryption.GetCryptoPassphrase(rv.VolID)
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

func (ri *rbdImage) initKMS(ctx context.Context, volOptions, credentials map[string]string) error {
	var (
		err       error
		ok        bool
		encrypted string
	)

	// if the KMS is of type VaultToken, additional metadata is needed
	// depending on the tenant, the KMS can be configured with other
	// options
	// FIXME: this works only on Kubernetes, how do other CO supply metadata?
	ri.Owner, ok = volOptions["csi.storage.k8s.io/pvc/namespace"]
	if !ok {
		util.DebugLog(ctx, "could not detect owner for %s", ri.String())
	}

	encrypted, ok = volOptions["encrypted"]
	if !ok {
		return nil
	}

	isEncrypted, err := strconv.ParseBool(encrypted)
	if err != nil {
		return fmt.Errorf(
			"invalid value set in 'encrypted': %s (should be \"true\" or \"false\")", encrypted)
	} else if !isEncrypted {
		return nil
	}

	err = ri.configureEncryption(volOptions["encryptionKMSID"], credentials)
	if err != nil {
		return fmt.Errorf("invalid encryption kms configuration: %w", err)
	}

	return nil
}

// configureEncryption sets up the VolumeEncryption for this rbdImage. Once
// configured, use isEncrypted() to see if the volume supports encryption.
func (ri *rbdImage) configureEncryption(kmsID string, credentials map[string]string) error {
	kms, err := util.GetKMS(ri.Owner, kmsID, credentials)
	if err != nil {
		return err
	}

	ri.encryption, err = util.NewVolumeEncryption(kmsID, kms)

	// if the KMS can not store the DEK itself, we'll store it in the
	// metadata of the RBD image itself
	if errors.Is(err, util.ErrDEKStoreNeeded) {
		ri.encryption.SetDEKStore(ri)
	}

	return nil
}

// StoreDEK saves the DEK in the metadata, overwrites any existing contents.
func (ri *rbdImage) StoreDEK(volumeID, dek string) error {
	if ri.VolID != volumeID {
		return fmt.Errorf("volume %q can not store DEK for %q", ri.String(), volumeID)
	}

	return ri.SetMetadata(metadataDEK, dek)
}

// FetchDEK reads the DEK from the image metadata.
func (ri *rbdImage) FetchDEK(volumeID string) (string, error) {
	if ri.VolID != volumeID {
		return "", fmt.Errorf("volume %q can not fetch DEK for %q", ri.String(), volumeID)
	}

	return ri.GetMetadata(metadataDEK)
}

// RemoveDEK does not need to remove the DEK from the metadata, the image is
// most likely getting removed.
func (ri *rbdImage) RemoveDEK(volumeID string) error {
	if ri.VolID != volumeID {
		return fmt.Errorf("volume %q can not remove DEK for %q", ri.String(), volumeID)
	}

	return nil
}
