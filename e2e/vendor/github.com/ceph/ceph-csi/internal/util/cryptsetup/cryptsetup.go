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
	"bufio"
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

	// LuksStatusIntegrityIdentifier is the identifier of the integrity section in a luks status.
	LuksStatusIntegrityIdentifier = "integrity"

	// LuksStatusIntegrityKeySize is the identifier of the integrity key size section in a luks status.
	LuksStatusIntegrityKeySize = "integrity keysize"

	// LuksStatusSectorSize is the identifier of the sector size section in a luks status.
	LuksStatusSectorSize = "sector size"

	// Recommended indicates a suitable cipher, key size and integrity mode tuple configuration.
	Recommended RecommendationLevel = "Recommended"

	// NotRecommended indicates a unsuitable, unknown or invalid cipher, key size and integrity mode tuple configuration.
	// NotRecommended configurations may result in failure.
	NotRecommended RecommendationLevel = "Not Recommended"

	// InvalidRecommended indicates that a cipher, key size and integrity mode tuple configuration is invalid.
	// Invalid configurations have a high chance of resulting in failure.
	InvalidRecommended RecommendationLevel = "Invalid"
)

const (
	aesXtsPlain      string = "aes-xts-plain64"
	aesXtsRandom     string = "aes-xts-random"
	serpentXtsPlain  string = "serpent-xts-plain64"
	serpentXtsRandom string = "serpent-xts-random"
)

var (
	hmacSha256 integritySpecification = integritySpecification{name: "hmac-sha256", luksStatusName: "hmac(sha256)"}
	hmacSha512 integritySpecification = integritySpecification{name: "hmac-sha512", luksStatusName: "hmac(sha512)"}
	aead       integritySpecification = integritySpecification{name: "aead", luksStatusName: "aead"}
)

// RecommendationLevel defines a "grade" for a chosen cipher, key size and integrity mode tuple.
type RecommendationLevel string

// integritySpecification maps an integrity algorithm's name to its expected representation in LUKS status output.
type integritySpecification struct {
	name           string
	luksStatusName string
}

// LuksStatus represents some of the information stored in a luks status output.
type LuksStatus struct {
	cipher        string
	keysize       uint
	integrityMode *string
	// if this is set we have to subtract it from the
	// keysize to get actual key size used for encryption
	intgrityKeySize *uint
	sectorSize      *uint
}

// Cipher returns the cipher name.
func (l *LuksStatus) Cipher() string {
	return l.cipher
}

// SetCipher sets the encryption algorithm and mode (e.g., "aes-xts-plain64").
func (l *LuksStatus) SetCipher(input string) {
	l.cipher = input
}

// CipherKeySize returns the effective encryption key size.
// It accounts for integrity protection by subtracting the integrity key size from the total reported key size.
func (l *LuksStatus) CipherKeySize() (uint, error) {
	// When integrityKeySize is set it means that a key for encryption and
	// a separate key for integrity protection is set. A luks status sums them
	// up in the 'keysize' field here we subtract them
	// so we can compare the encryption key sizes.
	if l.IntegrityKeySize() != nil {
		if l.keysize <= *l.intgrityKeySize {
			return 0, fmt.Errorf("cipher key size %d smaller than integrity key size %d", l.keysize, *l.intgrityKeySize)
		}

		return l.keysize - *l.intgrityKeySize, nil
	}

	return l.keysize, nil
}

// SetKeySize converts the size string to a uint and sets it, returning an error if parsing fails.
func (l *LuksStatus) SetKeySize(input string) error {
	return applySize(input, func(size uint) {
		l.keysize = size
	})
}

// IntegrityKeySize returns the integrity key size, or nil if non is configured.
func (l *LuksStatus) IntegrityKeySize() *uint {
	return clonePtr(l.intgrityKeySize)
}

// SetIntegrityKeySize parses the provided string and updates the integrity key size.
func (l *LuksStatus) SetIntegrityKeySize(input string) error {
	return applySize(input, func(size uint) {
		l.intgrityKeySize = &size
	})
}

// IntegrityMode returns the configured integrity mode or nil if not configured.
func (l *LuksStatus) IntegrityMode() *string {
	return clonePtr(l.integrityMode)
}

