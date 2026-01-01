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

	"github.com/ceph/ceph-csi/internal/nvmeof"
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
