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

package core

import (
	"testing"

	fsAdmin "github.com/ceph/go-ceph/cephfs/admin"
	"github.com/stretchr/testify/require"
)

func TestCreateVolume_NamespaceIsolated_True(t *testing.T) {
	t.Parallel()
	sv := &SubVolume{NamespaceIsolated: true}
	opts := buildSubVolumeCreateOpts(sv)
	require.True(t, opts.NamespaceIsolated)
}

func TestCreateVolume_NamespaceIsolated_False(t *testing.T) {
	t.Parallel()
	sv := &SubVolume{}
	opts := buildSubVolumeCreateOpts(sv)
	require.False(t, opts.NamespaceIsolated)
}

func TestGetSubVolumeInfo_PoolNamespace_Populated(t *testing.T) {
	t.Parallel()
	const ns = "csi-vol-4c742621-090a-484d-a33b"
	info := &fsAdmin.SubVolumeInfo{
		PoolNamespace: ns,
		BytesQuota:    fsAdmin.Infinite,
	}
	sv, err := subvolumeFromInfo(info, "test-vol")
	require.NoError(t, err)
	require.Equal(t, ns, sv.PoolNamespace)
}

func TestGetSubVolumeInfo_PoolNamespace_Empty(t *testing.T) {
	t.Parallel()
	info := &fsAdmin.SubVolumeInfo{
		BytesQuota: fsAdmin.Infinite,
	}
	sv, err := subvolumeFromInfo(info, "test-vol")
	require.NoError(t, err)
	require.Empty(t, sv.PoolNamespace)
}
