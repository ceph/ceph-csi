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

package cryptsetup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"k8s.io/cloud-provider/volume/helpers"

	"github.com/ceph/ceph-csi/internal/util/file"
	"github.com/ceph/ceph-csi/internal/util/log"
	"github.com/ceph/ceph-csi/internal/util/stripsecrets"
)

const (
	// Maximum time to wait for cryptsetup commands to complete.
	ExecutionTimeout = 2*time.Minute + 30*time.Second

	// Limit memory used by Argon2i PBKDF to 32 MiB.
	cryptsetupPBKDFMemoryLimit = 32 << 10 // 32768 KiB
	luks2MetadataSize          = 32 << 7  // 4096 KiB
	luks2KeySlotsSize          = 32 << 8  // 8192 KiB

	// The LUKS2 header size is variable and it can be adjusted
	// on creation by using the `--luks2-metadata-size` and
	// `--luks2-keyslots-size` options.
	Luks2HeaderSize = uint64((((2 * luks2MetadataSize) + luks2KeySlotsSize) * helpers.KiB))

	// Older Images provisioned (with <=3.14 Ceph-CSI) didn't use the
	// `--luks2-metadata-size` and `--luks2-keyslots-size` options
	// during luksFormat, So the header size will be default 16MiB.
	DefaultLuks2HeaderSize = 16 * helpers.MiB

	// LuksStatusCipherIdentifier is the identifier of the cipher section in a luks status.
	LuksStatusCipherIdentifier = "cipher"

	// LuksStatusKeySizeIdentifier is the identifier of the key size section in a luks status.
	LuksStatusKeySizeIdentifier = "keysize"

	// LuksStausInegrityIdentifier is the identifier of the integrity section in a luks status.
	LuksStausInegrityIdentifier = "integrity"

	// LuksStausInegrityKeySize is the identifier of the integrity key size section in a luks status.
	LuksStausInegrityKeySize = "integrity keysize"
)

// LuksStatus represents some of the information stored in a luks status output.
type LuksStatus struct {
	cipher        string
	keysize       uint
	integrityMode *string
	// if this is set we have to subtract it from the
	// keysize to get actual key size used for encryption
	intgrityKeySize *uint
}

type luksKeySizeSetter func(size uint)

func (l *LuksStatus) SetCipher(input string) error {
	l.cipher = input

	return nil
}

func (l *LuksStatus) SetKeySize(input string) error {
	return l.parseLuksStatusKeySize(input, func(size uint) {
		l.keysize = size
	})
}

func (l *LuksStatus) SetIntegrityKeySize(input string) error {
	return l.parseLuksStatusKeySize(input, func(size uint) {
		l.intgrityKeySize = &size
	})
}

// SetIntegrityModeFromLuks translates the raw integrity mode string from a luksDump
// (e.g., "hmac(sha256)") into its equivalent cryptsetup identifier (e.g., "hmac-sha256").
func (l *LuksStatus) SetIntegrityModeFromLuks(input string) error {
	result, err := integrityModeValidator.findCryptsetupMode(input)
	if err != nil {
		return fmt.Errorf("could not parse Integrity mode, %w", err)
	}
	l.integrityMode = &result

	return nil
}

func (l *LuksStatus) parseLuksStatusKeySize(input string, setter luksKeySizeSetter) error {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) < 2 {
		return fmt.Errorf("could not parse %s", input)
	}
	sizeString := parts[0]
	size, err := strconv.Atoi(sizeString)
	if err != nil {
		return fmt.Errorf("could not parse number from %q: %w", sizeString, err)
	}
	setter(uint(size))

	return nil
}

type validator struct {
	allowedValues map[string]string
	name          string
}

func (v *validator) validate(value string) error {
	if _, ok := v.allowedValues[value]; !ok {
		return fmt.Errorf("%s not allowed: %s", v.name, value)
	}

	return nil
}

func (v *validator) findCryptsetupMode(value string) (string, error) {
	for crypsetupMode, luksStatusMode := range v.allowedValues {
		if value == luksStatusMode {
			return crypsetupMode, nil
		}
	}

	return "", fmt.Errorf("could not find luksStatus equivalent for %s", value)
}

// GetAllowedCiphers gives all the ciphers that can be set.
func GetAllowedCiphers() []string {
	c := make([]string, 0, len(cipherValidator.allowedValues))
	for k := range cipherValidator.allowedValues {
		c = append(c, k)
	}

	return c
}

