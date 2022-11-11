/*
Copyright 2018 The Ceph-CSI Authors.

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

package util

// VolumeID string representation.
type VolumeID string

const (
	// VolIDVersion is the version number of volume ID encoding scheme.
	VolIDVersion uint16 = 1

	// RadosNamespace to store CSI specific objects and keys.
	RadosNamespace = "csi"
)
