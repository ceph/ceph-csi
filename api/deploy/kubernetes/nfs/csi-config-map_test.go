/*
Copyright 2022 The Ceph-CSI Authors.

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

package nfs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCSIConfigMap(t *testing.T) {
	cm, err := NewCSIConfigMap(CSIConfigMapDefaults)

	require.NoError(t, err)
	require.NotNil(t, cm)
	require.Equal(t, cm.Name, CSIConfigMapDefaults.Name)
}

func TestNewCSIConfigMapYAML(t *testing.T) {
	yaml, err := NewCSIConfigMapYAML(CSIConfigMapDefaults)

	require.NoError(t, err)
	require.NotEqual(t, "", yaml)
}