// SetIntegrityModeFromLuks translates the raw integrity mode string from a luksDump
// (e.g., "hmac(sha256)") into its equivalent cryptsetup identifier (e.g., "hmac-sha256").
func (l *LuksStatus) SetIntegrityModeFromLuks(input string) error {
	findCryptsetupMode := func(value string) (string, error) {
		for crypsetupMode, luksStatusMode := range integrityAllowRules.allowedValues {
			if value == luksStatusMode {
				return crypsetupMode, nil
			}
		}

		return "", fmt.Errorf("could not find luksStatus equivalent for %s", value)
	}
	result, err := findCryptsetupMode(input)
	if err != nil {
		return fmt.Errorf("could not parse Integrity mode, %w", err)
	}
	l.integrityMode = &result

	return nil
}

// SectorSize returns the device sector size in bytes, or nil if not set.
func (l *LuksStatus) SectorSize() *uint {
	return clonePtr(l.sectorSize)
}

// SetSectorSize parses and sets the device sector size in bytes, returning an error if parsing fails.
func (l *LuksStatus) SetSectorSize(input string) error {
	return applySize(input, func(size uint) {
		l.sectorSize = &size
	})
}

// CipherRecommendation defines the rules for the recommendation.
// If key and integrity same recommendation level choose the lower one!
// What about keeping a known good list, and reporting a warning about an unknown combination?
type CipherRecommendation struct {
	// Defines the recommendation for different key sizes *with this cipher*.
	recommendedKeySizes map[uint]RecommendationLevel

	// Defines the recommendations for different integrity modes *with this cipher*.
	recommendedIntegrity map[string]RecommendationLevel
}

var recommendationConfig = map[string]CipherRecommendation{
	aesXtsPlain: {
		recommendedKeySizes: map[uint]RecommendationLevel{
			128:  NotRecommended,
			256:  NotRecommended, // too short for xts mode. See xts mode "tweak"
			512:  Recommended,
			1024: Recommended,
		},
		recommendedIntegrity: map[string]RecommendationLevel{
			hmacSha256.name: Recommended,
			hmacSha512.name: Recommended,
			aead.name:       InvalidRecommended,
			"":              NotRecommended,
		},
	},
	serpentXtsPlain: {
		recommendedKeySizes: map[uint]RecommendationLevel{
			128:  NotRecommended,
			256:  NotRecommended, // too short for xts mode. See xts mode "tweak"
			512:  Recommended,
			1024: Recommended,
		},
		recommendedIntegrity: map[string]RecommendationLevel{
			hmacSha256.name: Recommended,
			hmacSha512.name: Recommended,
			aead.name:       InvalidRecommended,
			"":              NotRecommended,
		},
	},
	aesXtsRandom: {
		recommendedKeySizes: map[uint]RecommendationLevel{
			128:  NotRecommended,
			256:  NotRecommended, // too short for xts mode. See xts mode "tweak"
			512:  Recommended,
			1024: Recommended,
		},
		recommendedIntegrity: map[string]RecommendationLevel{
			hmacSha256.name: Recommended,
			hmacSha512.name: Recommended,
			aead.name:       InvalidRecommended,
			"":              InvalidRecommended, // "*-random" means the IV is a nonce.
			// This nonce is stored using DmIntegrity. Without any options this is not possible.
		},
	},
	serpentXtsRandom: {
		recommendedKeySizes: map[uint]RecommendationLevel{
			128:  NotRecommended,
			256:  NotRecommended, // too short for xts mode. See xts mode "tweak"
			512:  Recommended,
			1024: Recommended,
		},
		recommendedIntegrity: map[string]RecommendationLevel{
			hmacSha256.name: Recommended,
			hmacSha512.name: Recommended,
			aead.name:       InvalidRecommended,
			"":              InvalidRecommended,
		},
	},
}

var levelScores = map[RecommendationLevel]int{
	Recommended:        1,
	NotRecommended:     0,
	InvalidRecommended: -1,
}

func minLevel(l1, l2 RecommendationLevel) RecommendationLevel {
	if levelScores[l1] < levelScores[l2] {
		return l1
	}

	return l2
}

