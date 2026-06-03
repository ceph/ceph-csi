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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/ceph/ceph-csi/internal/util/log"
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
	content, err := os.ReadFile(filename) // #nosec:G304, file inclusion via variable.
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

// GetMappedID check the input id is matching key or value.
// If key==id the value will be returned.
// If value==id the key will be returned.
func GetMappedID(key, value, id string) string {
	if key == id {
		return value
	}
	if value == id {
		return key
	}

	return ""
}

// fetchMappedClusterIDAndMons returns monitors and clusterID info after checking cluster mapping.
func fetchMappedClusterIDAndMons(ctx context.Context,
	clusterID, clusterMappingConfigFile, csiConfigFile string,
) (string, string, error) {
	var mons string
	clusterMappingInfo, err := getClusterMappingInfo(clusterID, clusterMappingConfigFile)
	if err != nil {
		return "", "", err
	}

	if clusterMappingInfo != nil {
		for _, cm := range *clusterMappingInfo {
			for key, val := range cm.ClusterIDMapping {
				mappedClusterID := GetMappedID(key, val, clusterID)
				if mappedClusterID == "" {
					continue
				}
				log.DebugLog(ctx,
					"found new clusterID mapping %q for existing clusterID %q",
					mappedClusterID,
					clusterID)

				mons, err = Mons(csiConfigFile, mappedClusterID)
				if err != nil {
					log.DebugLog(ctx, "failed getting mons with mapped cluster id %q: %v",
						mappedClusterID, err)

					continue
				}

				return mons, mappedClusterID, nil
			}
		}
	}

	// check original clusterID for backward compatibility when cluster ids were expected to be same.
	mons, err = Mons(csiConfigFile, clusterID)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons with cluster id %q: %v", clusterID, err)

		return "", "", err
	}

	return mons, clusterID, err
}

// FetchMappedClusterIDAndMons returns monitors and clusterID info after checking cluster mapping.
func FetchMappedClusterIDAndMons(ctx context.Context, clusterID string) (string, string, error) {
	return fetchMappedClusterIDAndMons(ctx, clusterID, clusterMappingConfigFile, CsiConfigFile)
}
