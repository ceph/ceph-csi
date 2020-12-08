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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
)

const (
	// defaultCsiSubvolumeGroup defines the default name for the CephFS CSI subvolumegroup.
	// This was hardcoded once and defaults to the old value to keep backward compatibility.
	defaultCsiSubvolumeGroup = "csi"

	// CsiConfigFile is the location of the CSI config file
	CsiConfigFile = "/etc/ceph-csi-config/config.json"
)

// ClusterInfo strongly typed JSON spec for the below JSON structure.
type ClusterInfo struct {
	// ClusterID is used for unique identification
	ClusterID string `json:"clusterID"`
	// RadosNamespace is a rados namespace in the pool
	RadosNamespace string `json:"radosNamespace"`
	// Monitors is monitor list for corresponding cluster ID
	Monitors []string `json:"monitors"`
	// CephFS contains CephFS specific options
	CephFS struct {
		// SubvolumeGroup contains the name of the SubvolumeGroup for CSI volumes
		SubvolumeGroup string `json:"subvolumeGroup"`
	} `json:"cephFS"`
}

// Expected JSON structure in the passed in config file is,
// [
// 	{
// 		"clusterID": "<cluster-id>",
//		"radosNamespace": "<rados-namespace>",
// 		"monitors":
// 			[
// 				"<monitor-value>",
// 				"<monitor-value>",
// 				...
// 			],
//         "cephFS": {
//           "subvolumeGroup": "<subvolumegroup for cephfs volumes>"
//         }
// 	},
// 	...
// ].
func readClusterInfo(pathToConfig, clusterID string) (*ClusterInfo, error) {
	var config []ClusterInfo

	// #nosec
	content, err := ioutil.ReadFile(pathToConfig)
	if err != nil {
		err = fmt.Errorf("error fetching configuration for cluster ID %q: %w", clusterID, err)
		return nil, err
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed (%w), raw buffer response: %s",
			err, string(content))
	}

	for _, cluster := range config {
		if cluster.ClusterID == clusterID {
			return &cluster, nil
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

// RadosNamespace returns the namespace for the given clusterID.
func RadosNamespace(pathToConfig, clusterID string) (string, error) {
	cluster, err := readClusterInfo(pathToConfig, clusterID)
	if err != nil {
		return "", err
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
func GetMonsAndClusterID(options map[string]string) (string, string, error) {
	clusterID, ok := options["clusterID"]
	if !ok {
		return "", "", errors.New("clusterID must be set")
	}

	monitors, err := Mons(CsiConfigFile, clusterID)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch monitor list using clusterID (%s): %w", clusterID, err)
	}

	return monitors, clusterID, nil
}
