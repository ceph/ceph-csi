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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolumeGroupReplicationContentSpec defines the desired state of VolumeGroupReplicationContent
type VolumeGroupReplicationContentSpec struct {
	// VolumeGroupreplicationRef specifies the VolumeGroupReplication object to which this
	// VolumeGroupReplicationContent object is bound.
	// VolumeGroupReplication.Spec.VolumeGroupReplicationContentName field must reference to
	// this VolumeGroupReplicationContent's name for the bidirectional binding to be valid.
	// For a pre-existing VolumeGroupReplicationContent object, name and namespace of the
	// VolumeGroupReplication object MUST be provided for binding to happen.
	// This field is immutable after creation.
	// Required.
	// +kubebuilder:validation:XValidation:rule="has(self.name) && has(self.__namespace__)",message="both volumeGroupReplicationRef.name and volumeGroupReplicationRef.namespace must be set"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="volumeGroupReplicationRef is immutable"
	VolumeGroupReplicationRef corev1.ObjectReference `json:"volumeGroupReplicationRef"`

	// VolumeGroupReplicationHandle is a unique id returned by the CSI driver
	// to identify the VolumeGroupReplication on the storage system.
	VolumeGroupReplicationHandle string `json:"volumeGroupReplicationHandle"`

	// provisioner is the name of the CSI driver used to create the physical
	// volume group on
	// the underlying storage system.
	// This MUST be the same as the name returned by the CSI GetPluginName() call for
	// that driver.
	// Required.
	Provisioner string `json:"provisioner"`

	// VolumeGroupReplicationClassName is the name of the VolumeGroupReplicationClass from
	// which this group replication was (or will be) created.
	// +optional
	VolumeGroupReplicationClassName string `json:"volumeGroupReplicationClassName"`

	// Source specifies whether the snapshot is (or should be) dynamically provisioned
	// or already exists, and just requires a Kubernetes object representation.
	// This field is immutable after creation.
	// Required.
	Source VolumeGroupReplicationContentSource `json:"source"`
}

// VolumeGroupReplicationContentSource represents the CSI source of a group replication.
type VolumeGroupReplicationContentSource struct {
	// VolumeHandles is a list of volume handles on the backend to be grouped
	// and replicated.
	VolumeHandles []string `json:"volumeHandles"`
}

// VolumeGroupReplicationContentStatus defines the status of VolumeGroupReplicationContent
type VolumeGroupReplicationContentStatus struct {
	// PersistentVolumeRefList is the list of of PV for the group replication
	// The maximum number of allowed PV in the group is 100.
	// +optional
	PersistentVolumeRefList []corev1.LocalObjectReference `json:"persistentVolumeRefList,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// VolumeGroupReplicationContent is the Schema for the volumegroupreplicationcontents API
type VolumeGroupReplicationContent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeGroupReplicationContentSpec   `json:"spec,omitempty"`
	Status VolumeGroupReplicationContentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VolumeGroupReplicationContentList contains a list of VolumeGroupReplicationContent
type VolumeGroupReplicationContentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeGroupReplicationContent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VolumeGroupReplicationContent{}, &VolumeGroupReplicationContentList{})
}
