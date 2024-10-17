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
	"time"

	kmsapi "github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/cryptsetup"
	"github.com/ceph/ceph-csi/internal/util/lock"
	"github.com/ceph/ceph-csi/internal/util/log"

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

	// image metadata key for encryption.
	encryptionMetaKey    = "rbd.csi.ceph.com/encrypted"
	oldEncryptionMetaKey = ".rbd.csi.ceph.com/encrypted"

	// metadataDEK is the key in the image metadata where the (encrypted)
	// DEK is stored.
	metadataDEK    = "rbd.csi.ceph.com/dek"
	oldMetadataDEK = ".rbd.csi.ceph.com/dek"

	encryptionPassphraseSize = 20

	// rbdDefaultEncryptionType is the default to use when the
	// user did not specify an "encryptionType", but set
	// "encryption": true.
	rbdDefaultEncryptionType = util.EncryptionTypeBlock

	// Luks slots.
	luksSlot0 = "0"
	luksSlot1 = "1"
)

// checkRbdImageEncrypted verifies if rbd image was encrypted when created.
func (ri *rbdImage) checkRbdImageEncrypted(ctx context.Context) (rbdEncryptionState, error) {
	value, err := ri.MigrateMetadata(oldEncryptionMetaKey, encryptionMetaKey, string(rbdImageEncryptionUnknown))
	if errors.Is(err, librbd.ErrNotFound) {
		log.DebugLog(ctx, "image %s encrypted state not set", ri)

		return rbdImageEncryptionUnknown, nil
	} else if err != nil {
		log.ErrorLog(ctx, "checking image %s encrypted state metadata failed: %s", ri, err)

		return rbdImageEncryptionUnknown, err
	}

	encrypted := rbdEncryptionState(strings.TrimSpace(value))
	log.DebugLog(ctx, "image %s encrypted state metadata reports %q", ri, encrypted)

	return encrypted, nil
}

func (ri *rbdImage) ensureEncryptionMetadataSet(status rbdEncryptionState) error {
	err := ri.SetMetadata(encryptionMetaKey, string(status))
	if err != nil {
		return fmt.Errorf("failed to save encryption status for %s: %w", ri, err)
	}

	return nil
}

// isBlockEncrypted returns `true` if the rbdImage is (or needs to be) encrypted.
func (ri *rbdImage) isBlockEncrypted() bool {
	return ri.blockEncryption != nil
}

// isFileEncrypted returns `true` if the filesystem on the rbdImage is (or needs to be) encrypted.
func (ri *rbdImage) isFileEncrypted() bool {
	return ri.fileEncryption != nil
}

func IsFileEncrypted(ctx context.Context, volOptions map[string]string) (bool, error) {
	_, encType, err := ParseEncryptionOpts(volOptions, util.EncryptionTypeInvalid)
	if err != nil {
		return false, err
	}

	return encType == util.EncryptionTypeFile, nil
}

// setupBlockEncryption configures the metadata of the RBD image for encryption:
// - the Data-Encryption-Key (DEK) will be generated stored for use by the KMS;
// - the RBD image will be marked to support encryption in its metadata.
func (ri *rbdImage) setupBlockEncryption(ctx context.Context) error {
	err := ri.blockEncryption.StoreNewCryptoPassphrase(ctx, ri.VolID, encryptionPassphraseSize)
	if err != nil {
		log.ErrorLog(ctx, "failed to save encryption passphrase for "+
			"image %s: %s", ri, err)

		return err
	}

	err = ri.ensureEncryptionMetadataSet(rbdImageEncryptionPrepared)
	if err != nil {
		log.ErrorLog(ctx, "failed to save encryption status, deleting "+
			"image %s: %s", ri, err)

		return err
	}

	return nil
}

