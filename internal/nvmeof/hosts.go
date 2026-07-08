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
package nvmeof

import (
	"fmt"
	"strings"

	"github.com/ghodss/yaml"
)

// NVMeoFHostList holds a list of host NQNs that are allowed to access a volume.
type NVMeoFHostList struct {
	HostNQNs []string
}

// AllowHostNQNs is the VolumeAttributesClass mutable parameter key for specifying
// a YAML list of host NQNs to allow access to a volume. Use "*" to allow any host.
// Example:
//
//	allowHostNQNs: |
//	  - nqn.2014-08.org.nvmexpress:host1
//	  - nqn.2014-08.org.nvmexpress:host2
const AllowHostNQNs = "allowHostNQNs"

// NewNVMeoFHostListFromParams parses the hosts YAML list parameter and validates its contents.
// Returns:
//   - nil if the key is absent (caller should not modify existing hosts)
//   - *NVMeoFHostList with empty HostNQNs slice if key is present but empty (caller should remove all hosts)
//   - *NVMeoFHostList with populated HostNQNs slice if hosts are specified
func NewNVMeoFHostListFromParams(params map[string]string) (*NVMeoFHostList, error) {
	allowHostNQNs, exists := params[AllowHostNQNs]
	if !exists {
		return nil, nil // Key absent: don't modify existing hosts
	}
	if allowHostNQNs == "" {
		return &NVMeoFHostList{HostNQNs: []string{}}, nil // Key present but empty: remove all hosts
	}

	var allowHostsList []string
	if err := yaml.Unmarshal([]byte(allowHostNQNs), &allowHostsList); err != nil {
		return nil, fmt.Errorf("invalid %s: must be a YAML list of strings: %w", AllowHostNQNs, err)
	}

	return &NVMeoFHostList{HostNQNs: allowHostsList}, nil
}

// String returns a string representation of the host list.
func (h *NVMeoFHostList) String() string {
	if h == nil {
		return "nil"
	}
	if len(h.HostNQNs) == 0 {
		return "no hosts"
	}
	if len(h.HostNQNs) == 1 && h.HostNQNs[0] == "*" {
		return "all hosts (*)"
	}

	return fmt.Sprintf("%d host(s): %s", len(h.HostNQNs), strings.Join(h.HostNQNs, ", "))
}
