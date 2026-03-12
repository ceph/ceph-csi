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

// NVMeoFVolumeData holds the data required for an NVMe-oF volume.
type NVMeoFVolumeData struct {
	SubsystemNQN          string
	NamespaceID           uint32
	NamespaceUUID         string
	ListenerInfo          []ListenerDetails
	GatewayManagementInfo GatewayConfig
	Security              NVMeoFSecurityConfig
}

type NVMeoFSecurityConfig struct {
	DhchapMode          string
	AuthenticationKMSID string
}

// SetListenersWithDefaults applies default values to the existing ListenerInfo.
// If port is 0, it defaults to 4420.
// If address is empty, it defaults to 0.0.0.0.
func (v *NVMeoFVolumeData) SetListenersWithDefaults() {
	for i := range v.ListenerInfo {
		if v.ListenerInfo[i].Port == 0 {
			v.ListenerInfo[i].Port = 4420
		}
		if v.ListenerInfo[i].Address == "" {
			v.ListenerInfo[i].Address = "0.0.0.0"
		}
	}
}
