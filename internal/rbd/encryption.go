/*
Copyright 2021 The Ceph-CSI Authors.

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

package rbd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
)

type rbdEncryptionState string

const (
	// Encryption statuses for RbdImage
	rbdImageEncryptionUnknown  = rbdEncryptionState("")
	rbdImageEncrypted          = rbdEncryptionState("encrypted")
	rbdImageRequiresEncryption = rbdEncryptionState("requiresEncryption")

	// image metadata key for encryption
	encryptionMetaKey = ".rbd.csi.ceph.com/encrypted"
)

// checkRbdImageEncrypted verifies if rbd image was encrypted when created.
func (rv *rbdVolume) checkRbdImageEncrypted(ctx context.Context) (rbdEncryptionState, error) {
	value, err := rv.GetMetadata(encryptionMetaKey)
	if err != nil {
		util.ErrorLog(ctx, "checking image %s encrypted state metadata failed: %s", rv, err)
		return rbdImageEncryptionUnknown, err
	}

	encrypted := rbdEncryptionState(strings.TrimSpace(value))
	util.DebugLog(ctx, "image %s encrypted state metadata reports %q", rv, encrypted)
	return encrypted, nil
}

func (rv *rbdVolume) ensureEncryptionMetadataSet(status rbdEncryptionState) error {
	err := rv.SetMetadata(encryptionMetaKey, string(status))
	if err != nil {
		return fmt.Errorf("failed to save encryption status for %s: %w", rv, err)
	}

	return nil
}

// setupEncryption configures the metadata of the RBD image for encryption:
// - the Data-Encryption-Key (DEK) will be generated stored for use by the KMS;
// - the RBD image will be marked to support encryption in its metadata.
func (rv *rbdVolume) setupEncryption(ctx context.Context) error {
	err := util.StoreNewCryptoPassphrase(rv.VolID, rv.KMS)
	if err != nil {
		util.ErrorLog(ctx, "failed to save encryption passphrase for "+
			"image %s: %s", rv.String(), err)
		return err
	}

	err = rv.ensureEncryptionMetadataSet(rbdImageRequiresEncryption)
	if err != nil {
		util.ErrorLog(ctx, "failed to save encryption status, deleting "+
			"image %s: %s", rv.String(), err)
		return err
	}

	return nil
}
