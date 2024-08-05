/*
Copyright 2024 The Kubernetes-CSI-Addons Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolumeGroupReplicationClassSpec specifies parameters that an underlying storage system uses
// when creating a volumegroup replica. A specific VolumeGroupReplicationClass is used by specifying
// its name in a VolumeGroupReplication object.
// +kubebuilder:validation:XValidation:rule="has(self.parameters) == has(oldSelf.parameters)",message="parameters are immutable"
type VolumeGroupReplicationClassSpec struct {
	// Provisioner is the name of storage provisioner
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="provisioner is immutable"
	Provisioner string `json:"provisioner"`
	// Parameters is a key-value map with storage provisioner specific configurations for
	// creating volume group replicas
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="parameters are immutable"
	Parameters map[string]string `json:"parameters,omitempty"`
}

// VolumeGroupReplicationClassStatus defines the observed state of VolumeGroupReplicationClass
type VolumeGroupReplicationClassStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// VolumeGroupReplicationClass is the Schema for the volumegroupreplicationclasses API
type VolumeGroupReplicationClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeGroupReplicationClassSpec   `json:"spec,omitempty"`
	Status VolumeGroupReplicationClassStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VolumeGroupReplicationClassList contains a list of VolumeGroupReplicationClass
type VolumeGroupReplicationClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeGroupReplicationClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VolumeGroupReplicationClass{}, &VolumeGroupReplicationClassList{})
}