// GetRecommendation valides the (cipher, key size, integrity mode) tuple.
// It validates against the recommendation saved in recommendationConfig.
// Does not check if selected EncryptionsOptions are allowed by rules.
func GetRecommendation(options EncryptionOptions) RecommendationLevel {
	recommendation := Recommended

	configs, ok := recommendationConfig[options.Cipher()]
	if !ok {
		return InvalidRecommended
	}
	if options.KeySize() != nil {
		if recKeySize, keyOk := configs.recommendedKeySizes[*options.KeySize()]; keyOk {
			recommendation = minLevel(recKeySize, recommendation)
		} else {
			recommendation = NotRecommended
		}
	}
	if options.IntegrityMode() != nil {
		if recIntegrity, integrityOk := configs.recommendedIntegrity[*options.IntegrityMode()]; integrityOk {
			recommendation = minLevel(recIntegrity, recommendation)
		} else {
			recommendation = NotRecommended
		}
	}

	return recommendation
}

// allowRules maintains a registry of permitted configuration values.
type allowRules[T any] struct {
	allowedValues map[string]T
	name          string
}

// enforce validates that the provided value exists in the allowlist, returning an error if it does not.
func (v *allowRules[T]) enforce(value string) error {
	if _, ok := v.allowedValues[value]; !ok {
		return fmt.Errorf("%s not allowed: %s", v.name, value)
	}

	return nil
}

var (
	cipherAllowRules = &allowRules[CipherRecommendation]{
		name: "cipher",
		// If the cipher is not in the recommendationConfig then it is not allowed
		allowedValues: recommendationConfig,
	}

	integrityAllowRules = &allowRules[string]{
		name: "integrity mode",
		// Problem: The strings that identify integrity modes are different
		// in a luksStatus and the cryptsetup input.
		// In this map Keys represent the cryptsetup input and values represent the
		// LuksStatus equivalent.
		allowedValues: map[string]string{
			hmacSha256.name: hmacSha256.luksStatusName,
			hmacSha512.name: hmacSha512.luksStatusName,
			aead.name:       aead.luksStatusName,
		},
	}
)

// EncryptionOptions defines the configuration parameters for volume encryption.
type EncryptionOptions struct {
	cipher        string
	keysize       *uint
	integrityMode *string
	sectorSize    *uint
}

// SectorSize returns the device sector size in bytes, or nil if not set.
func (e *EncryptionOptions) SectorSize() *uint {
	return clonePtr(e.sectorSize)
}

// SetSectorSize parses the input string and sets the device sector size in bytes.
func (e *EncryptionOptions) SetSectorSize(size string) error {
	return applySize(size, func(size uint) {
		e.sectorSize = &size
	})
}

// IntegrityMode returns the configured integrity protection mode, or nil if disabled.
func (e *EncryptionOptions) IntegrityMode() *string {
	return clonePtr(e.integrityMode)
}

// // SetIntegrityMode validates and sets the integrity protection mode (e.g., "hmac-sha256").
func (e *EncryptionOptions) SetIntegrityMode(integrity string) error {
	if err := integrityAllowRules.enforce(integrity); err != nil {
		return fmt.Errorf("cannot set integrity mode '%s': %w", integrity, err)
	}
	e.integrityMode = &integrity

	return nil
}

// KeySize returns the encryption key size in bits, or nil if the default is used.
func (e *EncryptionOptions) KeySize() *uint {
	return clonePtr(e.keysize)
}

// SetKeySize parses and sets the encryption key size in bits.
func (e *EncryptionOptions) SetKeySize(keysize string) error {
	return applySize(keysize, func(keysize uint) {
		e.keysize = &keysize
	})
}

// Cipher returns the encryption cipher suite.
func (e *EncryptionOptions) Cipher() string {
	return e.cipher
}

