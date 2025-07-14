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
	// For a pre-existing VolumeGroupReplication object, MUST provide an empty/nil value for
	// VolumeGroupReplicationRef for the auto-binding to happen.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self != null ? has(self.name) && has(self.__namespace__) && has(self.uid) : true",message="volumeGroupReplicationRef.name, volumeGroupReplicationRef.namespace and volumeGroupReplicationRef.uid must be set if volumeGroupReplicationRef is defined"
	VolumeGroupReplicationRef *corev1.ObjectReference `json:"volumeGroupReplicationRef,omitempty"`

	// VolumeGroupReplicationHandle is a unique id returned by the CSI driver
	// to identify the VolumeGroupReplication on the storage system.
	// +kubebuilder:validation:Optional
	VolumeGroupReplicationHandle string `json:"volumeGroupReplicationHandle"`

	// provisioner is the name of the CSI driver used to create the physical
	// volume group on
	// the underlying storage system.
	// This MUST be the same as the name returned by the CSI GetPluginName() call for
	// that driver.
	// Required.
	// +kubebuilder:validation:Required
	Provisioner string `json:"provisioner"`

	// VolumeGroupReplicationClassName is the name of the VolumeGroupReplicationClass from
	// which this group replication was (or will be) created.
	// Required.
	// +kubebuilder:validation:Required
	VolumeGroupReplicationClassName string `json:"volumeGroupReplicationClassName"`

	// Source specifies whether the volume group is (or should be) dynamically provisioned
	// or already exists using the volumes listed here, and just requires a
	// Kubernetes object representation.
	// Required.
	// +kubebuilder:validation:Required
	Source VolumeGroupReplicationContentSource `json:"source"`
}

// VolumeGroupReplicationContentSource represents the CSI source of a group replication.
type VolumeGroupReplicationContentSource struct {
	// VolumeHandles is a list of volume handles on the backend to be grouped
	// and replicated.
	// +kubebuilder:validation:Required
	VolumeHandles []string `json:"volumeHandles"`
}

// VolumeGroupReplicationContentStatus defines the status of VolumeGroupReplicationContent
type VolumeGroupReplicationContentStatus struct {
	// PersistentVolumeRefList is the list of PV for the group replication
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
