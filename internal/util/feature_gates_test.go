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

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Feature gate tests must not run in parallel: they mutate the package-level
// activeFeatureGates map and running them concurrently causes a data race.

//nolint:paralleltest // mutates package-level activeFeatureGates
func TestInitFeatureGatesEmpty(t *testing.T) {
	require.NoError(t, InitFeatureGates(""))
	require.True(t, IsFeatureGateEnabled(SlowGRPCRestart))
}

//nolint:paralleltest // mutates package-level activeFeatureGates
func TestInitFeatureGatesDisable(t *testing.T) {
	require.NoError(t, InitFeatureGates("SlowGRPCRestart=false"))
	require.False(t, IsFeatureGateEnabled(SlowGRPCRestart))
}

//nolint:paralleltest // mutates package-level activeFeatureGates
func TestInitFeatureGatesEnable(t *testing.T) {
	require.NoError(t, InitFeatureGates("SlowGRPCRestart=true"))
	require.True(t, IsFeatureGateEnabled(SlowGRPCRestart))
}

//nolint:paralleltest // mutates package-level activeFeatureGates
func TestInitFeatureGatesUnknownKey(t *testing.T) {
	err := InitFeatureGates("UnknownGate=true")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown feature gate")
}

//nolint:paralleltest // mutates package-level activeFeatureGates
func TestInitFeatureGatesBadFormat(t *testing.T) {
	require.Error(t, InitFeatureGates("noequals"))
	require.Error(t, InitFeatureGates("SlowGRPCRestart=notabool"))
	require.Error(t, InitFeatureGates("=true"))
}

//nolint:paralleltest // mutates package-level activeFeatureGates
func TestIsFeatureGateEnabledBeforeInit(t *testing.T) {
	orig := activeFeatureGates
	activeFeatureGates = nil
	defer func() { activeFeatureGates = orig }()

	require.True(t, IsFeatureGateEnabled(SlowGRPCRestart))
}
