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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

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

// SetupListeners parses the listeners JSON and validates the required fields.
// It returns a slice of ListenerDetails or an error if the JSON is invalid
// or if required fields are missing.
func SetupListeners(listenersJSON string) ([]ListenerDetails, error) {
	if listenersJSON == "" { // No "listeners" entry was provided
		return []ListenerDetails{}, nil
	}
	var listeners []ListenerDetails
	if err := json.Unmarshal([]byte(listenersJSON), &listeners); err != nil {
		return nil, fmt.Errorf("failed to parse listeners JSON: %w", err)
	}

	if len(listeners) == 0 { // At least one listener is required
		return nil, errors.New("at least one listener must be specified")
	}

	// Validate each listener
	// Listener address can be empty. It will be set later to the default 0.0.0.0.
	// Port can be empty (it will be set later to the default 4420).
	for i, listener := range listeners {
		if listener.Hostname == "" {
			return nil, fmt.Errorf("listener %d: missing required fields (hostname)", i)
		}
	}

	return listeners, nil
}

// SetFromParameters populates the NVMeoFVolumeData fields based on the provided parameters map.
// It extracts the subsystem NQN, gateway management info, security config, and
// listener info from the parameters.
// It also applies default values to listeners if necessary.
func (v *NVMeoFVolumeData) SetFromParameters(parameters map[string]string) error {
	// set subsystem NQN
	v.SubsystemNQN = parameters["subsystemNQN"]

	// set gw management info
	if nvmeofGatewayPortStr := parameters["nvmeofGatewayPort"]; nvmeofGatewayPortStr != "" {
		parsed, err := strconv.ParseUint(nvmeofGatewayPortStr, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid nvmeofGatewayPort %s: %w", nvmeofGatewayPortStr, err)
		}
		v.GatewayManagementInfo.Port = uint32(parsed)
	}
	v.GatewayManagementInfo.Address = parameters["nvmeofGatewayAddress"]

	// set security config
	v.Security.DhchapMode = parameters["dhchapMode"]
	v.Security.AuthenticationKMSID = parameters["authenticationKMSID"]

	// If dhchapMode was explicitly provided and is not "none", and authenticationKMSID is empty,
	// use a default KMS ID - RBD metadata KMS.
	// In production, users should always provide a KMS ID when using DH-CHAP.
	if v.Security.DhchapMode != DHCHAPEmpty &&
		v.Security.DhchapMode != DHCHAPModeNone &&
		v.Security.AuthenticationKMSID == "" {
		v.Security.AuthenticationKMSID = "metadata"
	}

	// set listeners
	listeners, err := SetupListeners(parameters["listeners"])
	if err != nil {
		return fmt.Errorf("failed to set up listeners: %w", err)
	}
	v.ListenerInfo = listeners

	// Apply default values to listeners if necessary
	v.SetListenersWithDefaults()

	return nil
}
