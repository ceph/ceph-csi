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

package ocp

import (
	"strings"
	"testing"

	secv1 "github.com/openshift/api/security/v1"
	"github.com/stretchr/testify/require"
)

func TestNewSecurityContextConstraints(t *testing.T) {
	t.Parallel()

	t.Run("SecurityContextConstraintsDefaults", func(t *testing.T) {
		var (
			err error
			scc *secv1.SecurityContextConstraints
		)

		getSCC := func() {
			scc, err = NewSecurityContextConstraints(SecurityContextConstraintsDefaults)
		}

		require.NotPanics(t, getSCC)
		require.Nil(t, err)
		require.NotNil(t, scc)

		require.Equal(t, scc.Name, "ceph-csi")
		for _, user := range scc.Users {
			require.True(t, strings.HasPrefix(user, "system:serviceaccount:ceph-csi:csi"))
		}
	})

	t.Run("DeployerRook", func(t *testing.T) {
		var (
			err error
			scc *secv1.SecurityContextConstraints
		)

		rookValues := SecurityContextConstraintsValues{
			Namespace: "rook-ceph",
			Deployer:  "rook",
		}

		getSCC := func() {
			scc, err = NewSecurityContextConstraints(rookValues)
		}

		require.NotPanics(t, getSCC)
		require.Nil(t, err)
		require.NotNil(t, scc)

		require.Equal(t, scc.Name, "rook-ceph-csi")
		for _, user := range scc.Users {
			require.True(t, strings.HasPrefix(user, "system:serviceaccount:rook-ceph:rook-csi"))
		}
	})
}

func TestNewSecurityContextConstraintsYAML(t *testing.T) {
	t.Parallel()

	var (
		err  error
		yaml string
	)

	getYAML := func() {
		yaml, err = NewSecurityContextConstraintsYAML(SecurityContextConstraintsDefaults)
	}

	require.NotPanics(t, getYAML)
	require.Nil(t, err)
	require.NotEqual(t, "", yaml)
}
