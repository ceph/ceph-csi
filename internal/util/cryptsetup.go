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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ceph/ceph-csi/internal/util/file"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// Limit memory used by Argon2i PBKDF to 32 MiB.
const cryptsetupPBKDFMemoryLimit = 32 << 10 // 32768 KiB

// LuksFormat sets up volume as an encrypted LUKS partition.
func LuksFormat(devicePath, passphrase string) (string, string, error) {
	return execCryptsetupCommand(
		&passphrase,
		"-q",
		"luksFormat",
		"--type",
		"luks2",
		"--hash",
		"sha256",
		"--pbkdf-memory",
		strconv.Itoa(cryptsetupPBKDFMemoryLimit),
		devicePath,
		"-d",
		"/dev/stdin")
}

// LuksOpen opens LUKS encrypted partition and sets up a mapping.
func LuksOpen(devicePath, mapperFile, passphrase string) (string, string, error) {
	// cryptsetup option --disable-keyring (introduced with cryptsetup v2.0.0)
	// will be ignored with luks1
	return execCryptsetupCommand(&passphrase, "luksOpen", devicePath, mapperFile, "--disable-keyring", "-d", "/dev/stdin")
}

// LuksResize resizes LUKS encrypted partition.
func LuksResize(mapperFile string) (string, string, error) {
	return execCryptsetupCommand(nil, "resize", mapperFile)
}

// LuksClose removes existing mapping.
func LuksClose(mapperFile string) (string, string, error) {
	return execCryptsetupCommand(nil, "luksClose", mapperFile)
}

// LuksStatus returns encryption status of a provided device.
func LuksStatus(mapperFile string) (string, string, error) {
	return execCryptsetupCommand(nil, "status", mapperFile)
}

// LuksAddKey adds a new key to the specified slot.
func LuksAddKey(devicePath, passphrase, newPassphrase, slot string) error {
	passFile, err := file.CreateTempFile("luks-", passphrase)
	if err != nil {
		return err
	}
	defer os.Remove(passFile.Name())

	newPassFile, err := file.CreateTempFile("luks-", newPassphrase)
	if err != nil {
		return err
	}
	defer os.Remove(newPassFile.Name())

	_, stderr, err := execCryptsetupCommand(
		nil,
		"--verbose",
		"--key-file="+passFile.Name(),
		"--key-slot="+slot,
		"luksAddKey",
		devicePath,
		newPassFile.Name(),
	)

	// Return early if no error to save us some time
	if err == nil {
		return nil
	}

	// Possible scenarios
	// 1. The provided passphrase to unlock the disk is wrong
	// 2. The key slot is already in use
	// 	  If so, check if the key we want to add to the slot is already there
	//    If not, remove it and then add the new key to the slot
	if strings.Contains(stderr, fmt.Sprintf("Key slot %s is full", slot)) {
		// The given slot already has a key
		// Check if it is the one that we want to update with
		exists, fErr := LuksVerifyKey(devicePath, newPassphrase, slot)
		if fErr != nil {
			return fErr
		}

		// Verification passed, return early
		if exists {
			return nil
		}

		// Else, we remove the key from the given slot and add the new one
		// Note: we use existing passphrase here as we are not yet sure if
		// the newPassphrase is present in the headers
		fErr = LuksRemoveKey(devicePath, passphrase, slot)
		if fErr != nil {
			return fErr
		}

		// Now the slot is free, add the new key to it
		fErr = LuksAddKey(devicePath, passphrase, newPassphrase, slot)
		if fErr != nil {
			return fErr
		}

		// No errors, we good.
		return nil
	}

	// The existing passphrase is wrong and the slot is empty
	return err
}

// LuksRemoveKey removes the key by killing the specified slot.
func LuksRemoveKey(devicePath, passphrase, slot string) error {
	keyFile, err := file.CreateTempFile("luks-", passphrase)
	if err != nil {
		return err
	}
	defer os.Remove(keyFile.Name())

	_, stderr, err := execCryptsetupCommand(
		nil,
		"--verbose",
		"--key-file="+keyFile.Name(),
		"luksKillSlot",
		devicePath,
		slot,
	)
	if err != nil {
		// If a slot is not active, don't treat that as an error
		if !strings.Contains(stderr, fmt.Sprintf("Keyslot %s is not active.", slot)) {
			return fmt.Errorf("failed to kill slot %s for device %s: %w", slot, devicePath, err)
		}
	}

	return nil
}

// LuksVerifyKey verifies that a key exists in a given slot.
func LuksVerifyKey(devicePath, passphrase, slot string) (bool, error) {
	// Create a temp file that we will use to open the device
	keyFile, err := file.CreateTempFile("luks-", passphrase)
	if err != nil {
		return false, err
	}
	defer os.Remove(keyFile.Name())

	_, stderr, err := execCryptsetupCommand(
		nil,
		"--verbose",
		"--key-file="+keyFile.Name(),
		"--key-slot="+slot,
		"luksChangeKey",
		devicePath,
		keyFile.Name(),
	)
	if err != nil {
		// If the passphrase doesn't match the key in given slot
		if strings.Contains(stderr, "No key available with this passphrase.") {
			// No match, no error
			return false, nil
		}

		// Otherwise it was something else, return the wrapped error
		log.ErrorLogMsg("failed to verify key in slot %s. stderr: %s. err: %v", slot, stderr, err)

		return false, fmt.Errorf("failed to verify key in slot %s for device %s: %w", slot, devicePath, err)
	}

	return true, nil
}

func execCryptsetupCommand(stdin *string, args ...string) (string, string, error) {
	var (
		program       = "cryptsetup"
		cmd           = exec.Command(program, args...) // #nosec:G204, commands executing not vulnerable.
		sanitizedArgs = StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if err != nil {
		return stdout, stderr, fmt.Errorf("an error (%v)"+
			" occurred while running %s args: %v", err, program, sanitizedArgs)
	}

	return stdout, stderr, err
}
