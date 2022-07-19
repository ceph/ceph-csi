package cephfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"strconv"
	"time"

	fscryptactions "github.com/google/fscrypt/actions"
	fscryptcrypto "github.com/google/fscrypt/crypto"
	fscryptfilesystem "github.com/google/fscrypt/filesystem"
	fscryptmetadata "github.com/google/fscrypt/metadata"
	"github.com/pkg/xattr"

	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	FscryptHashingTimeTarget = 1 * time.Second
	FscryptProtectorPrefix   = "ceph-csi"
	FscryptSubdir            = "ceph-csi-encrypted"
)

func IsEncrypted(volOptions map[string]string) (bool, error) {
	if val, ok := volOptions["encrypted"]; ok {
		encrypted, err := strconv.ParseBool(val)
		if err != nil {
			return false, err
		}

		return encrypted, nil
	}

	return false, nil
}

// getPassphrase returns the passphrase from the configured Ceph CSI KMS to be used as a protector key in fscrypt.
func getPassphrase(ctx context.Context, encryption util.VolumeEncryption, volID fsutil.VolumeID) (string, error) {
	var (
		passphrase string
		err        error
	)

	switch encryption.KMS.RequiresDEKStore() {
	case kms.DEKStoreIntegrated:
		passphrase, err = encryption.GetCryptoPassphrase(string(volID))
		if err != nil {
			log.ErrorLog(ctx, "fscrypt: failed to get passphrase from KMS: %v", err)

			return "", err
		}
	case kms.DEKStoreMetadata:
		passphrase, err = encryption.KMS.GetSecret(string(volID))
		if err != nil {
			log.ErrorLog(ctx, "fscrypt: failed to GetSecret: %v", err)

			return "", err
		}
	}

	return passphrase, nil
}

// createKeyFuncFromVolumeEncryption returns an fscrypt key function returning
// encryption keys form a VolumeEncryption struct.
func createKeyFuncFromVolumeEncryption(ctx context.Context, encryption util.VolumeEncryption,
	volID fsutil.VolumeID,
) (func(fscryptactions.ProtectorInfo, bool) (*fscryptcrypto.Key, error), error) {
	passphrase, err := getPassphrase(ctx, encryption, volID)
	if err != nil {
		return nil, err
	}

	keyFunc := func(info fscryptactions.ProtectorInfo, retry bool) (*fscryptcrypto.Key, error) {
		key, err := fscryptcrypto.NewBlankKey(32)
		copy(key.Data(), passphrase)

		return key, err
	}

	return keyFunc, nil
}

// unlockExisting tries to unlock an already set up fscrypt directory using keys from Ceph CSI.
func unlockExisting(ctx context.Context, fscryptContext *fscryptactions.Context, encryptedPath string,
	protectorName string, keyFn func(fscryptactions.ProtectorInfo, bool) (*fscryptcrypto.Key, error),
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
			log.ErrorLog(ctx, "fscrypt: failed to lock policy after use: %w", err)
		}
	}()

	if err = policy.Provision(); err != nil {
		log.ErrorLog(ctx, "fscrypt: provision fail %v", err)

		return err
	}

	log.DebugLog(ctx, "fscrypt protector unlock: %s %+v", protectorName, policy)

	return nil
}

func initializeAndUnlock(ctx context.Context, fscryptContext *fscryptactions.Context, encryptedPath string,
	protectorName string, keyFn func(fscryptactions.ProtectorInfo, bool) (*fscryptcrypto.Key, error),
) error {
	var owner *user.User
	var err error

	if err = os.Mkdir(encryptedPath, 0o755); err != nil {
		return err
	}

	protector, err := fscryptactions.CreateProtector(fscryptContext, protectorName, keyFn, owner)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: protector name=%s create failed: %v", protectorName, err)

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

	return nil
}

// IsDirectoryUnlockedFscrypt checks if a directory is an unlocked fscrypted directory.
func (*NodeServer) IsDirectoryUnlockedFscrypt(directoryPath string) error {
	if _, err := fscryptmetadata.GetPolicy(directoryPath); err != nil {
		return fmt.Errorf("no fscrypt policy set on directory %q: %w", directoryPath, err)
	}

	_, err := xattr.Get(directoryPath, "ceph.fscrypt.auth")
	if err != nil {
		return fmt.Errorf("error reading ceph.fscrypt.auth xattr on %q: %w", directoryPath, err)
	}

	return nil
}

// MaybeInitializeFscrypt performs once per nodeserver initialization
// required by the fscrypt library. Creates /etc/fscrypt.conf.
func (*NodeServer) InitializeFscrypt(ctx context.Context, volOptions *store.VolumeOptions,
	stagingTargetPath string, volID fsutil.VolumeID,
) error {
	err := fscryptactions.CreateConfigFile(FscryptHashingTimeTarget, 2)
	if err != nil {
		existsError := &fscryptactions.ErrConfigFileExists{}
		if errors.As(err, &existsError) {
			log.ErrorLog(ctx, "fscrypt: config file %q already exists. Skipping fscrypt node setup",
				existsError.Path)

			return nil
		}

		return err
	}

	return nil
}

// MaybeFscryptUnlock unlocks possilby creating fresh fscrypt metadata
// iff a volume is encrypted. Otherwise return immediately Calling
// this function requires that InitializeFscrypt ran once on this node.
func (*NodeServer) MaybeFscryptUnlock(ctx context.Context, volOptions *store.VolumeOptions,
	stagingTargetPath string, volID fsutil.VolumeID,
) error {
	if !volOptions.IsEncrypted() {
		return nil
	}

	fscryptContext, err := fscryptactions.NewContextFromMountpoint(stagingTargetPath, nil)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: failed to create context from mountpoint %v: %w", stagingTargetPath)

		return err
	}

	log.DebugLog(ctx, "fscrypt context: %+v", fscryptContext)

	if err = fscryptContext.Mount.CheckSupport(); err != nil {
		log.ErrorLog(ctx, "fscrypt: filesystem mount %s does not support fscrypt", fscryptContext.Mount)

		return err
	}

	// A proper set up fscrypy directory requires metadata and a kernel policy:

	// 1. Do we have a metadata directory (.fscrypt) set up?
	metadataDirExists := false
	if err = fscryptContext.Mount.Setup(0o755); err != nil {
		alreadySetupErr := &fscryptfilesystem.ErrAlreadySetup{}
		if errors.As(err, &alreadySetupErr) {
			log.DebugLog(ctx, "fscrypt: metadata directory %q already set up", alreadySetupErr.Mount.Path)
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

	keyFn, err := createKeyFuncFromVolumeEncryption(ctx, *volOptions.Encryption, volID)
	if err != nil {
		log.ErrorLog(ctx, "fscrypt: could not create key function: %v", err)

		return err
	}

	protectorName := fmt.Sprintf("%s-%s", FscryptProtectorPrefix, volOptions.Encryption.GetID())

	switch volOptions.Encryption.KMS.RequiresDEKStore() {
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
		if volOptions.Encryption.KMS.RequiresDEKStore() == kms.DEKStoreIntegrated {
			if err := volOptions.Encryption.StoreNewCryptoPassphrase(string(volID), encryptionPassphraseSize); err != nil {
				log.ErrorLog(ctx, "fscrypt: store new crypto passphrase failed: %v", err)

				return err
			}
		}

		return initializeAndUnlock(ctx, fscryptContext, encryptedPath, protectorName, keyFn)
	}

	return fmt.Errorf("unsupported")
}
