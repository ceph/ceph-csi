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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func TestIsThickProvisionRequest(t *testing.T) {
	req := &csi.CreateVolumeRequest{
		Name: "fake",
		Parameters: map[string]string{
			"unkownOption": "not-set",
		},
	}

	// pass disabled/invalid values for "thickProvision" option
	if isThickProvisionRequest(req.GetParameters()) {
		t.Error("request is not for thick-provisioning")
	}

	req.Parameters["thickProvision"] = ""
	if isThickProvisionRequest(req.GetParameters()) {
		t.Errorf("request is not for thick-provisioning: %s", req.Parameters["thickProvision"])
	}

	req.Parameters["thickProvision"] = "false"
	if isThickProvisionRequest(req.GetParameters()) {
		t.Errorf("request is not for thick-provisioning: %s", req.Parameters["thickProvision"])
	}

	req.Parameters["thickProvision"] = "off"
	if isThickProvisionRequest(req.GetParameters()) {
		t.Errorf("request is not for thick-provisioning: %s", req.Parameters["thickProvision"])
	}

	req.Parameters["thickProvision"] = "no"
	if isThickProvisionRequest(req.GetParameters()) {
		t.Errorf("request is not for thick-provisioning: %s", req.Parameters["thickProvision"])
	}

	req.Parameters["thickProvision"] = "**true**"
	if isThickProvisionRequest(req.GetParameters()) {
		t.Errorf("request is not for thick-provisioning: %s", req.Parameters["thickProvision"])
	}

	// only "true" should enable thick provisioning
	req.Parameters["thickProvision"] = "true"
	if !isThickProvisionRequest(req.GetParameters()) {
		t.Errorf("request should be for thick-provisioning: %s", req.Parameters["thickProvision"])
	}
}
