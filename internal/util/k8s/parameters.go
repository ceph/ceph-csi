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
package k8s

import (
	"strings"
)

// CSI Parameters prefixed with csiParameterPrefix are passed through
// to the driver on CreateVolumeRequest/CreateSnapshotRequest calls.
const (
	csiParameterPrefix = "csi.storage.k8s.io/"
	pvcNamespaceKey    = "csi.storage.k8s.io/pvc/namespace"
)

// RemoveCSIPrefixedParameters removes parameters prefixed with csiParameterPrefix.
func RemoveCSIPrefixedParameters(param map[string]string) map[string]string {
	newParam := map[string]string{}
	for k, v := range param {
		if !strings.HasPrefix(k, csiParameterPrefix) {
			// add the parameter to the new map if its not having the prefix
			newParam[k] = v
		}
	}

	return newParam
}

// GetOwner returns the pvc namespace name from the parameter.
func GetOwner(param map[string]string) string {
	return param[pvcNamespaceKey]
}
