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
	"fmt"
	"strings"
)

// ConstructReadAffinityMapOption constructs a read affinity map option based on the provided crushLocationMap.
// It appends crush location labels in the format
// "read_from_replica=localize,crush_location=label1:value1|label2:value2|...".
func ConstructReadAffinityMapOption(crushLocationMap map[string]string) string {
	if len(crushLocationMap) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("read_from_replica=localize,crush_location=")
	first := true
	for key, val := range crushLocationMap {
		if first {
			b.WriteString(fmt.Sprintf("%s:%s", key, val))
			first = false
		} else {
			b.WriteString(fmt.Sprintf("|%s:%s", key, val))
		}
	}

	return b.String()
}

// GetReadAffinityMapOptions retrieves the readAffinityMapOptions from the CSI config file if it exists.
// If not, it falls back to returning the `cliReadAffinityMapOptions` from the command line.
// If neither of these options is available, it returns an empty string.
func GetReadAffinityMapOptions(
	csiConfigFile, clusterID, cliReadAffinityMapOptions string,
	nodeLabels map[string]string,
) (string, error) {
	var (
		err                       error
		configReadAffinityEnabled bool
		configCrushLocationLabels string
	)

	configReadAffinityEnabled, configCrushLocationLabels, err = GetCrushLocationLabels(csiConfigFile, clusterID)
	if err != nil {
		return "", err
	}

	if !configReadAffinityEnabled {
		return "", nil
	}

	if configCrushLocationLabels == "" {
		return cliReadAffinityMapOptions, nil
	}

	crushLocationMap := GetCrushLocationMap(configCrushLocationLabels, nodeLabels)
	readAffinityMapOptions := ConstructReadAffinityMapOption(crushLocationMap)

	return readAffinityMapOptions, nil
}
