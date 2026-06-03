/*
 * keyring.go - Add/remove encryption policy keys to/from kernel
 *
 * Copyright 2019 Google LLC
 * Author: Eric Biggers (ebiggers@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

// Package keyring manages adding, removing, and getting the status of
// encryption policy keys to/from the kernel.  Most public functions are in
// keyring.go, and they delegate to either user_keyring.go or fs_keyring.go,
// depending on whether a user keyring or a filesystem keyring is being used.
//
// v2 encryption policies always use the filesystem keyring.
// v1 policies use the user keyring by default, but can be configured to use the
// filesystem keyring instead (requires root and kernel v5.4+).
package keyring

import (
	"encoding/hex"
	"os/user"
	"strconv"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/filesystem"
	"github.com/google/fscrypt/metadata"
	"github.com/google/fscrypt/util"
)

// Keyring error values
var (
	ErrKeyAddedByOtherUsers  = errors.New("other users have added the key too")
	ErrKeyFilesOpen          = errors.New("some files using the key are still open")
	ErrKeyNotPresent         = errors.New("key not present or already removed")
	ErrV2PoliciesUnsupported = errors.New("kernel is too old to support v2 encryption policies")
)

// Options are the options which specify *which* keyring the key should be
// added/removed/gotten to, and how.
type Options struct {
	// Mount is the filesystem to which the key should be
	// added/removed/gotten.
	Mount *filesystem.Mount
	// User is the user for whom the key should be added/removed/gotten.
	User *user.User
	// UseFsKeyringForV1Policies is true if keys for v1 encryption policies
	// should be put in the filesystem's keyring (if supported) rather than
	// in the user's keyring.  Note that this makes AddEncryptionKey and
	// RemoveEncryptionKey require root privileges.
	UseFsKeyringForV1Policies bool
}

func shouldUseFsKeyring(descriptor string, options *Options) (bool, error) {
	// For v1 encryption policy keys, use the filesystem keyring if
	// use_fs_keyring_for_v1_policies is set in /etc/fscrypt.conf and the
	// kernel supports it.
	if len(descriptor) == hex.EncodedLen(unix.FSCRYPT_KEY_DESCRIPTOR_SIZE) {
		return options.UseFsKeyringForV1Policies && IsFsKeyringSupported(options.Mount), nil
	}
	// For v2 encryption policy keys, always use the filesystem keyring; the
	// kernel doesn't support any other way.
	if !IsFsKeyringSupported(options.Mount) {
		return true, ErrV2PoliciesUnsupported
	}
	return true, nil
}

// buildKeyDescription builds the description for an fscrypt key of type
// "logon". For ext4 and f2fs, it uses the legacy filesystem-specific prefixes
// for compatibility with kernels before v4.8 and v4.6 respectively. For other
// filesystems it uses the generic prefix "fscrypt".
func buildKeyDescription(options *Options, descriptor string) string {
	switch options.Mount.FilesystemType {
	case "ext4", "f2fs":
		return options.Mount.FilesystemType + ":" + descriptor
	default:
		return unix.FSCRYPT_KEY_DESC_PREFIX + descriptor
	}
}

// AddEncryptionKey adds an encryption policy key to a kernel keyring.  It uses
// either the filesystem keyring for the target Mount or the user keyring for
// the target User.
func AddEncryptionKey(key *crypto.Key, descriptor string, options *Options) error {
	if err := util.CheckValidLength(metadata.PolicyKeyLen, key.Len()); err != nil {
		return errors.Wrap(err, "policy key")
	}
	useFsKeyring, err := shouldUseFsKeyring(descriptor, options)
	if err != nil {
		return err
	}
	if useFsKeyring {
		return fsAddEncryptionKey(key, descriptor, options.Mount, options.User)
	}
	return userAddKey(key, buildKeyDescription(options, descriptor), options.User)
}

// RemoveEncryptionKey removes an encryption policy key from a kernel keyring.
// It uses either the filesystem keyring for the target Mount or the user
// keyring for the target User.
func RemoveEncryptionKey(descriptor string, options *Options, allUsers bool) error {
	useFsKeyring, err := shouldUseFsKeyring(descriptor, options)
	if err != nil {
		return err
	}
	if useFsKeyring {
		user := options.User
		if allUsers {
			user = nil
		}
		return fsRemoveEncryptionKey(descriptor, options.Mount, user)
	}
	return userRemoveKey(buildKeyDescription(options, descriptor), options.User)
}

// KeyStatus is an enum that represents the status of a key in a kernel keyring.
type KeyStatus int

// The possible values of KeyStatus.
const (
	KeyStatusUnknown = 0 + iota
	KeyAbsent
	KeyAbsentButFilesBusy
	KeyPresent
	KeyPresentButOnlyOtherUsers
)

func (status KeyStatus) String() string {
	switch status {
	case KeyStatusUnknown:
		return "Unknown"
	case KeyAbsent:
		return "Absent"
	case KeyAbsentButFilesBusy:
		return "AbsentButFilesBusy"
	case KeyPresent:
		return "Present"
	case KeyPresentButOnlyOtherUsers:
		return "PresentButOnlyOtherUsers"
	default:
		return strconv.Itoa(int(status))
	}
}

// GetEncryptionKeyStatus gets the status of an encryption policy key in a
// kernel keyring.  It uses either the filesystem keyring for the target Mount
// or the user keyring for the target User.
func GetEncryptionKeyStatus(descriptor string, options *Options) (KeyStatus, error) {
	useFsKeyring, err := shouldUseFsKeyring(descriptor, options)
	if err != nil {
		return KeyStatusUnknown, err
	}
	if useFsKeyring {
		return fsGetEncryptionKeyStatus(descriptor, options.Mount, options.User)
	}
	_, _, err = userFindKey(buildKeyDescription(options, descriptor), options.User)
	if err != nil {
		return KeyAbsent, nil
	}
	return KeyPresent, nil
}
