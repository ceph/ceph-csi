/*
Copyright 2026 The Ceph-CSI Authors.

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
	"maps"
	"strconv"
	"strings"
)

// FeatureGate identifies a toggleable feature.
type FeatureGate string

const (
	// SlowGRPCRestart restarts the process when a unary gRPC call is stuck
	// for more than 10 minutes. Enabled by default.
	SlowGRPCRestart FeatureGate = "SlowGRPCRestart"
)

var defaultFeatureGates = map[FeatureGate]bool{
	SlowGRPCRestart: true,
}

var activeFeatureGates map[FeatureGate]bool

// InitFeatureGates parses a comma-separated "Key=bool" string and initializes
// the active feature gates. Unknown keys are rejected with an error.
func InitFeatureGates(raw string) error {
	activeFeatureGates = make(map[FeatureGate]bool, len(defaultFeatureGates))
	maps.Copy(activeFeatureGates, defaultFeatureGates)

	if raw == "" {
		return nil
	}

	for entry := range strings.SplitSeq(raw, ",") {
		k, v, ok := strings.Cut(entry, "=")
		if !ok || k == "" {
			return fmt.Errorf("invalid feature gate entry %q, expected Key=bool", entry)
		}

		gate := FeatureGate(k)
		if _, known := defaultFeatureGates[gate]; !known {
			return fmt.Errorf("unknown feature gate %q", k)
		}

		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid value for feature gate %q: %w", k, err)
		}

		activeFeatureGates[gate] = enabled
	}

	return nil
}

// IsFeatureGateEnabled returns whether the given feature gate is enabled.
func IsFeatureGateEnabled(gate FeatureGate) bool {
	if v, ok := activeFeatureGates[gate]; ok {
		return v
	}

	return defaultFeatureGates[gate]
}
