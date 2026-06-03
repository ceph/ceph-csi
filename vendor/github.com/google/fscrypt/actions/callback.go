/*
 * callback.go - defines how the caller of an action function passes along a key
 * to be used in this package.
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

package actions

import (
	"log"

	"github.com/pkg/errors"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/filesystem"
	"github.com/google/fscrypt/metadata"
)

// ProtectorInfo is the information a caller will receive about a Protector
// before they have to return the corresponding key. This is currently a
// read-only view of metadata.ProtectorData.
type ProtectorInfo struct {
	data *metadata.ProtectorData
}

// Descriptor is the Protector's descriptor used to uniquely identify it.
func (pi *ProtectorInfo) Descriptor() string { return pi.data.GetProtectorDescriptor() }

// Source indicates the type of the descriptor (how it should be unlocked).
func (pi *ProtectorInfo) Source() metadata.SourceType { return pi.data.GetSource() }

// Name is used to describe custom passphrase and raw key descriptors.
func (pi *ProtectorInfo) Name() string { return pi.data.GetName() }

// UID is used to identify the user for login passphrases.
func (pi *ProtectorInfo) UID() int64 { return pi.data.GetUid() }

// KeyFunc is passed to a function that will require some type of key.
// The info parameter is provided so the callback knows which key to provide.
// The retry parameter indicates that a previous key provided by this callback
// was incorrect (this allows for user feedback like "incorrect passphrase").
//
// For passphrase sources, the returned key should be a passphrase. For raw
// sources, the returned key should be a 256-bit cryptographic key. Consumers
// of the callback will wipe the returned key. An error returned by the callback
// will be propagated back to the caller.
type KeyFunc func(info ProtectorInfo, retry bool) (*crypto.Key, error)

// getWrappingKey uses the provided callback to get the wrapping key
// corresponding to the ProtectorInfo. This runs the passphrase hash for
// passphrase sources or just relays the callback for raw sources.
func getWrappingKey(info ProtectorInfo, keyFn KeyFunc, retry bool) (*crypto.Key, error) {
	// For raw key sources, we can just use the key directly.
	if info.Source() == metadata.SourceType_raw_key {
		return keyFn(info, retry)
	}

	// Run the passphrase hash for other sources.
	passphrase, err := keyFn(info, retry)
	if err != nil {
		return nil, err
	}
	defer passphrase.Wipe()

	log.Printf("running passphrase hash for protector %s", info.Descriptor())
	return crypto.PassphraseHash(passphrase, info.data.Salt, info.data.Costs)
}

// unwrapProtectorKey uses the provided callback and ProtectorInfo to return
// the unwrapped protector key. This will repeatedly call keyFn to get the
// wrapping key until the correct key is returned by the callback or the
// callback returns an error.
func unwrapProtectorKey(info ProtectorInfo, keyFn KeyFunc) (*crypto.Key, error) {
	retry := false
	for {
		wrappingKey, err := getWrappingKey(info, keyFn, retry)
		if err != nil {
			return nil, err
		}

		protectorKey, err := crypto.Unwrap(wrappingKey, info.data.WrappedKey)
		wrappingKey.Wipe()

		switch errors.Cause(err) {
		case nil:
			log.Printf("valid wrapping key for protector %s", info.Descriptor())
			return protectorKey, nil
		case crypto.ErrBadAuth:
			// After the first failure, we let the callback know we are retrying.
			log.Printf("invalid wrapping key for protector %s", info.Descriptor())
			retry = true
			continue
		default:
			return nil, err
		}
	}
}

// ProtectorOption is information about a protector relative to a Policy.
type ProtectorOption struct {
	ProtectorInfo
	// LinkedMount is the mountpoint for a linked protector. It is nil if
	// the protector is not a linked protector (or there is a LoadError).
	LinkedMount *filesystem.Mount
	// LoadError is non-nil if there was an error in getting the data for
	// the protector.
	LoadError error
}

// OptionFunc is passed to a function that needs to unlock a Policy.
// The callback is used to specify which protector should be used to unlock a
// Policy. The descriptor indicates which Policy we are using, while the options
// correspond to the valid Protectors protecting the Policy.
//
// The OptionFunc should either return a valid index into options, which
// corresponds to the desired protector, or an error (which will be propagated
// back to the caller).
type OptionFunc func(policyDescriptor string, options []*ProtectorOption) (int, error)
