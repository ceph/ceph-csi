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

package rbd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/csi-addons/spec/lib/go/fence"
)

// TestFenceClusterNetwork is a minimal test for the FenceClusterNetwork()
// procedure. During unit-testing, there is no Ceph cluster available, so
// actual operations can not be performed.
func TestFenceClusterNetwork(t *testing.T) {
	t.Parallel()

	controller := NewFenceControllerServer()

	req := &fence.FenceClusterNetworkRequest{
		Parameters: map[string]string{},
		Secrets:    nil,
		Cidrs:      nil,
	}

	_, err := controller.FenceClusterNetwork(context.TODO(), req)
	assert.Error(t, err)
}

// TestUnfenceClusterNetwork is a minimal test for the UnfenceClusterNetwork()
// procedure. During unit-testing, there is no Ceph cluster available, so actual
// operations can not be performed.
func TestUnfenceClusterNetwork(t *testing.T) {
	t.Parallel()
	controller := NewFenceControllerServer()

	req := &fence.UnfenceClusterNetworkRequest{
		Parameters: map[string]string{},
		Secrets:    nil,
		Cidrs:      nil,
	}
	_, err := controller.UnfenceClusterNetwork(context.TODO(), req)
	assert.Error(t, err)
}
