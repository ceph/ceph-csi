/*
Copyright 2022 The Kubernetes-CSI-Addons Authors.

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

const (
	VolumeReplicationNameAnnotation = "replication.storage.openshift.io/volume-replication-name"
)

// ReplicationState represents the replication operations to be performed on the volume.
// +kubebuilder:validation:Enum=primary;secondary;resync
type ReplicationState string

const (
	// Primary ReplicationState enables mirroring and promotes the volume to primary.
	Primary ReplicationState = "primary"

	// Secondary ReplicationState demotes the volume to secondary and resyncs the volume if out of sync.
	Secondary ReplicationState = "secondary"

	// Resync option resyncs the volume.
	Resync ReplicationState = "resync"
)

// State captures the latest state of the replication operation.
type State string

const (
	// PrimaryState represents the Primary replication state.
	PrimaryState State = "Primary"

	// SecondaryState represents the Secondary replication state.
	SecondaryState State = "Secondary"

	// UnknownState represents the Unknown replication state.
	UnknownState State = "Unknown"
)

// VolumeReplicationSpec defines the desired state of VolumeReplication.
type VolumeReplicationSpec struct {
	// VolumeReplicationClass is the VolumeReplicationClass name for this VolumeReplication resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="volumeReplicationClass is immutable"
	VolumeReplicationClass string `json:"volumeReplicationClass"`

	// ReplicationState represents the replication operation to be performed on the volume.
	// Supported operations are "primary", "secondary" and "resync"
	// +kubebuilder:validation:Required
	ReplicationState ReplicationState `json:"replicationState"`

	// DataSource represents the object associated with the volume
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dataSource is immutable"
	DataSource corev1.TypedLocalObjectReference `json:"dataSource"`

	// AutoResync represents the volume to be auto resynced when
	// ReplicationState is "secondary"
	// +kubebuilder:default:=false
	AutoResync bool `json:"autoResync"`

	// replicationHandle represents an existing (but new) replication id
	// +kubebuilder:validation:Optional
	ReplicationHandle string `json:"replicationHandle"`
}

// VolumeReplicationStatus defines the observed state of VolumeReplication.
type VolumeReplicationStatus struct {
	State   State  `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
	// Conditions are the list of conditions and their status.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// observedGeneration is the last generation change the operator has dealt with
	// +optional
	ObservedGeneration int64            `json:"observedGeneration,omitempty"`
	LastStartTime      *metav1.Time     `json:"lastStartTime,omitempty"`
	LastCompletionTime *metav1.Time     `json:"lastCompletionTime,omitempty"`
	LastSyncTime       *metav1.Time     `json:"lastSyncTime,omitempty"`
	LastSyncBytes      *int64           `json:"lastSyncBytes,omitempty"`
	LastSyncDuration   *metav1.Duration `json:"lastSyncDuration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
// +kubebuilder:printcolumn:JSONPath=".spec.volumeReplicationClass",name=volumeReplicationClass,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.dataSource.name",name=pvcName,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.replicationState",name=desiredState,type=string
// +kubebuilder:printcolumn:JSONPath=".status.state",name=currentState,type=string
// +kubebuilder:resource:shortName=vr

// VolumeReplication is the Schema for the volumereplications API.
type VolumeReplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec VolumeReplicationSpec `json:"spec"`

	Status VolumeReplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VolumeReplicationList contains a list of VolumeReplication.
type VolumeReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeReplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VolumeReplication{}, &VolumeReplicationList{})
}
