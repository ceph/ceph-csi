/*
Copyright 2021 The Ceph-CSI Authors.

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
	"os"
)

// clusterMappingConfigFile is the location of the cluster mapping config file.
var clusterMappingConfigFile = "/etc/ceph-csi-config/cluster-mapping.json"

// ClusterMappingInfo holds the details of clusterID mapping and poolID mapping.
type ClusterMappingInfo struct {
	// ClusterIDMapping holds the details of clusterID mapping
	ClusterIDMapping map[string]string `json:"clusterIDMapping"`
	// rbdpoolIDMappingInfo holds the details of RBD poolID mapping.
	RBDpoolIDMappingInfo []map[string]string `json:"RBDPoolIDMapping"`
	// cephFSpoolIDMappingInfo holds the details of CephFS Fscid mapping.
	CephFSFscIDMappingInfo []map[string]string `json:"CephFSFscIDMapping"`
}

// Expected JSON structure in the passed in config file is,
// [{
// 	"clusterIDMapping": {
// 		"site1-storage": "site2-storage"
// 	},
// 	"RBDPoolIDMapping": [{
// 		"1": "2",
// 		"11": "12"
// 	}],
// 	"CephFSFscIDMapping": [{
// 		"13": "34",
// 		"3": "4"
// 	}]
// }, {
// 	"clusterIDMapping": {
// 		"site3-storage": "site2-storage"
// 	},
// 	"RBDPoolIDMapping": [{
// 		"5": "2",
// 		"16": "12"
// 	}],
// 	"CephFSFscIDMapping": [{
// 		"3": "34",
// 		"4": "4"
// 	}]
// ...
// }]

func readClusterMappingInfo(filename string) (*[]ClusterMappingInfo, error) {
	var info []ClusterMappingInfo
	content, err := ioutil.ReadFile(filename) // #nosec:G304, file inclusion via variable.
	if err != nil {
		err = fmt.Errorf("error fetching clusterID mapping %w", err)

		return nil, err
	}

	err = json.Unmarshal(content, &info)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed (%w), raw buffer response: %s",
			err, string(content))
	}

	return &info, nil
}

// getClusterMappingInfo returns corresponding cluster details like clusterID's
// poolID,fscID lists read from 'filename'.
func getClusterMappingInfo(clusterID, filename string) (*[]ClusterMappingInfo, error) {
	var mappingInfo []ClusterMappingInfo
	info, err := readClusterMappingInfo(filename)
	if err != nil {
		// discard not found error as this file is expected to be created by
		// the admin in case of failover.
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to fetch cluster mapping: %w", err)
	}
	for _, i := range *info {
		for key, val := range i.ClusterIDMapping {
			// Same file will be copied to the failover cluster check for both
			// key and value to check clusterID mapping exists
			if key == clusterID || val == clusterID {
				mappingInfo = append(mappingInfo, i)
			}
		}
	}

	// if the mapping is not found return response as nil
	if len(mappingInfo) == 0 {
		return nil, nil
	}

	return &mappingInfo, nil
}

// GetClusterMappingInfo returns corresponding cluster details like clusterID's
// poolID,fscID lists read from configfile.
func GetClusterMappingInfo(clusterID string) (*[]ClusterMappingInfo, error) {
	return getClusterMappingInfo(clusterID, clusterMappingConfigFile)
}
