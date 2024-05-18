/*
 * context.go - top-level interface to fscrypt packages
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

// Package actions is the high-level interface to the fscrypt packages. The
// functions here roughly correspond with commands for the tool in cmd/fscrypt.
// All of the actions include a significant amount of logging, so that good
// output can be provided for cmd/fscrypt's verbose mode.
// The top-level actions currently include:
//	- Creating a new config file
//	- Creating a context on which to perform actions
//	- Creating, unlocking, and modifying Protectors
//	- Creating, unlocking, and modifying Policies
package actions

import (
	"log"
	"os/user"

	"github.com/pkg/errors"

	"github.com/google/fscrypt/filesystem"
	"github.com/google/fscrypt/keyring"
	"github.com/google/fscrypt/metadata"
	"github.com/google/fscrypt/util"
)

// ErrLocked indicates that the key hasn't been unwrapped yet.
var ErrLocked = errors.New("key needs to be unlocked first")

// Context contains the necessary global state to perform most of fscrypt's
// actions.
type Context struct {
	// Config is the struct loaded from the global config file. It can be
	// modified after being loaded to customise parameters.
	Config *metadata.Config
	// Mount is the filesystem relative to which all Protectors and Policies
	// are added, edited, removed, and applied, and to which policies using
	// the filesystem keyring are provisioned.
	Mount *filesystem.Mount
	// TargetUser is the user for whom protectors are created, and to whose
	// keyring policies using the user keyring are provisioned.  It's also
	// the user for whom the keys are claimed in the filesystem keyring when
	// v2 policies are provisioned.
	TargetUser *user.User
	// TrustedUser is the user for whom policies and protectors are allowed
	// to be read.  Specifically, if TrustedUser is set, then only
	// policies and protectors owned by TrustedUser or by root will be
	// allowed to be read.  If it's nil, then all policies and protectors
	// the process has filesystem-level read access to will be allowed.
	TrustedUser *user.User
}

// NewContextFromPath makes a context for the filesystem containing the
// specified path and whose Config is loaded from the global config file. On
// success, the Context contains a valid Config and Mount. The target user
// defaults to the current effective user if none is specified.
func NewContextFromPath(path string, targetUser *user.User) (*Context, error) {
	ctx, err := newContextFromUser(targetUser)
	if err != nil {
		return nil, err
	}
	if ctx.Mount, err = filesystem.FindMount(path); err != nil {
		return nil, err
	}

	log.Printf("%s is on %s filesystem %q (%s)", path,
		ctx.Mount.FilesystemType, ctx.Mount.Path, ctx.Mount.Device)
	return ctx, nil
}

// NewContextFromMountpoint makes a context for the filesystem at the specified
// mountpoint and whose Config is loaded from the global config file. On
// success, the Context contains a valid Config and Mount. The target user
// defaults to the current effective user if none is specified.
func NewContextFromMountpoint(mountpoint string, targetUser *user.User) (*Context, error) {
	ctx, err := newContextFromUser(targetUser)
	if err != nil {
		return nil, err
	}
	if ctx.Mount, err = filesystem.GetMount(mountpoint); err != nil {
		return nil, err
	}

	log.Printf("found %s filesystem %q (%s)", ctx.Mount.FilesystemType,
		ctx.Mount.Path, ctx.Mount.Device)
	return ctx, nil
}

// newContextFromUser makes a context with the corresponding target user, and
// whose Config is loaded from the global config file. If the target user is
// nil, the effective user is used.
func newContextFromUser(targetUser *user.User) (*Context, error) {
	var err error
	if targetUser == nil {
		if targetUser, err = util.EffectiveUser(); err != nil {
			return nil, err
		}
	}

	ctx := &Context{TargetUser: targetUser}
	if ctx.Config, err = getConfig(); err != nil {
		return nil, err
	}

	// By default, when running as a non-root user we only read policies and
	// protectors owned by the user or root.  When running as root, we allow
	// reading all policies and protectors.
	if !ctx.Config.GetAllowCrossUserMetadata() && !util.IsUserRoot() {
		ctx.TrustedUser, err = util.EffectiveUser()
		if err != nil {
			return nil, err
		}
	}

	log.Printf("creating context for user %q", targetUser.Username)
	return ctx, nil
}

// checkContext verifies that the context contains a valid config and a mount
// which is being used with fscrypt.
func (ctx *Context) checkContext() error {
	if err := ctx.Config.CheckValidity(); err != nil {
		return &ErrBadConfig{ctx.Config, err}
	}
	return ctx.Mount.CheckSetup(ctx.TrustedUser)
}

func (ctx *Context) getKeyringOptions() *keyring.Options {
	return &keyring.Options{
		Mount:                     ctx.Mount,
		User:                      ctx.TargetUser,
		UseFsKeyringForV1Policies: ctx.Config.GetUseFsKeyringForV1Policies(),
	}
}

// getProtectorOption returns the ProtectorOption for the protector on the
// context's mountpoint with the specified descriptor.
func (ctx *Context) getProtectorOption(protectorDescriptor string) *ProtectorOption {
	mnt, data, err := ctx.Mount.GetProtector(protectorDescriptor, ctx.TrustedUser)
	if err != nil {
		return &ProtectorOption{ProtectorInfo{}, nil, err}
	}

	info := ProtectorInfo{data}
	// No linked path if on the same mountpoint
	if mnt == ctx.Mount {
		return &ProtectorOption{info, nil, nil}
	}
	return &ProtectorOption{info, mnt, nil}
}

// ProtectorOptions creates a slice of all the options for all of the Protectors
// on the Context's mountpoint.
func (ctx *Context) ProtectorOptions() ([]*ProtectorOption, error) {
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}
	descriptors, err := ctx.Mount.ListProtectors(ctx.TrustedUser)
	if err != nil {
		return nil, err
	}

	options := make([]*ProtectorOption, len(descriptors))
	for i, descriptor := range descriptors {
		options[i] = ctx.getProtectorOption(descriptor)
	}
	return options, nil
}
