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

type ClusterInfo struct {
	// ClusterID is used for unique identification
	ClusterID string
	// Monitors is monitor list for corresponding cluster ID
	Monitors []string
	// CephFS contains CephFS specific options
	CephFS CephFS
	// RBD Contains RBD specific options
	RBD RBD
	// NFS contains NFS specific options
	NFS NFS
	// Read affinity map options
	ReadAffinity ReadAffinity
}

type CephFS struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string
	// SubvolumeGroup contains the name of the SubvolumeGroup for CSI volumes
	SubvolumeGroup string
	// KernelMountOptions contains the kernel mount options for CephFS volumes
	KernelMountOptions string
	// FuseMountOptions contains the fuse mount options for CephFS volumes
	FuseMountOptions string
}
type RBD struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string
	// RadosNamespace is a rados namespace in the pool
	RadosNamespace string
}

type NFS struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string
}

type ReadAffinity struct {
	Enabled             bool
	CrushLocationLabels []string
}
