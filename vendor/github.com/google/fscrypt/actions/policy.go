/*
 * policy.go - functions for dealing with policies
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
	"os"
	"os/user"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/filesystem"
	"github.com/google/fscrypt/keyring"
	"github.com/google/fscrypt/metadata"
	"github.com/google/fscrypt/util"
)

// ErrAccessDeniedPossiblyV2 indicates that a directory's encryption policy
// couldn't be retrieved due to "permission denied", but it looks like it's due
// to the directory using a v2 policy but the kernel not supporting it.
type ErrAccessDeniedPossiblyV2 struct {
	DirPath string
}

func (err *ErrAccessDeniedPossiblyV2) Error() string {
	return fmt.Sprintf(`
	failed to get encryption policy of %s: permission denied

	This may be caused by the directory using a v2 encryption policy and the
	current kernel not supporting it. If indeed the case, then this
	directory can only be used on kernel v5.4 and later. You can create
	directories accessible on older kernels by changing policy_version to 1
	in %s.`,
		err.DirPath, ConfigFileLocation)
}

// ErrAlreadyProtected indicates that a policy is already protected by the given
// protector.
type ErrAlreadyProtected struct {
	Policy    *Policy
	Protector *Protector
}

func (err *ErrAlreadyProtected) Error() string {
	return fmt.Sprintf("policy %s is already protected by protector %s",
		err.Policy.Descriptor(), err.Protector.Descriptor())
}

// ErrDifferentFilesystem indicates that a policy can't be applied to a
// directory on a different filesystem.
type ErrDifferentFilesystem struct {
	PolicyMount *filesystem.Mount
	PathMount   *filesystem.Mount
}

func (err *ErrDifferentFilesystem) Error() string {
	return fmt.Sprintf(`cannot apply policy from filesystem %q to a
	directory on filesystem %q. Policies may only protect files on the same
	filesystem.`, err.PolicyMount.Path, err.PathMount.Path)
}

// ErrMissingPolicyMetadata indicates that a directory is encrypted but its
// policy metadata cannot be found.
type ErrMissingPolicyMetadata struct {
	Mount      *filesystem.Mount
	DirPath    string
	Descriptor string
}

func (err *ErrMissingPolicyMetadata) Error() string {
	return fmt.Sprintf(`filesystem %q does not contain the policy metadata
	for %q. This directory has either been encrypted with another tool (such
	as e4crypt), or the file %q has been deleted.`,
		err.Mount.Path, err.DirPath,
		err.Mount.PolicyPath(err.Descriptor))
}

// ErrNotProtected indicates that the given policy is not protected by the given
// protector.
type ErrNotProtected struct {
	PolicyDescriptor    string
	ProtectorDescriptor string
}

func (err *ErrNotProtected) Error() string {
	return fmt.Sprintf(`policy %s is not protected by protector %s`,
		err.PolicyDescriptor, err.ProtectorDescriptor)
}

// ErrOnlyProtector indicates that the last protector can't be removed from a
// policy.
type ErrOnlyProtector struct {
	Policy *Policy
}

func (err *ErrOnlyProtector) Error() string {
	return fmt.Sprintf(`cannot remove the only protector from policy %s. A
	policy must have at least one protector.`, err.Policy.Descriptor())
}

// ErrPolicyMetadataMismatch indicates that the policy metadata for an encrypted
// directory is inconsistent with that directory.
type ErrPolicyMetadataMismatch struct {
	DirPath   string
	Mount     *filesystem.Mount
	PathData  *metadata.PolicyData
	MountData *metadata.PolicyData
}

func (err *ErrPolicyMetadataMismatch) Error() string {
	return fmt.Sprintf(`inconsistent metadata between encrypted directory %q
	and its corresponding metadata file %q.

	Directory has descriptor:%s %s

	Metadata file has descriptor:%s %s`,
		err.DirPath, err.Mount.PolicyPath(err.PathData.KeyDescriptor),
		err.PathData.KeyDescriptor, err.PathData.Options,
		err.MountData.KeyDescriptor, err.MountData.Options)
}

// PurgeAllPolicies removes all policy keys on the filesystem from the kernel
// keyring. In order for this to fully take effect, the filesystem may also need
// to be unmounted or caches dropped.
func PurgeAllPolicies(ctx *Context) error {
	if err := ctx.checkContext(); err != nil {
		return err
	}
	policies, err := ctx.Mount.ListPolicies(nil)
	if err != nil {
		return err
	}

	for _, policyDescriptor := range policies {
		err = keyring.RemoveEncryptionKey(policyDescriptor, ctx.getKeyringOptions(), false)
		switch errors.Cause(err) {
		case nil, keyring.ErrKeyNotPresent:
			// We don't care if the key has already been removed
		case keyring.ErrKeyFilesOpen:
			log.Printf("Key for policy %s couldn't be fully removed because some files are still in-use",
				policyDescriptor)
		case keyring.ErrKeyAddedByOtherUsers:
			log.Printf("Key for policy %s couldn't be fully removed because other user(s) have added it too",
				policyDescriptor)
		default:
			return err
		}
	}
	return nil
}

// Policy represents an unlocked policy, so it contains the PolicyData as well
// as the actual protector key. These unlocked Polices can then be applied to a
// directory, or have their key material inserted into the keyring (which will
// allow encrypted files to be accessed). As with the key struct, a Policy
// should be wiped after use.
type Policy struct {
	Context             *Context
	data                *metadata.PolicyData
	key                 *crypto.Key
	created             bool
	ownerIfCreating     *user.User
	newLinkedProtectors []string
}

// CreatePolicy creates a Policy protected by given Protector and stores the
// appropriate data on the filesystem. On error, no data is changed on the
// filesystem.
func CreatePolicy(ctx *Context, protector *Protector) (*Policy, error) {
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}
	// Randomly create the underlying policy key (and wipe if we fail)
	key, err := crypto.NewRandomKey(metadata.PolicyKeyLen)
	if err != nil {
		return nil, err
	}

	keyDescriptor, err := crypto.ComputeKeyDescriptor(key, ctx.Config.Options.PolicyVersion)
	if err != nil {
		key.Wipe()
		return nil, err
	}

	policy := &Policy{
		Context: ctx,
		data: &metadata.PolicyData{
			Options:       ctx.Config.Options,
			KeyDescriptor: keyDescriptor,
		},
		key:     key,
		created: true,
	}

	policy.ownerIfCreating, err = getOwnerOfMetadataForProtector(protector)
	if err != nil {
		policy.Lock()
		return nil, err
	}

	if err = policy.AddProtector(protector); err != nil {
		policy.Lock()
		return nil, err
	}

	return policy, nil
}

// GetPolicy retrieves a locked policy with a specific descriptor. The Policy is
// still locked in this case, so it must be unlocked before using certain
// methods.
func GetPolicy(ctx *Context, descriptor string) (*Policy, error) {
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}
	data, err := ctx.Mount.GetPolicy(descriptor, ctx.TrustedUser)
	if err != nil {
		return nil, err
	}
	log.Printf("got data for %s from %q", descriptor, ctx.Mount.Path)

	return &Policy{Context: ctx, data: data}, nil
}

// GetPolicyFromPath returns the locked policy descriptor for a file on the
// filesystem. The Policy is still locked in this case, so it must be unlocked
// before using certain methods. An error is returned if the metadata is
// inconsistent or the path is not encrypted.
func GetPolicyFromPath(ctx *Context, path string) (*Policy, error) {
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}

	// We double check that the options agree for both the data we get from
	// the path, and the data we get from the mountpoint.
	pathData, err := metadata.GetPolicy(path)
	err = ctx.Mount.EncryptionSupportError(err)
	if err != nil {
		// On kernels that don't support v2 encryption policies, trying
		// to open a directory with a v2 policy simply gave EACCES. This
		// is ambiguous with other errors, but try to detect this case
		// and show a better error message.
		if os.IsPermission(err) &&
			filesystem.HaveReadAccessTo(path) &&
			!keyring.IsFsKeyringSupported(ctx.Mount) {
			return nil, &ErrAccessDeniedPossiblyV2{path}
		}
		return nil, err
	}
	descriptor := pathData.KeyDescriptor
	log.Printf("found policy %s for %q", descriptor, path)

	mountData, err := ctx.Mount.GetPolicy(descriptor, ctx.TrustedUser)
	if err != nil {
		log.Printf("getting policy metadata: %v", err)
		if _, ok := err.(*filesystem.ErrPolicyNotFound); ok {
			return nil, &ErrMissingPolicyMetadata{ctx.Mount, path, descriptor}
		}
		return nil, err
	}
	log.Printf("found data for policy %s on %q", descriptor, ctx.Mount.Path)

	if !proto.Equal(pathData.Options, mountData.Options) ||
		pathData.KeyDescriptor != mountData.KeyDescriptor {
		return nil, &ErrPolicyMetadataMismatch{path, ctx.Mount, pathData, mountData}
	}
	log.Print("data from filesystem and path agree")

	return &Policy{Context: ctx, data: mountData}, nil
}

// ProtectorOptions creates a slice of ProtectorOptions for the protectors
// protecting this policy.
func (policy *Policy) ProtectorOptions() []*ProtectorOption {
	options := make([]*ProtectorOption, len(policy.data.WrappedPolicyKeys))
	for i, wrappedPolicyKey := range policy.data.WrappedPolicyKeys {
		options[i] = policy.Context.getProtectorOption(wrappedPolicyKey.ProtectorDescriptor)
	}
	return options
}

// ProtectorDescriptors creates a slice of the Protector descriptors for the
// protectors protecting this policy.
func (policy *Policy) ProtectorDescriptors() []string {
	descriptors := make([]string, len(policy.data.WrappedPolicyKeys))
	for i, wrappedPolicyKey := range policy.data.WrappedPolicyKeys {
		descriptors[i] = wrappedPolicyKey.ProtectorDescriptor
	}
	return descriptors
}

// Descriptor returns the key descriptor for this policy.
func (policy *Policy) Descriptor() string {
	return policy.data.KeyDescriptor
}

// Options returns the encryption options of this policy.
func (policy *Policy) Options() *metadata.EncryptionOptions {
	return policy.data.Options
}

// Version returns the version of this policy.
func (policy *Policy) Version() int64 {
	return policy.data.Options.PolicyVersion
}

// Destroy removes a policy from the filesystem. It also removes any new
// protector links that were created for the policy. This does *not* wipe the
// policy's internal key from memory; use Lock() to do that.
func (policy *Policy) Destroy() error {
	for _, protectorDescriptor := range policy.newLinkedProtectors {
		policy.Context.Mount.RemoveProtector(protectorDescriptor)
	}
	return policy.Context.Mount.RemovePolicy(policy.Descriptor())
}

// Revert destroys a policy if it was created, but does nothing if it was just
// queried from the filesystem.
func (policy *Policy) Revert() error {
	if !policy.created {
		return nil
	}
	return policy.Destroy()
}

func (policy *Policy) String() string {
	return fmt.Sprintf("Policy: %s\nMountpoint: %s\nOptions: %v\nProtectors:%+v",
		policy.Descriptor(), policy.Context.Mount, policy.data.Options,
		policy.ProtectorDescriptors())
}

// Unlock unwraps the Policy's internal key. As a Protector is needed to unlock
// the Policy, callbacks to select the Policy and get the key are needed. This
// method will retry the keyFn as necessary to get the correct key for the
// selected protector. Does nothing if policy is already unlocked.
func (policy *Policy) Unlock(optionFn OptionFunc, keyFn KeyFunc) error {
	if policy.key != nil {
		return nil
	}
	options := policy.ProtectorOptions()

	// The OptionFunc indicates which option and wrapped key we should use.
	idx, err := optionFn(policy.Descriptor(), options)
	if err != nil {
		return err
	}
	option := options[idx]
	if option.LoadError != nil {
		return option.LoadError
	}

	log.Printf("protector %s selected in callback", option.Descriptor())
	protectorKey, err := unwrapProtectorKey(option.ProtectorInfo, keyFn)
	if err != nil {
		return err
	}
	defer protectorKey.Wipe()

	log.Printf("unwrapping policy %s with protector", policy.Descriptor())
	wrappedPolicyKey := policy.data.WrappedPolicyKeys[idx].WrappedKey
	policy.key, err = crypto.Unwrap(protectorKey, wrappedPolicyKey)
	return err
}

// UnlockWithProtector uses an unlocked Protector to unlock a policy. An error
// is returned if the Protector is not yet unlocked or does not protect the
// policy. Does nothing if policy is already unlocked.
func (policy *Policy) UnlockWithProtector(protector *Protector) error {
	if policy.key != nil {
		return nil
	}
	if protector.key == nil {
		return ErrLocked
	}
	idx, ok := policy.findWrappedKeyIndex(protector.Descriptor())
	if !ok {
		return &ErrNotProtected{policy.Descriptor(), protector.Descriptor()}
	}

	var err error
	wrappedPolicyKey := policy.data.WrappedPolicyKeys[idx].WrappedKey
	policy.key, err = crypto.Unwrap(protector.key, wrappedPolicyKey)
	return err
}

// Lock wipes a Policy's internal Key. It should always be called after using a
// Policy. This is often done with a defer statement. There is no effect if
// called multiple times.
func (policy *Policy) Lock() error {
	err := policy.key.Wipe()
	policy.key = nil
	return err
}

// UsesProtector returns if the policy is protected with the protector
func (policy *Policy) UsesProtector(protector *Protector) bool {
	_, ok := policy.findWrappedKeyIndex(protector.Descriptor())
	return ok
}

// getOwnerOfMetadataForProtector returns the User to whom the owner of any new
// policies or protector links for the given protector should be set.
//
// This will return a non-nil value only when the protector is a login protector
// and the process is running as root.  In this scenario, root is setting up
// encryption on the user's behalf, so we need to make new policies and
// protector links owned by the user (rather than root) to allow them to be read
// by the user, just like the login protector itself which is handled elsewhere.
func getOwnerOfMetadataForProtector(protector *Protector) (*user.User, error) {
	if protector.data.Source == metadata.SourceType_pam_passphrase && util.IsUserRoot() {
		owner, err := util.UserFromUID(protector.data.Uid)
		if err != nil {
			return nil, err
		}
		return owner, nil
	}
	return nil, nil
}

// AddProtector updates the data that is wrapping the Policy Key so that the
// provided Protector is now protecting the specified Policy. If an error is
// returned, no data has been changed. If the policy and protector are on
// different filesystems, a link will be created between them. The policy and
// protector must both be unlocked.
func (policy *Policy) AddProtector(protector *Protector) error {
	if policy.UsesProtector(protector) {
		return &ErrAlreadyProtected{policy, protector}
	}
	if policy.key == nil || protector.key == nil {
		return ErrLocked
	}

	// If the protector is on a different filesystem, we need to add a link
	// to it on the policy's filesystem.
	if policy.Context.Mount != protector.Context.Mount {
		log.Printf("policy on %s\n protector on %s\n", policy.Context.Mount, protector.Context.Mount)
		ownerIfCreating, err := getOwnerOfMetadataForProtector(protector)
		if err != nil {
			return err
		}
		isNewLink, err := policy.Context.Mount.AddLinkedProtector(
			protector.Descriptor(), protector.Context.Mount,
			protector.Context.TrustedUser, ownerIfCreating)
		if err != nil {
			return err
		}
		if isNewLink {
			policy.newLinkedProtectors = append(policy.newLinkedProtectors,
				protector.Descriptor())
		}
	} else {
		log.Printf("policy and protector both on %q", policy.Context.Mount)
	}

	// Create the wrapped policy key
	wrappedKey, err := crypto.Wrap(protector.key, policy.key)
	if err != nil {
		return err
	}

	// Append the wrapped key to the data
	policy.addKey(&metadata.WrappedPolicyKey{
		ProtectorDescriptor: protector.Descriptor(),
		WrappedKey:          wrappedKey,
	})

	if err := policy.commitData(); err != nil {
		// revert the addition on failure
		policy.removeKey(len(policy.data.WrappedPolicyKeys) - 1)
		return err
	}
	return nil
}

// RemoveProtector updates the data that is wrapping the Policy Key so that the
// protector with the given descriptor is no longer protecting the specified
// Policy.  If an error is returned, no data has been changed.  Note that the
// protector itself won't be removed, nor will a link to the protector be
// removed (in the case where the protector and policy are on different
// filesystems).  The policy can be locked or unlocked.
func (policy *Policy) RemoveProtector(protectorDescriptor string) error {
	idx, ok := policy.findWrappedKeyIndex(protectorDescriptor)
	if !ok {
		return &ErrNotProtected{policy.Descriptor(), protectorDescriptor}
	}

	if len(policy.data.WrappedPolicyKeys) == 1 {
		return &ErrOnlyProtector{policy}
	}

	// Remove the wrapped key from the data
	toRemove := policy.removeKey(idx)

	if err := policy.commitData(); err != nil {
		// revert the removal on failure (order is irrelevant)
		policy.addKey(toRemove)
		return err
	}
	return nil
}

// Apply sets the Policy on a specified directory. Currently we impose the
// additional constraint that policies and the directories they are applied to
// must reside on the same filesystem.
func (policy *Policy) Apply(path string) error {
	if pathMount, err := filesystem.FindMount(path); err != nil {
		return err
	} else if pathMount != policy.Context.Mount {
		return &ErrDifferentFilesystem{policy.Context.Mount, pathMount}
	}

	err := metadata.SetPolicy(path, policy.data)
	return policy.Context.Mount.EncryptionSupportError(err)
}

// GetProvisioningStatus returns the status of this policy's key in the keyring.
func (policy *Policy) GetProvisioningStatus() keyring.KeyStatus {
	status, _ := keyring.GetEncryptionKeyStatus(policy.Descriptor(),
		policy.Context.getKeyringOptions())
	return status
}

// IsProvisionedByTargetUser returns true if the policy's key is present in the
// target kernel keyring, but not if that keyring is a filesystem keyring and
// the key only been added by users other than Context.TargetUser.
func (policy *Policy) IsProvisionedByTargetUser() bool {
	return policy.GetProvisioningStatus() == keyring.KeyPresent
}

// Provision inserts the Policy key into the kernel keyring. This allows reading
// and writing of files encrypted with this directory. Requires unlocked Policy.
func (policy *Policy) Provision() error {
	if policy.key == nil {
		return ErrLocked
	}
	return keyring.AddEncryptionKey(policy.key, policy.Descriptor(),
		policy.Context.getKeyringOptions())
}

// Deprovision removes the Policy key from the kernel keyring. This prevents
// reading and writing to the directory --- unless the target keyring is a user
// keyring, in which case caches must be dropped too. If the Policy key was
// already removed, returns keyring.ErrKeyNotPresent.
func (policy *Policy) Deprovision(allUsers bool) error {
	return keyring.RemoveEncryptionKey(policy.Descriptor(),
		policy.Context.getKeyringOptions(), allUsers)
}

// NeedsUserKeyring returns true if Provision and Deprovision for this policy
// will use a user keyring (deprecated), not a filesystem keyring.
func (policy *Policy) NeedsUserKeyring() bool {
	return policy.Version() == 1 && !policy.Context.Config.GetUseFsKeyringForV1Policies()
}

// NeedsRootToProvision returns true if Provision and Deprovision will require
// root for this policy in the current configuration.
func (policy *Policy) NeedsRootToProvision() bool {
	return policy.Version() == 1 && policy.Context.Config.GetUseFsKeyringForV1Policies()
}

// CanBeAppliedWithoutProvisioning returns true if this process can apply this
// policy to a directory without first calling Provision.
func (policy *Policy) CanBeAppliedWithoutProvisioning() bool {
	return policy.Version() == 1 || util.IsUserRoot()
}

// commitData writes the Policy's current data to the filesystem.
func (policy *Policy) commitData() error {
	return policy.Context.Mount.AddPolicy(policy.data, policy.ownerIfCreating)
}

// findWrappedPolicyKey returns the index of the wrapped policy key
// corresponding to this policy and protector. The returned bool is false if no
// wrapped policy key corresponds to the specified protector, true otherwise.
func (policy *Policy) findWrappedKeyIndex(protectorDescriptor string) (int, bool) {
	for idx, wrappedPolicyKey := range policy.data.WrappedPolicyKeys {
		if wrappedPolicyKey.ProtectorDescriptor == protectorDescriptor {
			return idx, true
		}
	}
	return 0, false
}

// addKey adds the wrapped policy key to end of the wrapped key data.
func (policy *Policy) addKey(toAdd *metadata.WrappedPolicyKey) {
	policy.data.WrappedPolicyKeys = append(policy.data.WrappedPolicyKeys, toAdd)
}

// removeKey removes the wrapped policy key at the specified index. This
// does not preserve the order of the wrapped policy key array. If no index is
// specified the last key is removed.
func (policy *Policy) removeKey(index int) *metadata.WrappedPolicyKey {
	lastIdx := len(policy.data.WrappedPolicyKeys) - 1
	toRemove := policy.data.WrappedPolicyKeys[index]

	// See https://github.com/golang/go/wiki/SliceTricks
	policy.data.WrappedPolicyKeys[index] = policy.data.WrappedPolicyKeys[lastIdx]
	policy.data.WrappedPolicyKeys[lastIdx] = nil
	policy.data.WrappedPolicyKeys = policy.data.WrappedPolicyKeys[:lastIdx]

	return toRemove
}
