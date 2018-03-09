/*
Copyright 2018 The Kubernetes Authors.

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

package cephfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/pborman/uuid"
)

type volumeIdentifier struct {
	name, uuid, id string
}

func newVolumeIdentifier(volOptions *volumeOptions, req *csi.CreateVolumeRequest) *volumeIdentifier {
	volId := volumeIdentifier{
		name: req.GetName(),
		uuid: uuid.NewUUID().String(),
	}

	volId.id = "csi-cephfs-" + volId.uuid

	if volId.name == "" {
		volId.name = volOptions.Pool + "-dynamic-pvc-" + volId.uuid
	}

	return &volId
}
