# Encryption Key Rotation

## Proposal

Subject of this proposal is to add support for rotation of
encryption keys (KEKs) for encrypted volumes in Ceph-CSI.

Support for rotating keys on RWX/ROX volumes and filesystem encryption
with `fscrypt` is out of scope for now and shall be added later.

## Document Terminology

- Encryption Key: The passphrase that is used to encrypt and open the device.
- LUKS: The specification used by dm-crypt to process encrypted volumes on linux.

## Proposed Solution

The proposed solution in this document, is to address the rotation
of encryption keys for encrypted volumes.

This document outlines the rotation steps for PVCs backed by RBD.

### Implementation Summary

This feature builds upon the foundation laid by encrypted pvcs.

The following new methods are added to `cryptsetup.go` for
handling the key rotation.

- `LuksAddKey`: Adds a new key to specified LUKS slot
- `LuksRemoveKey`: Removes the specified key from its slot using `luksKillSlot`
- `LuksVerifyKey`: Verifies that the given key exists
  in the given slot using `luksChangeKey`.

### Implementation Details

The encryption key rotation request will contain with it
the volume ID and secrets.

The secrets are used to generate the credentials for authenticating
against a ceph cluster.

These values are then used to call `GenVolFromVolID` to get the
rbdVolume structure.

The `VolumeEncryption` struct is modified to make
`generateNewEncryptionPassphrase` a public member function.

The `EncryptionKeyRotation` service is registered and implemented
on the node-plugin.

The following steps are followed to process the device for key rotation:

- Create a `rbdvolume` object using volume ID,
  this is done by `GenVolFromVolID`.
- Fetch the current key from the KMS, it is needed for
  subsequent LUKS operations.
- Get the device path for the volume by calling `waitForPath` as all LUKS
  operations require the device path.
- Add the fetched key to LUKS slot 1, this will serve as a backup of the key.
- Generate a new key and store it locally. It will be updated
  in the KMS at later steps.
- Remove the existing key from slot 0 upon verifying that the
  key in KMS == the key in slot 0.
- Add new key to slot 0.
- Update the new key in the KMS.
- Fetch the key again and verify that the
  key in KMS == the new key we generated.
- We can now remove the backup key from slot 1.

Note that the key in the KMS can always be used to unlock the volume.
