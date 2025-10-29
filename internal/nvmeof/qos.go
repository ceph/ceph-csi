/*
Copyright 2025 The Ceph-CSI Authors.

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
)

// NVMeoFQosVolume holds the QoS parameters for an NVMe-oF volume.
type NVMeoFQosVolume struct {
	RwIosPerSecond    *uint64 // R/W IOs per second limit, 0 means unlimited.
	RwMbytesPerSecond *uint64 // R/W megabytes per second limit, 0 means unlimited.
	RMbytesPerSecond  *uint64 // Read megabytes per second limit, 0 means unlimited.
	WMbytesPerSecond  *uint64 // Write megabytes per second limit, 0 means unlimited.
}

// QoS parameter keys that user can set.
// these parameters are used to configure the QoS limits for NVMe-oF volumes.
// They correspond to the fields in NVMeoFQosVolume struct.
const (
	RwIosPerSecond    = "rwIosPerSecond"
	RwMbytesPerSecond = "rwMbytesPerSecond"
	RMbytesPerSecond  = "rMbytesPerSecond"
	WMbytesPerSecond  = "wMbytesPerSecond"
)

// String returns a string representation of the NVMeoFQosVolume.
func (q *NVMeoFQosVolume) String() string {
	if q == nil {
		return "nil"
	}

	parts := []string{}
	if q.RwIosPerSecond != nil {
		parts = append(parts, fmt.Sprintf("RwIops=%d", *q.RwIosPerSecond))
	}
	if q.RwMbytesPerSecond != nil {
		parts = append(parts, fmt.Sprintf("RwMB/s=%d", *q.RwMbytesPerSecond))
	}
	if q.RMbytesPerSecond != nil {
		parts = append(parts, fmt.Sprintf("RMB/s=%d", *q.RMbytesPerSecond))
	}
	if q.WMbytesPerSecond != nil {
		parts = append(parts, fmt.Sprintf("WMB/s=%d", *q.WMbytesPerSecond))
	}

	if len(parts) == 0 {
		return "no QoS limits"
	}

	return strings.Join(parts, ", ")
}
