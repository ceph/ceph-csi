/*


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

// RBDRestoreSpec defines the desired state of RBDRestore
type RBDRestoreSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Pool is the name for rbd pool
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Pool string `json:"pool"`

	// ImageName is the name for rbd image to restore
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ImageName string `json:"imagename"`

	// it can be ip:port in case of restore from remote or volumeName in case of local restore
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern="^([0-9]+.[0-9]+.[0-9]+.[0-9]+:[0-9]+)$"
	RestoreSrc string `json:"restoreSrc"`

	// Recreate indicates whether recreate rbd is needed
	Recreate bool `json:"recreate,omitempty"`

	// Size indicates rbd recreate size
	Size int64 `json:"size,omitempty"`
}

// RBDRestoreStatus defines the observed state of RBDRestore
type RBDRestoreStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase RBDRestoreStatusPhase `json:"phase,omitempty"`
}

// RBDRestoreStatus is to hold result of action.
type RBDRestoreStatusPhase string

// Status written onto CStrorRestore object.
const (
	// RSTRBDStatusDone , restore operation is completed.
	RSTRBDStatusDone RBDRestoreStatusPhase = "Done"

	// RSTRBDStatusFailed , restore operation is failed.
	RSTRBDStatusFailed RBDRestoreStatusPhase = "Failed"

	// RSTRBDStatusInit , restore operation is initialized.
	RSTRBDStatusInit RBDRestoreStatusPhase = "Init"

	// RSTRBDStatusPending , restore operation is pending.
	RSTRBDStatusPending RBDRestoreStatusPhase = "Pending"

	// RSTRBDStatusInProgress , restore operation is in progress.
	RSTRBDStatusInProgress RBDRestoreStatusPhase = "InProgress"

	// RSTRBDStatusInvalid , restore operation is invalid.
	RSTRBDStatusInvalid RBDRestoreStatusPhase = "Invalid"
)

// +genclient
// +kubebuilder:object:root=true
// RBDRestore is the Schema for the rbdrestores API
type RBDRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RBDRestoreSpec   `json:"spec,omitempty"`
	Status RBDRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RBDRestoreList contains a list of RBDRestore
type RBDRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RBDRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RBDRestore{}, &RBDRestoreList{})
}
