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

package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/nvmeof"
	rbdutil "github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util/log"
)

func (cs *Server) getOrInitSecurityKeys(
	ctx context.Context,
	kmsID string,
	credentials map[string]string,
) (nvmeof.SecurityKeyManager, error) {
	if cs.securityKeys != nil {
		return cs.securityKeys, nil
	}

	var err error
	securityKeys, err := nvmeof.InitSecurityKeyManager(ctx, kmsID, credentials)

	// Only cache if it doesn't need external DEKStore (like Vault)
	// (RBD Metadata KMS needs fresh RBD volume each call)
	if !errors.Is(err, nvmeof.ErrDEKStoreNeeded) {
		cs.securityKeys = securityKeys
	}

	return securityKeys, err
}

// setupDHCHAPKeys configures DH-CHAP authentication and returns the host key.
// Returns empty string if DH-CHAP is disabled.
func (cs *Server) setupDHCHAPKeys(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
	nodeID,
	subsystemNQN,
	hostNQN,
	dhchapMode string,
) (nvmeof.DHCHAPKeys, error) {
	dhchapKeys := nvmeof.DHCHAPKeys{}
	// Return empty key if DH-CHAP is disabled
	if dhchapMode == nvmeof.DHCHAPEmpty || dhchapMode == nvmeof.DHCHAPModeNone {
		return dhchapKeys, nil
	}

	log.DebugLog(ctx, "DH-CHAP mode: %s - setting up authentication for node %s", dhchapMode, nodeID)
	// Get authentication KMS ID from volume context
	volumeContext := req.GetVolumeContext()
	authKMSID := volumeContext[vcAuthenticationKMSID]
	secrets := req.GetSecrets()

	// Initialize security key manager
	securityKeysManager, err := cs.getOrInitSecurityKeys(ctx, authKMSID, secrets)

	// Setup DEK store if needed (for metadata KMS)
	// (just for test, not for production!! for production, use external KMS like Vault)
	if errors.Is(err, nvmeof.ErrDEKStoreNeeded) {
		volumeID := req.GetVolumeId()
		mgr := rbdutil.NewManager(cs.backendServer.Driver.GetInstanceID(), nil, secrets)
		defer mgr.Destroy(ctx)

		rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
		if err != nil {
			return dhchapKeys, fmt.Errorf("failed to find volume with ID %q: %w", volumeID, err)
		}
		defer rbdVol.Destroy(ctx)

		dekStore := nvmeof.NewRBDVolumeDEKStore(rbdVol)
		securityKeysManager.SetDEKStore(dekStore)
	} else if err != nil {
		return dhchapKeys, fmt.Errorf("failed to initialize security key manager: %w", err)
	}

	// Get or create DH-CHAP host key
	hostDhchapKey, err := nvmeof.GetOrCreateDHCHAPHostKey(ctx, securityKeysManager, nodeID, subsystemNQN, hostNQN)
	if err != nil {
		return dhchapKeys, fmt.Errorf("failed to get/create DH-CHAP host key: %w", err)
	}

	log.DebugLog(ctx, "DH-CHAP host key retrieved for node %s, subsystem %s", nodeID, subsystemNQN)

	// If bidirectional, get or create subsystem key
	if dhchapMode == nvmeof.DHCHAPModeBiDirectional {
		subsystemKey, err := nvmeof.GetOrCreateDHCHAPSubsystemKey(ctx, securityKeysManager, nodeID, subsystemNQN, hostNQN)
		if err != nil {
			return dhchapKeys, fmt.Errorf("failed to get/create DH-CHAP subsystem key: %w", err)
		}
		log.DebugLog(ctx, "DH-CHAP subsystem key retrieved for node %s, subsystem %s", nodeID, subsystemNQN)
		dhchapKeys = nvmeof.DHCHAPKeys{
			SubsystemKey: subsystemKey,
		}
	}
	dhchapKeys.HostKey = hostDhchapKey

	return dhchapKeys, nil
}

// cleanupDHCHAPKeys removes DH-CHAP keys for the unpublished host.
func (cs *Server) cleanupDHCHAPKeys(
	ctx context.Context,
	secrets map[string]string,
	nodeID,
	volumeID,
	subsystemNQN,
	dhchapMode,
	authKMSID string,
) error {
	// Return empty key if DH-CHAP is disabled
	if dhchapMode == nvmeof.DHCHAPEmpty || dhchapMode == nvmeof.DHCHAPModeNone {
		return nil
	}
	log.DebugLog(ctx, "Cleaning up DH-CHAP keys for node %s, subsystem %s", nodeID, subsystemNQN)

	log.DebugLog(ctx, "DH-CHAP mode: %s - setting up authentication for node %s", dhchapMode, nodeID)
	// Get authentication KMS ID from volume context

	// Initialize security key manager
	securityKeysManager, err := cs.getOrInitSecurityKeys(ctx, authKMSID, secrets)

	// Setup DEK store if needed (for metadata KMS)
	// (just for test, not for production!! for production, use external KMS like Vault)
	if errors.Is(err, nvmeof.ErrDEKStoreNeeded) {
		mgr := rbdutil.NewManager(cs.backendServer.Driver.GetInstanceID(), nil, secrets)
		defer mgr.Destroy(ctx)

		rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
		if err != nil {
			return fmt.Errorf("failed to find volume with ID %q: %w", volumeID, err)
		}
		defer rbdVol.Destroy(ctx)

		dekStore := nvmeof.NewRBDVolumeDEKStore(rbdVol)
		securityKeysManager.SetDEKStore(dekStore)
	} else if err != nil {
		return fmt.Errorf("failed to initialize security key manager: %w", err)
	}

	// Remove host key
	if err := nvmeof.RemoveDHCHAPHostKey(ctx, securityKeysManager, nodeID, subsystemNQN); err != nil {
		log.ErrorLog(ctx, "Failed to remove DH-CHAP host key: %v", err)
		// Don't return error - continue to try subsystem key
	} else {
		log.DebugLog(ctx, "DH-CHAP host key removed for node %s", nodeID)
	}

	// Remove subsystem key if bidirectional
	if dhchapMode == nvmeof.DHCHAPModeBiDirectional {
		if err := nvmeof.RemoveDHCHAPSubsystemKey(ctx, securityKeysManager, nodeID, subsystemNQN); err != nil {
			log.ErrorLog(ctx, "Failed to remove DH-CHAP subsystem key: %v", err)
		} else {
			log.DebugLog(ctx, "DH-CHAP subsystem key removed for node %s", nodeID)
		}
	}

	return nil
}
