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

// RBDBackupSpec defines the desired state of RBDBackup
type RBDBackupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// VolumeName is a name of the volume for which this backup is destined
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	VolumeName string `json:"volumeName"`

	// Pool is the snapshot id for rbd pool
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Pool string `json:"pool"`

	// SnapshotName is the snapshot id for VolumeSnapshotContent
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SnapshotName string `json:"snapshotName"`

	// BackupDest is the remote address for backup transfer
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern="^([0-9]+.[0-9]+.[0-9]+.[0-9]+:[0-9]+)$"
	BackupDest string `json:"backupDest"`
}

// RBDBackupStatusPhase defines the observed state of RBDBackup
type RBDBackupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase RBDBackupStatusPhase `json:"phase,omitempty"`
}

// RBDBackupStatusPhase is to hold status of backup
type RBDBackupStatusPhase string

// Status written onto RBDBackup objects.
const (
	// BKPRBDStatusDone , backup is completed.
	BKPRBDStatusDone RBDBackupStatusPhase = "Done"

	// BKPRBDStatusFailed , backup is failed.
	BKPRBDStatusFailed RBDBackupStatusPhase = "Failed"

	// BKPRBDStatusInit , backup is initialized.
	BKPRBDStatusInit RBDBackupStatusPhase = "Init"

	// BKPRBDStatusPending , backup is pending.
	BKPRBDStatusPending RBDBackupStatusPhase = "Pending"

	// BKPRBDStatusInProgress , backup is in progress.
	BKPRBDStatusInProgress RBDBackupStatusPhase = "InProgress"

	// BKPRBDStatusInvalid , backup operation is invalid.
	BKPRBDStatusInvalid RBDBackupStatusPhase = "Invalid"
)

// +genclient
// +kubebuilder:object:root=true
// RBDBackup is the Schema for the rbdbackups API
type RBDBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RBDBackupSpec   `json:"spec,omitempty"`
	Status RBDBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RBDBackupList contains a list of RBDBackup
type RBDBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RBDBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RBDBackup{}, &RBDBackupList{})
}
