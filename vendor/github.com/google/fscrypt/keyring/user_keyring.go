/*
 * user_keyring.go - Add/remove encryption policy keys to/from user keyrings.
 * This is the deprecated mechanism; see fs_keyring.go for the new mechanism.
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
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

package keyring

import (
	"os/user"
	"runtime"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"fmt"
	"log"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/security"
	"github.com/google/fscrypt/util"
)

// ErrAccessUserKeyring indicates that a user's keyring cannot be
// accessed.
type ErrAccessUserKeyring struct {
	TargetUser      *user.User
	UnderlyingError error
}

func (err *ErrAccessUserKeyring) Error() string {
	return fmt.Sprintf("could not access user keyring for %q: %s",
		err.TargetUser.Username, err.UnderlyingError)
}

// ErrSessionUserKeyring indicates that a user's keyring is not linked
// into the session keyring.
type ErrSessionUserKeyring struct {
	TargetUser *user.User
}

func (err *ErrSessionUserKeyring) Error() string {
	return fmt.Sprintf("user keyring for %q is not linked into the session keyring",
		err.TargetUser.Username)
}

// KeyType is always logon as required by filesystem encryption.
const KeyType = "logon"

// userAddKey puts the provided policy key into the user keyring for the
// specified user with the provided description, and type logon.
func userAddKey(key *crypto.Key, description string, targetUser *user.User) error {
	runtime.LockOSThread() // ensure target user keyring remains possessed in thread keyring
	defer runtime.UnlockOSThread()

	// Create our payload (containing an FscryptKey)
	payload, err := crypto.NewBlankKey(int(unsafe.Sizeof(unix.FscryptKey{})))
	if err != nil {
		return err
	}
	defer payload.Wipe()

	// Cast the payload to an FscryptKey so we can initialize the fields.
	fscryptKey := (*unix.FscryptKey)(payload.UnsafePtr())
	// Mode is ignored by the kernel
	fscryptKey.Mode = 0
	fscryptKey.Size = uint32(key.Len())
	copy(fscryptKey.Raw[:], key.Data())

	keyringID, err := UserKeyringID(targetUser, true)
	if err != nil {
		return err
	}
	keyID, err := unix.AddKey(KeyType, description, payload.Data(), keyringID)
	log.Printf("KeyctlAddKey(%s, %s, <data>, %d) = %d, %v",
		KeyType, description, keyringID, keyID, err)
	if err != nil {
		return errors.Wrapf(err,
			"error adding key with description %s to user keyring for %q",
			description, targetUser.Username)
	}
	return nil
}

// userRemoveKey tries to remove a policy key from the user keyring with the
// provided description. An error is returned if the key does not exist.
func userRemoveKey(description string, targetUser *user.User) error {
	runtime.LockOSThread() // ensure target user keyring remains possessed in thread keyring
	defer runtime.UnlockOSThread()

	keyID, keyringID, err := userFindKey(description, targetUser)
	if err != nil {
		return ErrKeyNotPresent
	}

	_, err = unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, keyringID, 0, 0)
	log.Printf("KeyctlUnlink(%d, %d) = %v", keyID, keyringID, err)
	if err != nil {
		return errors.Wrapf(err,
			"error removing key with description %s from user keyring for %q",
			description, targetUser.Username)
	}
	return nil
}

// userFindKey tries to locate a key with the provided description in the user
// keyring for the target user. The key ID and keyring ID are returned if we can
// find the key. An error is returned if the key does not exist.
func userFindKey(description string, targetUser *user.User) (int, int, error) {
	runtime.LockOSThread() // ensure target user keyring remains possessed in thread keyring
	defer runtime.UnlockOSThread()

	keyringID, err := UserKeyringID(targetUser, false)
	if err != nil {
		return 0, 0, err
	}

	keyID, err := unix.KeyctlSearch(keyringID, KeyType, description, 0)
	log.Printf("KeyctlSearch(%d, %s, %s) = %d, %v", keyringID, KeyType, description, keyID, err)
	if err != nil {
		return 0, 0, errors.Wrapf(err,
			"error searching for key %s in user keyring for %q",
			description, targetUser.Username)
	}
	return keyID, keyringID, err
}

// UserKeyringID returns the key id of the target user's user keyring. We also
// ensure that the keyring will be accessible by linking it into the thread
// keyring and linking it into the root user keyring (permissions allowing). If
// checkSession is true, an error is returned if a normal user requests their
// user keyring, but it is not in the current session keyring.
func UserKeyringID(targetUser *user.User, checkSession bool) (int, error) {
	runtime.LockOSThread() // ensure target user keyring remains possessed in thread keyring
	defer runtime.UnlockOSThread()

	uid := util.AtoiOrPanic(targetUser.Uid)
	targetKeyring, err := userKeyringIDLookup(uid)
	if err != nil {
		return 0, &ErrAccessUserKeyring{targetUser, err}
	}

	if !util.IsUserRoot() {
		// Make sure the returned keyring will be accessible by checking
		// that it is in the session keyring.
		if checkSession && !isUserKeyringInSession(uid) {
			return 0, &ErrSessionUserKeyring{targetUser}
		}
		return targetKeyring, nil
	}

	// Make sure the returned keyring will be accessible by linking it into
	// the root user's user keyring (which will not be garbage collected).
	rootKeyring, err := userKeyringIDLookup(0)
	if err != nil {
		return 0, errors.Wrapf(err, "error looking up root's user keyring")
	}

	if rootKeyring != targetKeyring {
		if err = keyringLink(targetKeyring, rootKeyring); err != nil {
			return 0, errors.Wrapf(err,
				"error linking user keyring for %q into root's user keyring",
				targetUser.Username)
		}
	}
	return targetKeyring, nil
}

func userKeyringIDLookup(uid int) (keyringID int, err error) {

	// Our goals here are to:
	//    - Find the user keyring (for the provided uid)
	//    - Link it into the current thread keyring (so we can use it)
	//    - Make no permanent changes to the process privileges
	// Complicating this are the facts that:
	//    - The value of KEY_SPEC_USER_KEYRING is determined by the ruid
	//    - Keyring linking permissions use the euid
	// So we have to change both the ruid and euid to make this work,
	// setting the suid to 0 so that we can later switch back.
	ruid, euid, suid := security.GetUids()
	if ruid != uid || euid != uid {
		if err = security.SetUids(uid, uid, 0); err != nil {
			return
		}
		defer func() {
			resetErr := security.SetUids(ruid, euid, suid)
			if resetErr != nil {
				err = resetErr
			}
		}()
	}

	// We get the value of KEY_SPEC_USER_KEYRING. Note that this will also
	// trigger the creation of the uid keyring if it does not yet exist.
	keyringID, err = unix.KeyctlGetKeyringID(unix.KEY_SPEC_USER_KEYRING, true)
	log.Printf("keyringID(_uid.%d) = %d, %v", uid, keyringID, err)
	if err != nil {
		return 0, err
	}

	// We still want to use this keyring after our privileges are reset. So
	// we link it into the thread keyring, preventing a loss of access.
	//
	// We must be under LockOSThread() for this to work reliably.  Note that
	// we can't just use the process keyring, since it doesn't work reliably
	// in Go programs, due to the Go runtime creating threads before the
	// program starts and has a chance to create the process keyring.
	if err = keyringLink(keyringID, unix.KEY_SPEC_THREAD_KEYRING); err != nil {
		return 0, err
	}

	return keyringID, nil
}

// isUserKeyringInSession tells us if the user's uid keyring is in the current
// session keyring.
func isUserKeyringInSession(uid int) bool {
	// We cannot use unix.KEY_SPEC_SESSION_KEYRING directly as that might
	// create a session keyring if one does not exist.
	sessionKeyring, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_SESSION_KEYRING, false)
	log.Printf("keyringID(session) = %d, %v", sessionKeyring, err)
	if err != nil {
		return false
	}

	description := fmt.Sprintf("_uid.%d", uid)
	id, err := unix.KeyctlSearch(sessionKeyring, "keyring", description, 0)
	log.Printf("KeyctlSearch(%d, keyring, %s) = %d, %v", sessionKeyring, description, id, err)
	return err == nil
}

func keyringLink(keyID int, keyringID int) error {
	_, err := unix.KeyctlInt(unix.KEYCTL_LINK, keyID, keyringID, 0, 0)
	log.Printf("KeyctlLink(%d, %d) = %v", keyID, keyringID, err)
	return err
}
