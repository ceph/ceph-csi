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

// VolumeGroupReplicationSpec defines the desired state of VolumeGroupReplication
type VolumeGroupReplicationSpec struct {
	// volumeGroupReplicationClassName is the volumeGroupReplicationClass name for this VolumeGroupReplication resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="volumeGroupReplicationClassName is immutable"
	VolumeGroupReplicationClassName string `json:"volumeGroupReplicationClassName"`

	// volumeReplicationClassName is the volumeReplicationClass name for the VolumeReplication object
	// created for this volumeGroupReplication
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="volumeReplicationClassName is immutable"
	VolumeReplicationClassName string `json:"volumeReplicationClassName"`

	// Name of the VolumeReplication object created for this volumeGroupReplication
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="volumeReplicationName is immutable"
	VolumeReplicationName string `json:"volumeReplicationName,omitempty"`

	// Name of the VolumeGroupReplicationContent object created for this volumeGroupReplication
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="volumeGroupReplicationContentName is immutable"
	VolumeGroupReplicationContentName string `json:"volumeGroupReplicationContentName,omitempty"`

	// Source specifies where a group replications will be created from.
	// This field is immutable after creation.
	// Required.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="source is immutable"
	Source VolumeGroupReplicationSource `json:"source"`

	// ReplicationState represents the replication operation to be performed on the group.
	// Supported operations are "primary", "secondary" and "resync"
	// +kubebuilder:validation:Required
	ReplicationState ReplicationState `json:"replicationState"`

	// AutoResync represents the group to be auto resynced when
	// ReplicationState is "secondary"
	// +kubebuilder:default:=false
	AutoResync bool `json:"autoResync"`
}

// VolumeGroupReplicationSource specifies the source for the the volumeGroupReplication
type VolumeGroupReplicationSource struct {
	// Selector is a label query over persistent volume claims that are to be
	// grouped together for replication.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="selector is immutable"
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// VolumeGroupReplicationStatus defines the observed state of VolumeGroupReplication
type VolumeGroupReplicationStatus struct {
	VolumeReplicationStatus `json:",inline"`
	// PersistentVolumeClaimsRefList is the list of PVCs for the volume group replication.
	// The maximum number of allowed PVCs in the group is 100.
	// +optional
	PersistentVolumeClaimsRefList []corev1.LocalObjectReference `json:"persistentVolumeClaimsRefList,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VolumeGroupReplication is the Schema for the volumegroupreplications API
type VolumeGroupReplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeGroupReplicationSpec   `json:"spec,omitempty"`
	Status VolumeGroupReplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VolumeGroupReplicationList contains a list of VolumeGroupReplication
type VolumeGroupReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeGroupReplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VolumeGroupReplication{}, &VolumeGroupReplicationList{})
}
