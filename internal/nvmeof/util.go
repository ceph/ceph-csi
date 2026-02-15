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
	"net"
	"strings"

	"github.com/google/uuid"
)

// formatUUID is a helper to format UUID with dashes.
// Any dashes are removed from the passed rawUUID, and a UUID with dashes in
// standard positions is returned.
// When the rawUUID can not be parsed into a UUID, it will be returned as-is,
// with the assumption that the caller knows what it is doing.
func formatUUID(rawUUID string) string {
	// Remove any existing dashes
	clean := strings.ReplaceAll(rawUUID, "-", "")

	newUUID, err := uuid.Parse(clean)
	if err != nil {
		// rawUUID is not in a standard format, return as is.
		return rawUUID
	}

	return newUUID.String()
}

// ResolveIPAddress resolves the given host to an IP address.
// It returns the first resolved IP address as a string, or an error if resolution fails.
func ResolveIPAddress(host string) (string, error) {
	// TODO - IPv6 support: we currently return the first resolved address,
	// which may be an IPv4 or IPv6 address. We should consider how to handle this
	// in a way that supports both protocols.
	addrs, err := net.LookupHost(host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no IP addresses for %s", host)
	}

	return addrs[0], nil
}