var (
	cipherValidator = &validator{
		name: "cipher",
		allowedValues: map[string]string{
			"aes-xts-plain64":     "",
			"serpent-xts-plain64": "",
			"twofish-xts-plain64": "",
			"aegis128-random":     "",
		},
	}

	integrityModeValidator = &validator{
		name: "integrity mode",
		// Problem: The strings that identify integrity modes are different
		// in a luksStatus and the cryptsetup input.
		// In this map Keys represent the cryptsetup input and values represent the
		// LuksStatus equivalent.
		allowedValues: map[string]string{
			"hmac-sha256": "hmac(sha256)",
			"hmac-sha512": "hmac(sha512)",
			"aead":        "aead",
		},
	}
)

type EncryptionOptions struct {
	cipher        string
	keysize       *uint
	integrityMode *string
}

func (e *EncryptionOptions) IntegrityMode() *string {
	return e.integrityMode
}

func (e *EncryptionOptions) SetIntegrityMode(integrity string) error {
	if err := integrityModeValidator.validate(integrity); err != nil {
		return fmt.Errorf("cannot set integrity mode '%s': %w", integrity, err)
	}
	e.integrityMode = &integrity

	return nil
}

func (e *EncryptionOptions) KeySize() *uint {
	return e.keysize
}

func (e *EncryptionOptions) SetKeySize(keysize string) error {
	parsedVal, err := strconv.ParseUint(keysize, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid keySize value '%s': %w", keysize, err)
	}
	e.keysize = new(uint)
	*e.keysize = uint(parsedVal)

	return nil
}

func (e *EncryptionOptions) Cipher() string {
	return e.cipher
}

func (e *EncryptionOptions) SetCipher(cipher string) error {
	if err := cipherValidator.validate(cipher); err != nil {
		return fmt.Errorf("cannot set cipher'%s': %w", cipher, err)
	}
	e.cipher = cipher

	return nil
}

// Equal compares the desired EncryptionOptions with actual LuksStatus.
// It translates between the logic used by crypsetup and luks status.
//
// Key Translation Logic:
//
//	KeySize: `luksStatus.keysize` reports the total size of key material
//	(e.g., data key + integrity key). `EncryptionOption.keysize` only contains the size of the data key.
//	This function subtracts the `luksStatus.intgrityKeySize`
//	(if present) from the total `luksStatus.keysize` before comparing.
//	To only compare the size of the data key of the EncryptionOption and luksStatus.
func (e *EncryptionOptions) Equal(luksStatus LuksStatus) bool {
	if e.cipher != luksStatus.cipher {
		return false
	}
	if e.keysize != nil {
		var encryptionKeySize uint
		if luksStatus.intgrityKeySize != nil {
			// When integrityKeySize is set it means that a key for encryption and
			// a separate key for integrity protection is set. A luks status sums them
			// up in the 'keysize' field here we subtract them
			// so we can compare the encryption key sizes.
			encryptionKeySize = luksStatus.keysize - *luksStatus.intgrityKeySize
		} else {
			encryptionKeySize = luksStatus.keysize
		}
		if *e.keysize != encryptionKeySize {
			return false
		}
	}
	if e.integrityMode != nil {
		if (luksStatus.integrityMode == nil) || (*e.integrityMode != *luksStatus.integrityMode) {
			return false
		}
	}

	return true
}

// LuksWrapper is a struct that provides a context-aware wrapper around cryptsetup commands.
type LUKSWrapper interface {
	Format(devicePath, passphrase string, cipher *EncryptionOptions) (string, string, error)
	Open(devicePath, mapperFile, passphrase string) (string, string, error)
	Close(mapperFile string) (string, string, error)
	AddKey(devicePath, passphrase, newPassphrase, slot string) error
	RemoveKey(devicePath, passphrase, slot string) error
	Resize(mapperFile string) (string, string, error)
	VerifyKey(devicePath, passphrase, slot string) (bool, error)
	Status(mapperFile string) (string, string, error)
}

// luksWrapper is a type that implements LUKSWrapper interface
// and provides a shared context for its methods.
type luksWrapper struct {
	ctx context.Context
}

// NewLUKSWrapper creates a new LUKSWrapper instance with the provided context.
// The context is used to control the lifetime of the cryptsetup commands.
func NewLUKSWrapper(ctx context.Context) LUKSWrapper {
	return &luksWrapper{ctx: ctx}
}

