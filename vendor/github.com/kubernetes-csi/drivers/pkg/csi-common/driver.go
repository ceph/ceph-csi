/*
Copyright 2017 The Kubernetes Authors.

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

package csicommon

import (
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

type CSIDriver struct {
	name    string
	nodeID  string
	version *csi.Version
	supVers []*csi.Version
	cap     []*csi.ControllerServiceCapability
	vc      []*csi.VolumeCapability_AccessMode
}

// Creates a NewCSIDriver object. Assumes vendor version is equal to driver version &
// does not support optional driver plugin info manifest field. Refer to CSI spec for more details.
func NewCSIDriver(name string, v *csi.Version, supVers []*csi.Version, nodeID string) *CSIDriver {
	if name == "" {
		glog.Errorf("Driver name missing")
		return nil
	}

	if nodeID == "" {
		glog.Errorf("NodeID missing")
		return nil
	}

	if v == nil {
		glog.Errorf("Version argument missing")
		return nil
	}

	found := false
	for _, sv := range supVers {
		if sv.GetMajor() == v.GetMajor() && sv.GetMinor() == v.GetMinor() && sv.GetPatch() == v.GetPatch() {
			found = true
		}
	}

	if !found {
		supVers = append(supVers, v)
	}

	driver := CSIDriver{
		name:    name,
		version: v,
		supVers: supVers,
		nodeID:  nodeID,
	}

	return &driver
}

func (d *CSIDriver) CheckVersion(v *csi.Version) error {
	if v == nil {
		return status.Error(codes.InvalidArgument, "Version missing")
	}

	// Assumes always backward compatible
	for _, sv := range d.supVers {
		if v.Major == sv.Major && v.Minor <= sv.Minor {
			return nil
		}
	}

	return status.Error(codes.InvalidArgument, "Unsupported version: "+GetVersionString(v))
}

func (d *CSIDriver) ValidateControllerServiceRequest(v *csi.Version, c csi.ControllerServiceCapability_RPC_Type) error {
	if v == nil {
		return status.Error(codes.InvalidArgument, "Version not specified")
	}

	if err := d.CheckVersion(v); err != nil {
		return status.Error(codes.InvalidArgument, "Unsupported version")
	}

	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range d.cap {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}

	return status.Error(codes.InvalidArgument, "Unsupported version: "+GetVersionString(v))
}

func (d *CSIDriver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability

	for _, c := range cl {
		glog.Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}

	d.cap = csc

	return
}

func (d *CSIDriver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		glog.Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, NewVolumeCapabilityAccessMode(c))
	}
	d.vc = vca
	return vca
}

func (d *CSIDriver) GetVolumeCapabilityAccessModes() []*csi.VolumeCapability_AccessMode {
	return d.vc
}
