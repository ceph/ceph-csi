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
	"fmt"
	"io/ioutil"
	"strings"
)

/*
Mons returns a comma separated MON list from the csi config for the given clusterID
Expected JSON structure in the passed in config file is,
[
	{
		"clusterID": "<cluster-id>",
		"monitors":
			[
				"<monitor-value>",
				"<monitor-value>",
				...
			]
	},
	...
]
*/

// clusterInfo strongly typed JSON spec for the above JSON structure
type clusterInfo struct {
	ClusterID string   `json:"clusterID"`
	Monitors  []string `json:"monitors"`
}

func Mons(pathToConfig, clusterID string) (string, error) {
	var config []clusterInfo

	// #nosec
	content, err := ioutil.ReadFile(pathToConfig)
	if err != nil {
		err = fmt.Errorf("error fetching configuration for cluster ID (%s). (%s)", clusterID, err)
		return "", err
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		return "", fmt.Errorf("unmarshal failed: %v. raw buffer response: %s",
			err, string(content))
	}

	for _, cluster := range config {
		if cluster.ClusterID == clusterID {
			if len(cluster.Monitors) == 0 {
				return "", fmt.Errorf("empty monitor list for cluster ID (%s) in config", clusterID)
			}
			return strings.Join(cluster.Monitors, ","), nil
		}
	}
	return "", fmt.Errorf("missing configuration for cluster ID (%s)", clusterID)
}