// LuksFormat sets up volume as an encrypted LUKS partition.
func (l *luksWrapper) Format(devicePath, passphrase string, cipherOptions *EncryptionOptions) (string, string, error) {
	args := []string{
		"-q",
		"luksFormat",
		"--type",
		"luks2",
		"--hash",
		"sha256",
	}
	if cipherOptions != nil {
		args = append(args, "--cipher", cipherOptions.Cipher())
		if cipherOptions.IntegrityMode() != nil {
			args = append(args, "--integrity", *cipherOptions.IntegrityMode())
		}
		if cipherOptions.KeySize() != nil {
			args = append(args, "--key-size", fmt.Sprintf("%d", *cipherOptions.KeySize()))
		}
	}
	args = append(args,
		"--luks2-metadata-size",
		strconv.Itoa(luks2MetadataSize)+"k",
		"--luks2-keyslots-size",
		strconv.Itoa(luks2KeySlotsSize)+"k",
		"--pbkdf-memory",
		strconv.Itoa(cryptsetupPBKDFMemoryLimit),
		devicePath,
		"-d",
		"-")

	return l.execCryptsetupCommand(&passphrase, args...)
}

// LuksOpen opens LUKS encrypted partition and sets up a mapping.
func (l *luksWrapper) Open(devicePath, mapperFile, passphrase string) (string, string, error) {
	// cryptsetup option --disable-keyring (introduced with cryptsetup v2.0.0)
	// will be ignored with luks1
	return l.execCryptsetupCommand(
		&passphrase,
		"luksOpen",
		devicePath,
		mapperFile,
		"--disable-keyring",
		"-d",
		"-")
}

// LuksResize resizes LUKS encrypted partition.
func (l *luksWrapper) Resize(mapperFile string) (string, string, error) {
	return l.execCryptsetupCommand(nil, "resize", mapperFile)
}

// LuksClose removes existing mapping.
func (l *luksWrapper) Close(mapperFile string) (string, string, error) {
	return l.execCryptsetupCommand(nil, "luksClose", mapperFile)
}

// LuksStatus returns encryption status of a provided device.
func (l *luksWrapper) Status(mapperFile string) (string, string, error) {
	return l.execCryptsetupCommand(nil, "status", mapperFile)
}

// LuksAddKey adds a new key to the specified slot.
func (l *luksWrapper) AddKey(devicePath, passphrase, newPassphrase, slot string) error {
	passFile, err := file.CreateTempFile("luks-", passphrase)
	if err != nil {
		return err
	}
	defer os.Remove(passFile.Name()) //nolint:errcheck // failed to delete temp file :-(

	newPassFile, err := file.CreateTempFile("luks-", newPassphrase)
	if err != nil {
		return err
	}
	defer os.Remove(newPassFile.Name()) //nolint:errcheck // failed to delete temp file :-(

	_, stderr, err := l.execCryptsetupCommand(
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
		exists, fErr := l.VerifyKey(devicePath, newPassphrase, slot)
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
		fErr = l.RemoveKey(devicePath, passphrase, slot)
		if fErr != nil {
			return fErr
		}

		// Now the slot is free, add the new key to it
		fErr = l.AddKey(devicePath, passphrase, newPassphrase, slot)
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
func (l *luksWrapper) RemoveKey(devicePath, passphrase, slot string) error {
	keyFile, err := file.CreateTempFile("luks-", passphrase)
	if err != nil {
		return err
	}
	defer os.Remove(keyFile.Name()) //nolint:errcheck // failed to delete temp file :-(

	_, stderr, err := l.execCryptsetupCommand(
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
func (l *luksWrapper) VerifyKey(devicePath, passphrase, slot string) (bool, error) {
	// Create a temp file that we will use to open the device
	keyFile, err := file.CreateTempFile("luks-", passphrase)
	if err != nil {
		return false, err
	}
	defer os.Remove(keyFile.Name()) //nolint:errcheck // failed to delete temp file :-(

	_, stderr, err := l.execCryptsetupCommand(
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

func (l *luksWrapper) execCryptsetupCommand(stdin *string, args ...string) (string, string, error) {
	var (
		program       = "cryptsetup"
		cmd           = exec.CommandContext(l.ctx, program, args...) // #nosec:G204, commands executing not vulnerable.
		sanitizedArgs = stripsecrets.InArgs(args)
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

	if errors.Is(l.ctx.Err(), context.DeadlineExceeded) {
		return stdout, stderr, fmt.Errorf("timeout occurred while running %s args: %v", program, sanitizedArgs)
	}

	if err != nil {
		return stdout, stderr, fmt.Errorf("an error (%v)"+
			" occurred while running %s args: %v", err, program, sanitizedArgs)
	}

	return stdout, stderr, err
}
