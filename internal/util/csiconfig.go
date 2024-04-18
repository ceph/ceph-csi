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

	"github.com/ceph/ceph-csi/api/deploy/kubernetes"
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

// Expected JSON structure in the passed in config file is,
//nolint:godot // example json content should not contain unwanted dot.
/*
[{
	"clusterID": "<cluster-id>",
	"rbd": {
		"radosNamespace": "<rados-namespace>"
		"mirrorDaemonCount": 1
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
func readClusterInfo(pathToConfig, clusterID string) (*kubernetes.ClusterInfo, error) {
	var config []kubernetes.ClusterInfo

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

	return cluster.RBD.RadosNamespace, nil
}

// GetRBDMirrorDaemonCount returns the number of mirror daemon count for the
// given clusterID.
func GetRBDMirrorDaemonCount(pathToConfig, clusterID string) (int, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return 0, err
	}

	// if it is empty, set the default to 1 which is most common in a cluster.
	if cluster.RBD.MirrorDaemonCount == 0 {
		return 1, nil
	}

	return cluster.RBD.MirrorDaemonCount, nil
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

// GetCrushLocationLabels returns the `readAffinity.enabled` and `readAffinity.crushLocationLabels`
// values from the CSI config for the given `clusterID`. If `readAffinity.enabled` is set to true
// it returns `true` and `crushLocationLabels`, else returns `false` and an empty string.
func GetCrushLocationLabels(pathToConfig, clusterID string) (bool, string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return false, "", err
	}

	if !cluster.ReadAffinity.Enabled {
		return false, "", nil
	}

	crushLocationLabels := strings.Join(cluster.ReadAffinity.CrushLocationLabels, ",")

	return true, crushLocationLabels, nil
}

// GetCephFSMountOptions returns the `kernelMountOptions` and `fuseMountOptions` for CephFS volumes.
func GetCephFSMountOptions(pathToConfig, clusterID string) (string, string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", "", err
	}

	return cluster.CephFS.KernelMountOptions, cluster.CephFS.FuseMountOptions, nil
}