// copyEncryptionConfig copies the VolumeEncryption object from the source
// rbdImage to the passed argument if the source rbdImage is encrypted.
// This function re-encrypts the passphrase  from the original, so that
// both encrypted passphrases (potentially, depends on the DEKStore) have
// different contents.
// When copyOnlyPassphrase is set to true, only the passphrase is copied to the
// destination rbdImage's VolumeEncryption object which needs to be initialized
// beforehand and is possibly different from the source VolumeEncryption
// (Usecase: Restoring snapshot into a storageclass with different encryption config).
func (ri *rbdImage) copyEncryptionConfig(ctx context.Context, cp *rbdImage, copyOnlyPassphrase bool) error {
	// nothing to do if parent image is not encrypted.
	if !ri.isBlockEncrypted() && !ri.isFileEncrypted() {
		return nil
	}

	if ri.VolID == cp.VolID {
		return fmt.Errorf("BUG: %q and %q have the same VolID (%s) "+
			"set!? Call stack: %s", ri, cp, ri.VolID, util.CallStack())
	}

	if ri.isBlockEncrypted() {
		// get the unencrypted passphrase
		passphrase, err := ri.blockEncryption.GetCryptoPassphrase(ctx, ri.VolID)
		if err != nil {
			return fmt.Errorf("failed to fetch passphrase for %q: %w",
				ri, err)
		}

		if !copyOnlyPassphrase {
			cp.blockEncryption, err = util.NewVolumeEncryption(ri.blockEncryption.GetID(), ri.blockEncryption.KMS)
			if errors.Is(err, util.ErrDEKStoreNeeded) {
				cp.blockEncryption.SetDEKStore(cp)
			}
		}

		// re-encrypt the plain passphrase for the cloned volume
		err = cp.blockEncryption.StoreCryptoPassphrase(ctx, cp.VolID, passphrase)
		if err != nil {
			return fmt.Errorf("failed to store passphrase for %q: %w",
				cp, err)
		}
	}

	if ri.isFileEncrypted() && !copyOnlyPassphrase {
		var err error
		cp.fileEncryption, err = util.NewVolumeEncryption(ri.fileEncryption.GetID(), ri.fileEncryption.KMS)
		if errors.Is(err, util.ErrDEKStoreNeeded) {
			_, err := ri.fileEncryption.KMS.GetSecret(ctx, "")
			if errors.Is(err, kmsapi.ErrGetSecretUnsupported) {
				return err
			}
		}
	}

	if ri.isFileEncrypted() && ri.fileEncryption.KMS.RequiresDEKStore() == kmsapi.DEKStoreIntegrated {
		// get the unencrypted passphrase
		passphrase, err := ri.fileEncryption.GetCryptoPassphrase(ctx, ri.VolID)
		if err != nil {
			return fmt.Errorf("failed to fetch passphrase for %q: %w",
				ri, err)
		}

		// re-encrypt the plain passphrase for the cloned volume
		err = cp.fileEncryption.StoreCryptoPassphrase(ctx, cp.VolID, passphrase)
		if err != nil {
			return fmt.Errorf("failed to store passphrase for %q: %w",
				cp, err)
		}
	}

	// copy encryption status for the original volume
	status, err := ri.checkRbdImageEncrypted(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to get encryption status for %q: %w",
			ri, err)
	}

	err = cp.ensureEncryptionMetadataSet(status)
	if err != nil {
		return fmt.Errorf("failed to store encryption status for %q: "+
			"%w", cp, err)
	}

	return nil
}

// repairEncryptionConfig checks the encryption state of the current rbdImage,
// and makes sure that the destination rbdImage has the same configuration.
func (ri *rbdImage) repairEncryptionConfig(ctx context.Context, dest *rbdImage) error {
	if !ri.isBlockEncrypted() && !ri.isFileEncrypted() {
		return nil
	}

	// if ri is encrypted, copy its configuration in case it is missing
	if !dest.isBlockEncrypted() && !dest.isFileEncrypted() {
		// dest needs to be connected to the cluster, otherwise it will
		// not be possible to write any metadata
		if dest.conn == nil {
			dest.conn = ri.conn.Copy()
		}

		return ri.copyEncryptionConfig(ctx, dest, true)
	}

	return nil
}

func (ri *rbdImage) encryptDevice(ctx context.Context, devicePath string) error {
	passphrase, err := ri.blockEncryption.GetCryptoPassphrase(ctx, ri.VolID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get crypto passphrase for %s: %v",
			ri, err)

		return err
	}

	if err = util.EncryptVolume(ctx, devicePath, passphrase); err != nil {
		err = fmt.Errorf("failed to encrypt volume %s: %w", ri, err)
		log.ErrorLog(ctx, err.Error())

		return err
	}

	err = ri.ensureEncryptionMetadataSet(rbdImageEncrypted)
	if err != nil {
		log.ErrorLog(ctx, err.Error())

		return err
	}

	return nil
}

func (rv *rbdVolume) openEncryptedDevice(ctx context.Context, devicePath string) (string, error) {
	passphrase, err := rv.blockEncryption.GetCryptoPassphrase(ctx, rv.VolID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get passphrase for encrypted device %s: %v",
			rv, err)

		return "", err
	}

	mapperFile, mapperFilePath := util.VolumeMapper(rv.VolID)

	isOpen, err := util.IsDeviceOpen(ctx, mapperFilePath)
	if err != nil {
		log.ErrorLog(ctx, "failed to check device %s encryption status: %s", devicePath, err)

		return devicePath, err
	}
	if isOpen {
		log.DebugLog(ctx, "encrypted device is already open at %s", mapperFilePath)
	} else {
		err = util.OpenEncryptedVolume(ctx, devicePath, mapperFile, passphrase)
		if err != nil {
			log.ErrorLog(ctx, "failed to open device %s: %v",
				rv, err)

			return devicePath, err
		}
	}

	return mapperFilePath, nil
}

