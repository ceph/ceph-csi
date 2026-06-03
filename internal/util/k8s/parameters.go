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

	// PV and PVC metadata keys used by external provisioner as part of
	// create requests as parameters, when `extra-create-metadata` is true.
	pvcNameKey      = csiParameterPrefix + "pvc/name"
	pvcNamespaceKey = csiParameterPrefix + "pvc/namespace"
	pvNameKey       = csiParameterPrefix + "pv/name"

	// snapshot metadata keys.
	volSnapNameKey        = csiParameterPrefix + "volumesnapshot/name"
	volSnapNamespaceKey   = csiParameterPrefix + "volumesnapshot/namespace"
	volSnapContentNameKey = csiParameterPrefix + "volumesnapshotcontent/name"
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

// GetVolumeMetadata filter parameters, only return PV/PVC/PVCNamespace metadata.
func GetVolumeMetadata(parameters map[string]string) map[string]string {
	keys := []string{pvcNameKey, pvcNamespaceKey, pvNameKey}
	newParam := map[string]string{}
	for k, v := range parameters {
		for _, key := range keys {
			if strings.Contains(k, key) {
				newParam[k] = v
			}
		}
	}

	return newParam
}

// GetVolumeMetadataKeys return volume metadata keys.
func GetVolumeMetadataKeys() []string {
	return []string{
		pvcNameKey,
		pvcNamespaceKey,
		pvNameKey,
	}
}

// PrepareVolumeMetadata return PV/PVC/PVCNamespace metadata based on inputs.
func PrepareVolumeMetadata(pvcName, pvcNamespace, pvName string) map[string]string {
	newParam := map[string]string{}
	if pvcName != "" {
		newParam[pvcNameKey] = pvcName
	}
	if pvcNamespace != "" {
		newParam[pvcNamespaceKey] = pvcNamespace
	}
	if pvName != "" {
		newParam[pvNameKey] = pvName
	}

	return newParam
}

// GetSnapshotMetadata filter parameters, only return
// snapshot-name/snapshot-namespace/snapshotcontent-name metadata.
func GetSnapshotMetadata(parameters map[string]string) map[string]string {
	keys := []string{volSnapNameKey, volSnapNamespaceKey, volSnapContentNameKey}
	newParam := map[string]string{}
	for k, v := range parameters {
		for _, key := range keys {
			if strings.Contains(k, key) {
				newParam[k] = v
			}
		}
	}

	return newParam
}

// GetSnapshotMetadataKeys return snapshot metadata keys.
func GetSnapshotMetadataKeys() []string {
	return []string{
		volSnapNameKey,
		volSnapNamespaceKey,
		volSnapContentNameKey,
	}
}