// SetCipher validates and if successful sets the encryption cipher suite.
func (e *EncryptionOptions) SetCipher(cipher string) error {
	if err := cipherAllowRules.enforce(cipher); err != nil {
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
func (e *EncryptionOptions) Equal(luksStatus LuksStatus) (bool, error) {
	if e.Cipher() != luksStatus.Cipher() {
		return false, nil
	}
	if e.KeySize() != nil {
		cipherKeySize, err := luksStatus.CipherKeySize()
		if err != nil {
			return false, err
		}
		if *e.KeySize() != cipherKeySize {
			return false, nil
		}
	}
	if e.IntegrityMode() != nil {
		if (luksStatus.IntegrityMode() == nil) || (*e.IntegrityMode() != *luksStatus.IntegrityMode()) {
			return false, nil
		}
	}
	if e.SectorSize() != nil {
		if (luksStatus.SectorSize() == nil) || (*e.SectorSize() != *luksStatus.SectorSize()) {
			return false, nil
		}
	}

	return true, nil
}

// ParseLuksStatus parses parts of the output of a "cryptsetup luksStatus <device>" command.
func ParseLuksStatus(dump string) (*LuksStatus, error) {
	var (
		luksStatus *LuksStatus
		err        error
	)
	scanner := bufio.NewScanner(strings.NewReader(dump))
	initLuksStatus := func() {
		// Only init options when key present
		if luksStatus == nil {
			luksStatus = &LuksStatus{}
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		parts := strings.SplitN(trimmedLine, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case LuksStatusCipherIdentifier:
			initLuksStatus()
			luksStatus.SetCipher(value)

		case LuksStatusKeySizeIdentifier:
			initLuksStatus()
			size, errs := parseLuksStatusSize(value)
			if errs != nil {
				return nil, fmt.Errorf("failed to parse luks status key size %w", err)
			}
			err = luksStatus.SetKeySize(size)

		case LuksStatusIntegrityIdentifier:
			initLuksStatus()
			err = luksStatus.SetIntegrityModeFromLuks(value)

		case LuksStatusIntegrityKeySize:
			initLuksStatus()
			size, errs := parseLuksStatusSize(value)
			if errs != nil {
				return nil, fmt.Errorf("failed to parse luks status integrity key size %w", err)
			}
			err = luksStatus.SetIntegrityKeySize(size)

		case LuksStatusSectorSize:
			initLuksStatus()
			size, errs := parseLuksStatusSize(value)
			if errs != nil {
				return nil, fmt.Errorf("failed to parse luks status sector seize, %w", errs)
			}
			err = luksStatus.SetSectorSize(size)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to set luks status attribute %q: %w, %s", key, err, dump)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading luks status: %w", err)
	}
	if luksStatus == nil {
		return nil, fmt.Errorf("parsing failed with status: %s", dump)
	}

	return luksStatus, nil
}

func parseLuksStatusSize(input string) (string, error) {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) < 1 { // will only split into one part of no unit like [bytes] or [bits] is given
		return "", fmt.Errorf("could not parse %s, %+v", input, parts)
	}
	sizeString := parts[0]

	return sizeString, nil
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
	IsIntegrityProtected(mapperFile string) (bool, error)
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
			args = append(args, "--key-size", strconv.FormatUint(uint64(*cipherOptions.KeySize()), 10))
		}
		if cipherOptions.SectorSize() != nil {
			args = append(args, "--sector-size", strconv.FormatUint(uint64(*cipherOptions.SectorSize()), 10))
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

// IsIntegrityProtected checks if a block device is integrity protected.
func (l *luksWrapper) IsIntegrityProtected(mapperFile string) (bool, error) {
	stdout, stderr, err := l.Status(mapperFile)
	if err != nil || stderr != "" {
		return false, fmt.Errorf("cryptsetup status failed, with msg: %s and error %w", stderr, err)
	}
	luksStatus, err := ParseLuksStatus(stdout)
	if err != nil {
		return false, fmt.Errorf("failed to parse crypsetup status %w", err)
	}
	if luksStatus.integrityMode != nil {
		return true, nil
	}

	return false, nil
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

	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(l.ctx.Err(), context.DeadlineExceeded) {
		return stdout, stderr, fmt.Errorf("timeout occurred while running %s args: %v", program, sanitizedArgs)
	}

	if err != nil {
		if e := strings.TrimSpace(stderr); e != "" {
			return stdout, stderr, fmt.Errorf("error running %s args: %v: %w; stderr: %s", program, sanitizedArgs, err, e)
		}

		return stdout, stderr, fmt.Errorf("error running %s args: %v: %w", program, sanitizedArgs, err)
	}

	return stdout, stderr, nil
}

func clonePtr[T any](pointer *T) *T {
	if pointer == nil {
		return nil
	}
	result := *pointer

	return &result
}

func applySize(input string, setter func(size uint)) error {
	size, err := strconv.ParseUint(input, 10, 32)
	if err != nil {
		return fmt.Errorf("could not parse number from %q: %w", input, err)
	}
	setter(uint(size))

	return nil
}