func (ri *rbdImage) initKMS(ctx context.Context, volOptions, credentials map[string]string) error {
	kmsID, encType, err := ParseEncryptionOpts(volOptions, rbdDefaultEncryptionType)
	if err != nil {
		return err
	}

	switch encType {
	case util.EncryptionTypeBlock:
		err = ri.configureBlockEncryption(kmsID, credentials)
	case util.EncryptionTypeFile:
		err = ri.configureFileEncryption(ctx, kmsID, credentials)
	case util.EncryptionTypeInvalid:
		return errors.New("invalid encryption type")
	case util.EncryptionTypeNone:
		return nil
	}

	if err != nil {
		return fmt.Errorf("invalid encryption kms configuration: %w", err)
	}

	return nil
}

// ParseEncryptionOpts returns kmsID and sets Owner attribute.
func ParseEncryptionOpts(
	volOptions map[string]string,
	fallbackEncType util.EncryptionType,
) (string, util.EncryptionType, error) {
	var (
		err              error
		ok               bool
		encrypted, kmsID string
	)
	encrypted, ok = volOptions["encrypted"]
	if !ok {
		return "", util.EncryptionTypeNone, nil
	}
	ok, err = strconv.ParseBool(encrypted)
	if err != nil {
		return "", util.EncryptionTypeInvalid, err
	}
	if !ok {
		return "", util.EncryptionTypeNone, nil
	}
	kmsID, err = util.FetchEncryptionKMSID(encrypted, volOptions["encryptionKMSID"])
	if err != nil {
		return "", util.EncryptionTypeInvalid, err
	}

	encType := util.FetchEncryptionType(volOptions, fallbackEncType)

	return kmsID, encType, nil
}

// configureBlockDeviceEncryption sets up the VolumeEncryption for this rbdImage. Once
// configured, use isBlockEncrypted() to see if the volume supports block encryption.
func (ri *rbdImage) configureBlockEncryption(kmsID string, credentials map[string]string) error {
	kms, err := kmsapi.GetKMS(ri.Owner, kmsID, credentials)
	if err != nil {
		return err
	}

	ri.blockEncryption, err = util.NewVolumeEncryption(kmsID, kms)

	// if the KMS can not store the DEK itself, we'll store it in the
	// metadata of the RBD image itself
	if errors.Is(err, util.ErrDEKStoreNeeded) {
		ri.blockEncryption.SetDEKStore(ri)
	}

	return nil
}

// configureBlockDeviceEncryption sets up the VolumeEncryption for this rbdImage. Once
// configured, use isEncrypted() to see if the volume supports encryption.
func (ri *rbdImage) configureFileEncryption(ctx context.Context, kmsID string, credentials map[string]string) error {
	kms, err := kmsapi.GetKMS(ri.Owner, kmsID, credentials)
	if err != nil {
		return err
	}

	ri.fileEncryption, err = util.NewVolumeEncryption(kmsID, kms)

	if errors.Is(err, util.ErrDEKStoreNeeded) {
		// fscrypt uses secrets directly from the KMS.
		// Therefore we do not support an additional DEK
		// store. Since not all "metadata" KMS support
		// GetSecret, test for support here. Postpone any
		// other error handling
		_, err := ri.fileEncryption.KMS.GetSecret(ctx, "")
		if errors.Is(err, kmsapi.ErrGetSecretUnsupported) {
			return err
		}
	}

	return nil
}

// StoreDEK saves the DEK in the metadata, overwrites any existing contents.
func (ri *rbdImage) StoreDEK(ctx context.Context, volumeID, dek string) error {
	if ri.VolID == "" {
		return fmt.Errorf("BUG: %q does not have VolID set, call "+
			"stack: %s", ri, util.CallStack())
	} else if ri.VolID != volumeID {
		return fmt.Errorf("volume %q can not store DEK for %q",
			ri, volumeID)
	}

	return ri.SetMetadata(metadataDEK, dek)
}

// FetchDEK reads the DEK from the image metadata.
func (ri *rbdImage) FetchDEK(ctx context.Context, volumeID string) (string, error) {
	if ri.VolID == "" {
		return "", fmt.Errorf("BUG: %q does not have VolID set, call "+
			"stack: %s", ri, util.CallStack())
	} else if ri.VolID != volumeID {
		return "", fmt.Errorf("volume %q can not fetch DEK for %q", ri, volumeID)
	}

	return ri.MigrateMetadata(oldMetadataDEK, metadataDEK, "")
}

