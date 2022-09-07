/*
Copyright 2022 The Ceph-CSI Authors.
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

package fscrypt

/*
#include <linux/fs.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"time"
	"unsafe"

	fscryptactions "github.com/google/fscrypt/actions"
	fscryptcrypto "github.com/google/fscrypt/crypto"
	fscryptfilesystem "github.com/google/fscrypt/filesystem"
	fscryptmetadata "github.com/google/fscrypt/metadata"
	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"

	"github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	FscryptHashingTimeTarget = 1 * time.Second
	FscryptProtectorPrefix   = "ceph-csi"
	FscryptSubdir            = "ceph-csi-encrypted"
	encryptionPassphraseSize = 64
)

var policyV2Support = []util.KernelVersion{
	{
		Version:      5,
		PatchLevel:   4,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	},
}

func AppendEncyptedSubdirectory(dir string) string {
	return path.Join(dir, FscryptSubdir)
}

// getPassphrase returns the passphrase from the configured Ceph CSI KMS to be used as a protector key in fscrypt.
func getPassphrase(ctx context.Context, encryption util.VolumeEncryption, volID string) (string, error) {
	var (
		passphrase string
		err        error
	)

	switch encryption.KMS.RequiresDEKStore() {
	case kms.DEKStoreIntegrated:
		passphrase, err = encryption.GetCryptoPassphrase(volID)
		if err != nil {
			log.ErrorLog(ctx, "fscrypt: failed to get passphrase from KMS: %v", err)

			return "", err
		}
	case kms.DEKStoreMetadata:
		passphrase, err = encryption.KMS.GetSecret(volID)
		if err != nil {
			log.ErrorLog(ctx, "fscrypt: failed to GetSecret: %v", err)

			return "", err
		}
	}

	return passphrase, nil
}

// createKeyFuncFromVolumeEncryption returns an fscrypt key function returning
// encryption keys form a VolumeEncryption struct.
func createKeyFuncFromVolumeEncryption(
	ctx context.Context,
	encryption util.VolumeEncryption,
	volID string,
) (func(fscryptactions.ProtectorInfo, bool) (*fscryptcrypto.Key, error), error) {
	keyFunc := func(info fscryptactions.ProtectorInfo, retry bool) (*fscryptcrypto.Key, error) {
		passphrase, err := getPassphrase(ctx, encryption, volID)
		if err != nil {
			return nil, err
		}

		key, err := fscryptcrypto.NewBlankKey(encryptionPassphraseSize / 2)
		copy(key.Data(), passphrase)

		return key, err
	}

	return keyFunc, nil
}

// fsyncEncryptedDirectory calls sync on dirPath. It is intended to
// work around the fscrypt library not syncing the directory it sets a
// policy on.
// TODO Remove when the fscrypt dependency has https://github.com/google/fscrypt/pull/359
func fsyncEncryptedDirectory(dirPath string) error {
	dir, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	defer dir.Close()

	return dir.Sync()
}

// unlockExisting tries to unlock an already set up fscrypt directory using keys from Ceph CSI.
func unlockExisting(
	ctx context.Context,
	fscryptContext *fscryptactions.Context,
	encryptedPath string, protectorName string,
	keyFn func(fscryptactions.ProtectorInfo, bool) (*fscryptcrypto.Key, error),
) error {
	var err error

	policy, err := fscryptactions.GetPolicyFromPath(fscryptContext, encryptedPath)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: policy get failed %v", err)

		return err
	}

	optionFn := func(policyDescriptor string, options []*fscryptactions.ProtectorOption) (int, error) {
		for idx, option := range options {
			if option.Name() == protectorName {
				return idx, nil
			}
		}

		return 0, &fscryptactions.ErrNotProtected{PolicyDescriptor: policyDescriptor, ProtectorDescriptor: protectorName}
	}

	if err = policy.Unlock(optionFn, keyFn); err != nil {
		log.ErrorLog(ctx, "fscrypt: unlock with protector error: %v", err)

		return err
	}

	defer func() {
		err = policy.Lock()
		if err != nil {
			log.ErrorLog(ctx, "fscrypt: failed to lock policy after use: %v", err)
		}
	}()

	if err = policy.Provision(); err != nil {
		log.ErrorLog(ctx, "fscrypt: provision fail %v", err)

		return err
	}

	log.DebugLog(ctx, "fscrypt protector unlock: %s %+v", protectorName, policy)

	return nil
}

func initializeAndUnlock(
	ctx context.Context,
	fscryptContext *fscryptactions.Context,
	encryptedPath string, protectorName string,
	keyFn func(fscryptactions.ProtectorInfo, bool) (*fscryptcrypto.Key, error),
) error {
	var owner *user.User
	var err error

	if err = os.Mkdir(encryptedPath, 0o755); err != nil {
		return err
	}

	protector, err := fscryptactions.CreateProtector(fscryptContext, protectorName, keyFn, owner)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: protector name=%s create failed: %v. reverting.", protectorName, err)
		if revertErr := protector.Revert(); revertErr != nil {
			return revertErr
		}

		return err
	}

	if err = protector.Unlock(keyFn); err != nil {
		return err
	}
	log.DebugLog(ctx, "fscrypt protector unlock: %+v", protector)

	var policy *fscryptactions.Policy
	if policy, err = fscryptactions.CreatePolicy(fscryptContext, protector); err != nil {
		return err
	}
	defer func() {
		err = policy.Lock()
		if err != nil {
			log.ErrorLog(ctx, "fscrypt: failed to lock policy after init: %w")
			err = policy.Revert()
			if err != nil {
				log.ErrorLog(ctx, "fscrypt: failed to revert policy after failed lock: %w")
			}
		}
	}()

	if err = policy.UnlockWithProtector(protector); err != nil {
		log.ErrorLog(ctx, "fscrypt: Failed to unlock policy: %v", err)

		return err
	}

	if err = policy.Provision(); err != nil {
		log.ErrorLog(ctx, "fscrypt: Failed to provision policy: %v", err)

		return err
	}

	if err = policy.Apply(encryptedPath); err != nil {
		log.ErrorLog(ctx, "fscrypt: Failed to apply protector (see also kernel log): %w", err)
		if err = policy.Deprovision(false); err != nil {
			log.ErrorLog(ctx, "fscrypt: Policy cleanup response to failing apply failed: %w", err)
		}

		return err
	}

	if err = fsyncEncryptedDirectory(encryptedPath); err != nil {
		log.ErrorLog(ctx, "fscrypt: fsync encrypted dir - to flush kernel policy to disk failed %v", err)

		return err
	}

	return nil
}

// getInodeEncryptedAttribute returns the inode's encrypt attribute similar to lsattr(1)
func getInodeEncryptedAttribute(p string) (bool, error) {
	file, err := os.Open(p)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var attr int
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), unix.FS_IOC_GETFLAGS,
		uintptr(unsafe.Pointer(&attr)))
	if errno != 0 {
		return false, fmt.Errorf("error calling ioctl_iflags: %w", errno)
	}

	if attr&C.FS_ENCRYPT_FL != 0 {
		return true, nil
	}

	return false, nil
}

// IsDirectoryUnlockedFscrypt checks if a directory is an unlocked fscrypted directory.
func IsDirectoryUnlocked(directoryPath, filesystem string) error {
	if _, err := fscryptmetadata.GetPolicy(directoryPath); err != nil {
		return fmt.Errorf("no fscrypt policy set on directory %q: %w", directoryPath, err)
	}

	switch filesystem {
	case "ceph":
		_, err := xattr.Get(directoryPath, "ceph.fscrypt.auth")
		if err != nil {
			return fmt.Errorf("error reading ceph.fscrypt.auth xattr on %q: %w", directoryPath, err)
		}
	default:
		encrypted, err := getInodeEncryptedAttribute(directoryPath)
		if err != nil {
			return err
		}

		if !encrypted {
			return fmt.Errorf("path %s does not have the encrypted inode flag set. Encryption init must have failed",
				directoryPath)
		}
	}

	return nil
}

func getBestPolicyVersion() (int64, error) {
	// fetch the current running kernel info
	release, err := util.GetKernelVersion()
	if err != nil {
		return 0, fmt.Errorf("fetching current kernel version failed: %w", err)
	}

	switch {
	case util.CheckKernelSupport(release, policyV2Support):
		return 2, nil
	default:
		return 1, nil
	}
}

// InitializeNode performs once per nodeserver initialization
// required by the fscrypt library. Creates /etc/fscrypt.conf.
func InitializeNode(ctx context.Context) error {
	policyVersion, err := getBestPolicyVersion()
	if err != nil {
		return fmt.Errorf("fscrypt node init failed to determine best policy version: %w", err)
	}

	err = fscryptactions.CreateConfigFile(FscryptHashingTimeTarget, policyVersion)
	if err != nil {
		existsError := &fscryptactions.ErrConfigFileExists{}
		if errors.As(err, &existsError) {
			log.ErrorLog(ctx, "fscrypt: config file %q already exists. Skipping fscrypt node setup",
				existsError.Path)

			return nil
		}

		return fmt.Errorf("fscrypt node init failed to create node configuration (/etc/fscrypt.conf): %w",
			err)
	}

	return nil
}

// FscryptUnlock unlocks possilby creating fresh fscrypt metadata
// iff a volume is encrypted. Otherwise return immediately Calling
// this function requires that InitializeFscrypt ran once on this node.
func Unlock(
	ctx context.Context,
	volEncryption *util.VolumeEncryption,
	stagingTargetPath string, volID string,
) error {
	// Fetches keys from KMS. Do this first to catch KMS errors before setting up anything.
	keyFn, err := createKeyFuncFromVolumeEncryption(ctx, *volEncryption, volID)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: could not create key function: %v", err)

		return err
	}

	err = fscryptfilesystem.UpdateMountInfo()
	if err != nil {
		return err
	}

	fscryptContext, err := fscryptactions.NewContextFromMountpoint(stagingTargetPath, nil)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: failed to create context from mountpoint %v: %w", stagingTargetPath, err)

		return err
	}

	fscryptContext.Config.UseFsKeyringForV1Policies = true

	log.DebugLog(ctx, "fscrypt context: %+v", fscryptContext)

	if err = fscryptContext.Mount.CheckSupport(); err != nil {
		log.ErrorLog(ctx, "fscrypt: filesystem mount %s does not support fscrypt", fscryptContext.Mount)

		return err
	}

	// A proper set up fscrypy directory requires metadata and a kernel policy:

	// 1. Do we have a metadata directory (.fscrypt) set up?
	metadataDirExists := false
	if err = fscryptContext.Mount.Setup(fscryptfilesystem.SingleUserWritable); err != nil {
		alreadySetupErr := &fscryptfilesystem.ErrAlreadySetup{}
		if errors.As(err, &alreadySetupErr) {
			log.DebugLog(ctx, "fscrypt: metadata directory in %q already set up", alreadySetupErr.Mount.Path)
			metadataDirExists = true
		} else {
			log.ErrorLog(ctx, "fscrypt: mount setup failed: %v", err)

			return err
		}
	}

	encryptedPath := path.Join(stagingTargetPath, FscryptSubdir)
	kernelPolicyExists := false
	// 2. Ask the kernel if the directory has an fscrypt policy in place.
	if _, err = fscryptmetadata.GetPolicy(encryptedPath); err == nil { // encrypted directory already set up
		kernelPolicyExists = true
	}

	if metadataDirExists != kernelPolicyExists {
		return fmt.Errorf("fscrypt: unsupported state metadata=%t kernel_policy=%t",
			metadataDirExists, kernelPolicyExists)
	}

	protectorName := FscryptProtectorPrefix

	switch volEncryption.KMS.RequiresDEKStore() {
	case kms.DEKStoreMetadata:
		// Metadata style KMS use the KMS secret as a custom
		// passphrase directly in fscrypt, circumenting key
		// derivation on the CSI side to allow users to fall
		// back on the fscrypt commandline tool easily
		fscryptContext.Config.Source = fscryptmetadata.SourceType_custom_passphrase
	case kms.DEKStoreIntegrated:
		fscryptContext.Config.Source = fscryptmetadata.SourceType_raw_key
	}

	if kernelPolicyExists && metadataDirExists {
		log.DebugLog(ctx, "fscrypt: Encrypted directory already set up, policy exists")

		return unlockExisting(ctx, fscryptContext, encryptedPath, protectorName, keyFn)
	}

	if !kernelPolicyExists && !metadataDirExists {
		log.DebugLog(ctx, "fscrypt: Creating new protector and policy")
		if volEncryption.KMS.RequiresDEKStore() == kms.DEKStoreIntegrated {
			if err := volEncryption.StoreNewCryptoPassphrase(volID, encryptionPassphraseSize); err != nil {
				log.ErrorLog(ctx, "fscrypt: store new crypto passphrase failed: %v", err)

				return err
			}
		}

		return initializeAndUnlock(ctx, fscryptContext, encryptedPath, protectorName, keyFn)
	}

	return fmt.Errorf("unsupported")
}
