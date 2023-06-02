/*
Copyright 2019 The Ceph-CSI Authors.

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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	// defaultCsiSubvolumeGroup defines the default name for the CephFS CSI subvolumegroup.
	// This was hardcoded once and defaults to the old value to keep backward compatibility.
	defaultCsiSubvolumeGroup = "csi"

	// CsiConfigFile is the location of the CSI config file.
	CsiConfigFile = "/etc/ceph-csi-config/config.json"

	// ClusterIDKey is the name of the key containing clusterID.
	ClusterIDKey = "clusterID"
)

// ClusterInfo strongly typed JSON spec for the below JSON structure.
type ClusterInfo struct {
	// ClusterID is used for unique identification
	ClusterID string `json:"clusterID"`
	// RadosNamespace is a rados namespace in the pool
	RadosNamespace string `json:"radosNamespace"` // For backward compatibility. TODO: Remove this in 3.7.0
	// Monitors is monitor list for corresponding cluster ID
	Monitors []string `json:"monitors"`
	// CephFS contains CephFS specific options
	CephFS struct {
		// symlink filepath for the network namespace where we need to execute commands.
		NetNamespaceFilePath string `json:"netNamespaceFilePath"`
		// SubvolumeGroup contains the name of the SubvolumeGroup for CSI volumes
		SubvolumeGroup string `json:"subvolumeGroup"`
	} `json:"cephFS"`

	// RBD Contains RBD specific options
	RBD struct {
		// symlink filepath for the network namespace where we need to execute commands.
		NetNamespaceFilePath string `json:"netNamespaceFilePath"`
		// RadosNamespace is a rados namespace in the pool
		RadosNamespace string `json:"radosNamespace"`
	} `json:"rbd"`
	// NFS contains NFS specific options
	NFS struct {
		// symlink filepath for the network namespace where we need to execute commands.
		NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	} `json:"nfs"`
}

// Expected JSON structure in the passed in config file is,
//nolint:godot // example json content should not contain unwanted dot.
/*
[{
	"clusterID": "<cluster-id>",
	"rbd": {
		"radosNamespace": "<rados-namespace>"
	},
	"monitors": [
		"<monitor-value>",
		"<monitor-value>"
	],
	"cephFS": {
		"subvolumeGroup": "<subvolumegroup for cephfs volumes>"
	}
}]
*/
func readClusterInfo(pathToConfig, clusterID string) (*ClusterInfo, error) {
	var config []ClusterInfo

	// #nosec
	content, err := os.ReadFile(pathToConfig)
	if err != nil {
		err = fmt.Errorf("error fetching configuration for cluster ID %q: %w", clusterID, err)

		return nil, err
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed (%w), raw buffer response: %s",
			err, string(content))
	}

	for i := range config {
		if config[i].ClusterID == clusterID {
			return &config[i], nil
		}
	}

	return nil, fmt.Errorf("missing configuration for cluster ID %q", clusterID)
}

// Mons returns a comma separated MON list from the csi config for the given clusterID.
func Mons(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
	}

	if len(cluster.Monitors) == 0 {
		return "", fmt.Errorf("empty monitor list for cluster ID (%s) in config", clusterID)
	}

	return strings.Join(cluster.Monitors, ","), nil
}

// GetRadosNamespace returns the namespace for the given clusterID.
func GetRadosNamespace(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
	}

	if cluster.RBD.RadosNamespace != "" {
		return cluster.RBD.RadosNamespace, nil
	}

	return cluster.RadosNamespace, nil
}

// CephFSSubvolumeGroup returns the subvolumeGroup for CephFS volumes. If not set, it returns the default value "csi".
func CephFSSubvolumeGroup(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
	}

	if cluster.CephFS.SubvolumeGroup == "" {
		return defaultCsiSubvolumeGroup, nil
	}

	return cluster.CephFS.SubvolumeGroup, nil
}

// GetMonsAndClusterID returns monitors and clusterID information read from
// configfile.
func GetMonsAndClusterID(ctx context.Context, clusterID string, checkClusterIDMapping bool) (string, string, error) {
	if checkClusterIDMapping {
		monitors, mappedClusterID, err := FetchMappedClusterIDAndMons(ctx, clusterID)
		if err != nil {
			return "", "", err
		}

		return monitors, mappedClusterID, nil
	}

	monitors, err := Mons(CsiConfigFile, clusterID)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch monitor list using clusterID (%s): %w", clusterID, err)
	}

	return monitors, clusterID, nil
}

// GetClusterID fetches clusterID from given options map.
func GetClusterID(options map[string]string) (string, error) {
	clusterID, ok := options[ClusterIDKey]
	if !ok {
		return "", ErrClusterIDNotSet
	}

	return clusterID, nil
}

func GetRBDNetNamespaceFilePath(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
	}

	return cluster.RBD.NetNamespaceFilePath, nil
}

// GetCephFSNetNamespaceFilePath returns the netNamespaceFilePath for CephFS volumes.
func GetCephFSNetNamespaceFilePath(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
	}

	return cluster.CephFS.NetNamespaceFilePath, nil
}

// GetNFSNetNamespaceFilePath returns the netNamespaceFilePath for NFS volumes.
func GetNFSNetNamespaceFilePath(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
	}

	return cluster.NFS.NetNamespaceFilePath, nil
}
