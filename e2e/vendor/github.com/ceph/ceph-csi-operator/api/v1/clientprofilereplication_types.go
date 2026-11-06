/*
Copyright 2026.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClientProfileReplicationSpec defines the desired replication destination mapping
type ClientProfileReplicationSpec struct {
	// LocalClientProfile is the name of the local ClientProfile CR
	// +kubebuilder:validation:Required
	LocalClientProfile string `json:"localClientProfile"`

	// RemoteClientProfile is the name of the remote cluster's client profile
	// +kubebuilder:validation:Required
	RemoteClientProfile string `json:"remoteClientProfile"`

	// RBD contains RBD-specific replication configuration
	// +optional
	RBD *RBDReplicationSpec `json:"rbd,omitempty"`
}

// RBDReplicationSpec defines RBD-specific replication configuration
type RBDReplicationSpec struct {
	// PoolMapping maps local pool names to remote pool IDs
	// +optional
	PoolMapping []PoolMappingSpec `json:"poolMapping,omitempty"`
}

// PoolMappingSpec defines the mapping for a single pool
type PoolMappingSpec struct {
	// Name is the pool name (must be consistent across clusters)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// RemoteID is the pool ID on the remote cluster
	// +kubebuilder:validation:Required
	RemoteID string `json:"remoteID"`
}

// ClientProfileReplicationStatus defines the observed state of ClientProfileReplication.
type ClientProfileReplicationStatus struct {
	// Phase indicates the current state of this CR
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides human-readable details about the current phase
	// +optional
	Message string `json:"message,omitempty"`
}

const (
	// ClientProfileReplicationPhaseReady indicates the CR is accepted and active
	ClientProfileReplicationPhaseReady = "Ready"

	// ClientProfileReplicationPhaseRejected indicates the CR is rejected (conflict)
	ClientProfileReplicationPhaseRejected = "Rejected"

	// ClientProfileReplicationPhasePending indicates validation is in progress
	ClientProfileReplicationPhasePending = "Pending"
)

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// ClientProfileReplication is the Schema for the clientprofilereplications API
type ClientProfileReplication struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ClientProfileReplication
	// +required
	Spec ClientProfileReplicationSpec `json:"spec"`

	// status defines the observed state of ClientProfileReplication
	// +optional
	Status ClientProfileReplicationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ClientProfileReplicationList contains a list of ClientProfileReplication
type ClientProfileReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ClientProfileReplication `json:"items"`
}
