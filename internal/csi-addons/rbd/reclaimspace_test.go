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
	"testing"

	"github.com/stretchr/testify/assert"

	rs "github.com/csi-addons/spec/lib/go/reclaimspace"
)

// TestControllerReclaimSpace is a minimal test for the
// ControllerReclaimSpace() procedure. During unit-testing, there is no Ceph
// cluster available, so actual operations can not be performed.
func TestControllerReclaimSpace(t *testing.T) {
	t.Parallel()

	controller := NewReclaimSpaceControllerServer()

	req := &rs.ControllerReclaimSpaceRequest{
		VolumeId: "",
		Secrets:  nil,
	}

	_, err := controller.ControllerReclaimSpace(context.TODO(), req)
	assert.Error(t, err)
}

// TestNodeReclaimSpace is a minimal test for the NodeReclaimSpace() procedure.
// During unit-testing, there is no Ceph cluster available, so actual
// operations can not be performed.
func TestNodeReclaimSpace(t *testing.T) {
	t.Parallel()

	node := NewReclaimSpaceNodeServer()

	req := &rs.NodeReclaimSpaceRequest{
		VolumeId:         "",
		VolumePath:       "",
		VolumeCapability: nil,
		Secrets:          nil,
	}

	_, err := node.NodeReclaimSpace(context.TODO(), req)
	assert.Error(t, err)
}
