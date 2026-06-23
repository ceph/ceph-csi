/*
Copyright 2023 The Ceph-CSI Authors.

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

package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
)

type ClusterInfo struct {
	// ClusterID is used for unique identification
	ClusterID string `json:"clusterID"`
	// Monitors is monitor list for corresponding cluster ID
	Monitors []string `json:"monitors"`
	// CephFS contains CephFS specific options
	CephFS CephFS `json:"cephFS"`
	// RBD Contains RBD specific options
	RBD RBD `json:"rbd"`
	// NFS contains NFS specific options
	NFS NFS `json:"nfs"`
	// Read affinity map options
	ReadAffinity ReadAffinity `json:"readAffinity"`
	// ReplicationDestination defines the destination cluster for replication.
	// Populated by ceph-csi-operator from ReplicationDestinationConfig CR.
	// +optional
	ReplicationDestination *ReplicationDestinationInfo `json:"replicationDestination,omitempty"`
}

type CephFS struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	// SubvolumeGroup contains the name of the SubvolumeGroup for CSI volumes
	SubvolumeGroup string `json:"subvolumeGroup"`
	// RadosNamespace is a rados namespace in the filesystem metadata pool
	RadosNamespace string `json:"radosNamespace"`
	// KernelMountOptions contains the kernel mount options for CephFS volumes
	KernelMountOptions string `json:"kernelMountOptions"`
	// FuseMountOptions contains the fuse mount options for CephFS volumes
	FuseMountOptions string `json:"fuseMountOptions"`
	// ControllerPublishSecretRef contains the secret reference for controller
	// publish operations.
	ControllerPublishSecretRef corev1.SecretReference `json:"controllerPublishSecretRef"`
}
type RBD struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	// RadosNamespace is a rados namespace in the pool
	RadosNamespace string `json:"radosNamespace"`
	// RBD mirror daemons running in the ceph cluster.
	MirrorDaemonCount int `json:"mirrorDaemonCount"`
	// ControllerPublishSecretRef contains the secret reference for controller
	// publish operations.
	ControllerPublishSecretRef corev1.SecretReference `json:"controllerPublishSecretRef"`
	// NodePublishSecretRef contains the secret reference for node publish
	// operations. Used for retrieving QoS metadata during NodePublishVolume.
	NodePublishSecretRef corev1.SecretReference `json:"nodePublishSecretRef"`
}

type NFS struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
}

type ReadAffinity struct {
	Enabled             bool     `json:"enabled"`
	CrushLocationLabels []string `json:"crushLocationLabels"`
}

// ReplicationDestinationInfo contains destination cluster information for
// replication. It enables the CSI driver to map source volume/group IDs
// to destination volume/group IDs when pool IDs differ across clusters.
type ReplicationDestinationInfo struct {
	// RemoteClusterID is the clusterID of the destination cluster
	RemoteClusterID string `json:"remoteClusterID"`
	// RBD contains RBD-specific replication destination configuration
	// +optional
	RBD *RemoteRBDDetails `json:"rbd,omitempty"`
}

// RemoteRBDDetails contains RBD-specific remote cluster details for replication.
type RemoteRBDDetails struct {
	// RemotePoolMapping maps pool names to remote pool details.
	// Key: pool name (e.g., "rbd", "replicapool")
	// If empty, pool IDs are assumed identical on both clusters.
	// +optional
	RemotePoolMapping map[string]RemotePoolDetails `json:"remotePoolMapping,omitempty"`
}

// RemotePoolDetails contains details of a pool on the remote cluster.
type RemotePoolDetails struct {
	// PoolID is the remote pool ID as a decimal string (e.g., "5")
	PoolID string `json:"poolID"`
}
