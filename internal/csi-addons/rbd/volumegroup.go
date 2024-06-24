/*
Copyright 2024 The Ceph-CSI Authors.

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
	corerbd "github.com/ceph/ceph-csi/internal/rbd"

	"github.com/csi-addons/spec/lib/go/volumegroup"
	"google.golang.org/grpc"
)

// VolumeGroupServer struct of rbd CSI driver with supported methods of VolumeGroup
// controller server spec.
type VolumeGroupServer struct {
	// added UnimplementedControllerServer as a member of
	// ControllerServer. if volumegroup spec add more RPC services in the proto
	// file, then we don't need to add all RPC methods leading to forward
	// compatibility.
	*volumegroup.UnimplementedControllerServer
	// Embed ControllerServer as it implements helper functions
	*corerbd.ControllerServer
}

// NewVolumeGroupServer creates a new VolumeGroupServer which handles
// the VolumeGroup Service requests from the CSI-Addons specification.
func NewVolumeGroupServer(c *corerbd.ControllerServer) *VolumeGroupServer {
	return &VolumeGroupServer{ControllerServer: c}
}

func (vs *VolumeGroupServer) RegisterService(server grpc.ServiceRegistrar) {
	volumegroup.RegisterControllerServer(server, vs)
}