// RemoveDEK does not need to remove the DEK from the metadata, the image is
// most likely getting removed.
func (ri *rbdImage) RemoveDEK(ctx context.Context, volumeID string) error {
	if ri.VolID == "" {
		return fmt.Errorf("BUG: %q does not have VolID set, call "+
			"stack: %s", ri, util.CallStack())
	} else if ri.VolID != volumeID {
		return fmt.Errorf("volume %q can not remove DEK for %q",
			ri, volumeID)
	}

	return nil
}

// GetEncryptionPassphraseSize returns the value of `encryptionPassphraseSize`.
func GetEncryptionPassphraseSize() int {
	return encryptionPassphraseSize
}

// RotateEncryptionKey processes the key rotation for the RBD Volume.
func (rv *rbdVolume) RotateEncryptionKey(ctx context.Context) error {
	if !rv.isBlockEncrypted() {
		return errors.New("key rotation unsupported for non block encrypted device")
	}

	// Verify that the underlying device has been setup for encryption
	currState, err := rv.checkRbdImageEncrypted(ctx)
	if err != nil {
		return fmt.Errorf("failed to check encryption state: %w", err)
	}

	if currState != rbdImageEncrypted {
		return errors.New("key rotation not supported for unencrypted device")
	}

	// Call open Ioctx to create a new ioctx object
	// if the obj already exists, no error is returned
	err = rv.openIoctx()
	if err != nil {
		return fmt.Errorf("failed to open ioctx, err: %w", err)
	}

	// Lock params
	lockName := rv.VolID + "-mutexlock"
	lockDesc := "Key rotation mutex lock for " + rv.VolID
	lockCookie := rv.VolID + "-enc-key-rotate"

	// Keep this a little more than ExecutionTimeout to have some buffer
	// for cleanup. If this lock is a part of some gRPC call, the client
	// should always timeout after the lockDuration to avoid issues.
	lockDuration := cryptsetup.ExecutionTimeout + 30*time.Second
	timedCtx, cancel := context.WithTimeout(ctx, cryptsetup.ExecutionTimeout)
	defer cancel()

	// Acquire the exclusive lock based on vol id
	lck := lock.NewLock(rv.ioctx, rv.VolID, lockName, lockCookie, lockDesc, lockDuration)
	err = lck.LockExclusive(ctx)
	if err != nil {
		return err
	}
	defer lck.Unlock(ctx)
	log.DebugLog(ctx, "acquired ioctx lock for vol id: %s", rv.VolID)

	// Get the device path for the underlying image
	useNbd := rv.Mounter == rbdNbdMounter && hasNBD
	devicePath, found := waitForPath(ctx, rv.Pool, rv.RadosNamespace, rv.RbdImageName, 1, useNbd)
	if !found {
		return fmt.Errorf("failed to get the device path for %q: %w", rv, err)
	}

	// Step 1: Get the current passphrase
	oldPassphrase, err := rv.blockEncryption.GetCryptoPassphrase(ctx, rv.VolID)
	if err != nil {
		return fmt.Errorf("failed to fetch the current passphrase for %q: %w", rv, err)
	}

	// Create a new luks wrapper
	luks := cryptsetup.NewLUKSWrapper(timedCtx)

	// Step 2: Add current key to slot 1
	err = luks.AddKey(devicePath, oldPassphrase, oldPassphrase, luksSlot1)
	if err != nil {
		return fmt.Errorf("failed to add curr key to luksSlot1: %w", err)
	}

	// Step 3: Generate new key and add it to slot 0
	newPassphrase, err := rv.blockEncryption.GetNewCryptoPassphrase(
		GetEncryptionPassphraseSize())
	if err != nil {
		return fmt.Errorf("failed to generate a new passphrase: %w", err)
	}

	err = luks.AddKey(devicePath, oldPassphrase, newPassphrase, luksSlot0)
	if err != nil {
		return fmt.Errorf("failed to add the new key to luksSlot0: %w", err)
	}

	// Step 4: Add the new key to KMS
	err = rv.blockEncryption.StoreCryptoPassphrase(timedCtx, rv.VolID, newPassphrase)
	if err != nil {
		return fmt.Errorf("failed to update the new key into the KMS: %w", err)
	}

	// Step 5: Remove the old key from slot 1
	// We use the newPassphrase to authenticate LUKS here
	err = luks.RemoveKey(devicePath, newPassphrase, luksSlot1)
	if err != nil {
		return fmt.Errorf("failed to remove the backup key from luksSlot1: %w", err)
	}

	// Return error accordingly.
	return nil
}
