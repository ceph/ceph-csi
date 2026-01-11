/*
Copyright 2026 The Ceph-CSI Authors.

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

package nodeserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/nvmeof"
	rbdutil "github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
)

// getOrInitSecurityKeys initializes or retrieves cached security keys manager.
func (cs *NodeServer) getOrInitSecurityKeys(
	ctx context.Context,
	kmsID string,
	secrets map[string]string,
) (nvmeof.SecurityKeyManager, error) {
	if cs.securityKeys != nil {
		return cs.securityKeys, nil
	}

	var err error
	securityKeys, err := nvmeof.InitSecurityKeyManager(ctx, kmsID, secrets)

	// Only cache if it doesn't need external DEKStore (like Vault)
	// (RBD Metadata KMS needs fresh RBD volume each call)
	if !errors.Is(err, nvmeof.ErrDEKStoreNeeded) {
		cs.securityKeys = securityKeys
	}

	return securityKeys, err
}

// setupDHCHAPAuth configures DH-CHAP authentication for the connection.
func (ns *NodeServer) setupDHCHAPAuth(
	ctx context.Context,
	volumeID string,
	info *NvmeConnectionInfo,
	secrets map[string]string,
	authKMSID string,
	connectReq *nvmeof.ConnectRequest,
) error {
	// Initialize security key manager
	securityKeys, err := ns.getOrInitSecurityKeys(ctx, authKMSID, secrets)

	// Setup DEK store if needed (for metadata KMS)
	if errors.Is(err, nvmeof.ErrDEKStoreNeeded) {
		cr, err := util.NewUserCredentialsWithMigration(secrets)
		if err != nil {
			return fmt.Errorf("failed to get user credentials: %w", err)
		}
		defer cr.DeleteCredentials()

		rbdVol, err := rbdutil.GenVolFromVolID(ctx, volumeID, cr, secrets)
		if err != nil {
			return fmt.Errorf("failed to get volume: %w", err)
		}
		defer rbdVol.Destroy(ctx)

		dekStore := nvmeof.NewRBDVolumeDEKStore(rbdVol)
		securityKeys.SetDEKStore(dekStore)
	} else if err != nil {
		return fmt.Errorf("failed to initialize security key manager: %w", err)
	}

	// Get host key
	hostKey, err := nvmeof.GetOrCreateDHCHAPHostKey(ctx, securityKeys, ns.nodeID, info.SubsystemNQN, connectReq.HostNQN)
	if err != nil {
		return fmt.Errorf("failed to get DHCHAP host key: %w", err)
	}
	connectReq.HostDhchapKey = hostKey

	// Get subsystem key for bidirectional mode
	if info.DhchapMode == nvmeof.DHCHAPModeBiDirectional {
		subsystemKey, err := nvmeof.GetOrCreateDHCHAPSubsystemKey(ctx, securityKeys, ns.nodeID,
			info.SubsystemNQN, connectReq.HostNQN)
		if err != nil {
			return fmt.Errorf("failed to get DH-CHAP subsystem key: %w", err)
		}
		connectReq.SubsystemDhchapKey = subsystemKey
	}

	return nil
}
