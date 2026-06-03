/*
 * protector.go - functions for dealing with protectors
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
	"fmt"
	"log"
	"os/user"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/metadata"
	"github.com/google/fscrypt/util"
)

// LoginProtectorMountpoint is the mountpoint where login protectors are stored.
// This can be overridden by the user of this package.
var LoginProtectorMountpoint = "/"

// ErrLoginProtectorExists indicates that a user already has a login protector.
type ErrLoginProtectorExists struct {
	User *user.User
}

func (err *ErrLoginProtectorExists) Error() string {
	return fmt.Sprintf("user %q already has a login protector", err.User.Username)
}

// ErrLoginProtectorName indicates that a name was given for a login protector.
type ErrLoginProtectorName struct {
	Name string
	User *user.User
}

func (err *ErrLoginProtectorName) Error() string {
	return fmt.Sprintf(`cannot assign name %q to new login protector for
	user %q because login protectors are identified by user, not by name.`,
		err.Name, err.User.Username)
}

// ErrMissingProtectorName indicates that a protector name is needed.
type ErrMissingProtectorName struct {
	Source metadata.SourceType
}

func (err *ErrMissingProtectorName) Error() string {
	return fmt.Sprintf("%s protectors must be named", err.Source)
}

// ErrProtectorNameExists indicates that a protector name already exists.
type ErrProtectorNameExists struct {
	Name string
}

func (err *ErrProtectorNameExists) Error() string {
	return fmt.Sprintf("there is already a protector named %q", err.Name)
}

// checkForProtectorWithName returns an error if there is already a protector
// on the filesystem with a specific name (or if we cannot read the necessary
// data).
func checkForProtectorWithName(ctx *Context, name string) error {
	options, err := ctx.ProtectorOptions()
	if err != nil {
		return err
	}
	for _, option := range options {
		if option.Name() == name {
			return &ErrProtectorNameExists{name}
		}
	}
	return nil
}

// checkIfUserHasLoginProtector returns an error if there is already a login
// protector on the filesystem for a specific user (or if we cannot read the
// necessary data).
func checkIfUserHasLoginProtector(ctx *Context, uid int64) error {
	options, err := ctx.ProtectorOptions()
	if err != nil {
		return err
	}
	for _, option := range options {
		if option.Source() == metadata.SourceType_pam_passphrase && option.UID() == uid {
			return &ErrLoginProtectorExists{ctx.TargetUser}
		}
	}
	return nil
}

// Protector represents an unlocked protector, so it contains the ProtectorData
// as well as the actual protector key. These unlocked Protectors are necessary
// to unlock policies and create new polices. As with the key struct, a
// Protector should be wiped after use.
type Protector struct {
	Context         *Context
	data            *metadata.ProtectorData
	key             *crypto.Key
	created         bool
	ownerIfCreating *user.User
}

// CreateProtector creates an unlocked protector with a given name (name only
// needed for custom and raw protector types). The keyFn provided to create the
// Protector key will only be called once. If an error is returned, no data has
// been changed on the filesystem.
func CreateProtector(ctx *Context, name string, keyFn KeyFunc, owner *user.User) (*Protector, error) {
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}
	// Sanity checks for names
	if ctx.Config.Source == metadata.SourceType_pam_passphrase {
		// login protectors don't need a name (we use the username instead)
		if name != "" {
			return nil, &ErrLoginProtectorName{name, ctx.TargetUser}
		}
	} else {
		// non-login protectors need a name (so we can distinguish between them)
		if name == "" {
			return nil, &ErrMissingProtectorName{ctx.Config.Source}
		}
		// we don't want to duplicate naming
		if err := checkForProtectorWithName(ctx, name); err != nil {
			return nil, err
		}
	}

	var err error
	protector := &Protector{
		Context: ctx,
		data: &metadata.ProtectorData{
			Name:   name,
			Source: ctx.Config.Source,
		},
		created:         true,
		ownerIfCreating: owner,
	}

	// Extra data is needed for some SourceTypes
	switch protector.data.Source {
	case metadata.SourceType_pam_passphrase:
		// As the pam passphrases are user specific, we also store the
		// UID for this kind of source.
		protector.data.Uid = int64(util.AtoiOrPanic(ctx.TargetUser.Uid))
		// Make sure we aren't duplicating protectors
		if err = checkIfUserHasLoginProtector(ctx, protector.data.Uid); err != nil {
			return nil, err
		}
		fallthrough
	case metadata.SourceType_custom_passphrase:
		// Our passphrase sources need costs and a random salt.
		if protector.data.Salt, err = crypto.NewRandomBuffer(metadata.SaltLen); err != nil {
			return nil, err
		}

		protector.data.Costs = ctx.Config.HashCosts
	}

	// Randomly create the underlying protector key (and wipe if we fail)
	if protector.key, err = crypto.NewRandomKey(metadata.InternalKeyLen); err != nil {
		return nil, err
	}
	protector.data.ProtectorDescriptor, err = crypto.ComputeKeyDescriptor(protector.key, 1)
	if err != nil {
		protector.Lock()
		return nil, err
	}

	if err = protector.Rewrap(keyFn); err != nil {
		protector.Lock()
		return nil, err
	}

	return protector, nil
}

// GetProtector retrieves a Protector with a specific descriptor. The Protector
// is still locked in this case, so it must be unlocked before using certain
// methods.
func GetProtector(ctx *Context, descriptor string) (*Protector, error) {
	log.Printf("Getting protector %s", descriptor)
	err := ctx.checkContext()
	if err != nil {
		return nil, err
	}

	protector := &Protector{Context: ctx}
	protector.data, err = ctx.Mount.GetRegularProtector(descriptor, ctx.TrustedUser)
	return protector, err
}

// GetProtectorFromOption retrieves a protector based on a protector option.
// If the option had a load error, this function returns that error. The
// Protector is still locked in this case, so it must be unlocked before using
// certain methods.
func GetProtectorFromOption(ctx *Context, option *ProtectorOption) (*Protector, error) {
	log.Printf("Getting protector %s from option", option.Descriptor())
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}
	if option.LoadError != nil {
		return nil, option.LoadError
	}

	// Replace the context if this is a linked protector
	if option.LinkedMount != nil {
		ctx = &Context{ctx.Config, option.LinkedMount, ctx.TargetUser, ctx.TrustedUser}
	}
	return &Protector{Context: ctx, data: option.data}, nil
}

// Descriptor returns the protector descriptor.
func (protector *Protector) Descriptor() string {
	return protector.data.ProtectorDescriptor
}

// Destroy removes a protector from the filesystem. The internal key should
// still be wiped with Lock().
func (protector *Protector) Destroy() error {
	return protector.Context.Mount.RemoveProtector(protector.Descriptor())
}

// Revert destroys a protector if it was created, but does nothing if it was
// just queried from the filesystem.
func (protector *Protector) Revert() error {
	if !protector.created {
		return nil
	}
	return protector.Destroy()
}

func (protector *Protector) String() string {
	return fmt.Sprintf("Protector: %s\nMountpoint: %s\nSource: %s\nName: %s\nCosts: %v\nUID: %d",
		protector.Descriptor(), protector.Context.Mount, protector.data.Source,
		protector.data.Name, protector.data.Costs, protector.data.Uid)
}

// Unlock unwraps the Protector's internal key. The keyFn provided to unwrap the
// Protector key will be retried as necessary to get the correct key. Lock()
// should be called after use. Does nothing if protector is already unlocked.
func (protector *Protector) Unlock(keyFn KeyFunc) (err error) {
	if protector.key != nil {
		return
	}
	protector.key, err = unwrapProtectorKey(ProtectorInfo{protector.data}, keyFn)
	return
}

// Lock wipes a Protector's internal Key. It should always be called after using
// an unlocked Protector. This is often done with a defer statement. There is
// no effect if called multiple times.
func (protector *Protector) Lock() error {
	err := protector.key.Wipe()
	protector.key = nil
	return err
}

// Rewrap updates the data that is wrapping the Protector Key. This is useful if
// a user's password has changed, for example. The keyFn provided to rewrap
// the Protector key will only be called once. Requires unlocked Protector.
func (protector *Protector) Rewrap(keyFn KeyFunc) error {
	if protector.key == nil {
		return ErrLocked
	}
	wrappingKey, err := getWrappingKey(ProtectorInfo{protector.data}, keyFn, false)
	if err != nil {
		return err
	}

	// Revert change to wrapped key on failure
	oldWrappedKey := protector.data.WrappedKey
	defer func() {
		wrappingKey.Wipe()
		if err != nil {
			protector.data.WrappedKey = oldWrappedKey
		}
	}()

	if protector.data.WrappedKey, err = crypto.Wrap(wrappingKey, protector.key); err != nil {
		return err
	}

	return protector.Context.Mount.AddProtector(protector.data, protector.ownerIfCreating)
}
