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

package util

import (
	"strings"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// GetCrushLocationMap returns the crush location map, determined from
// the crush location labels and their values from the CO system.
// Expects crushLocationLabels in arg to be in the format "[prefix/]<name>,[prefix/]<name>,...",.
// Returns map of crush location types with its array of associated values.
func GetCrushLocationMap(crushLocationLabels, nodeName string) (map[string]string, error) {
	if crushLocationLabels == "" {
		return nil, nil
	}

	nodeLabels, err := k8sGetNodeLabels(nodeName)
	if err != nil {
		return nil, err
	}

	return getCrushLocationMap(crushLocationLabels, nodeLabels), nil
}

// getCrushLocationMap returns the crush location map, determined from
// the crush location labels and node labels.
func getCrushLocationMap(crushLocationLabels string, nodeLabels map[string]string) map[string]string {
	labelsToRead := strings.Split(crushLocationLabels, labelSeparator)
	log.DefaultLog("CRUSH location labels passed for processing: %+v", labelsToRead)

	labelsIn := make(map[string]bool, len(labelsToRead))
	for _, label := range labelsToRead {
		labelsIn[label] = true
	}

	// Determine values for requested labels from node labels
	crushLocationMap := make(map[string]string, len(labelsIn))
	for key, value := range nodeLabels {
		if _, ok := labelsIn[key]; !ok {
			continue
		}
		// label found split name component and store value
		nameIdx := strings.IndexRune(key, keySeparator)
		crushLocationType := strings.TrimSpace(key[nameIdx+1:])
		if crushLocationType == "hostname" {
			// ceph defaults to "host" while Kubernetes uses "hostname" as key.
			crushLocationType = "host"
		}
		// replace "." with "-" to satisfy ceph crush map.
		value = strings.ReplaceAll(strings.TrimSpace(value), ".", "-")
		crushLocationMap[crushLocationType] = value
	}

	if len(crushLocationMap) == 0 {
		return nil
	}
	log.DefaultLog("list of CRUSH location processed: %+v", crushLocationMap)

	return crushLocationMap
}
