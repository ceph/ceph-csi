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

	pb "github.com/ceph/ceph-nvmeof/lib/go/nvmeof"
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
// It returns the first resolved IP address matching the requested address family.
func ResolveIPAddress(host string, family pb.AddressFamily) (string, error) {
	return resolveIPAddress(host, family, net.LookupHost)
}

func resolveIPAddress(host string, family pb.AddressFamily, lookupHost func(string) ([]string, error)) (string, error) {
	if family != pb.AddressFamily_ipv4 && family != pb.AddressFamily_ipv6 {
		return "", fmt.Errorf("unsupported address family %s", family)
	}
	addrs, err := lookupHost(host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no IP addresses for %s", host)
	}

	for _, address := range addrs {
		ip := net.ParseIP(address)
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		if (family == pb.AddressFamily_ipv4) == (ip.To4() != nil) {
			return address, nil
		}
	}

	return "", fmt.Errorf("no %s addresses for %s", family, host)
}
