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
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	pb "github.com/ceph/ceph-nvmeof/lib/go/nvmeof"
	"github.com/google/uuid"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// LogNetworkDiagnostics logs the host firewall/network state that is relevant
// when the kernel nvme-tcp reconnect fails (-ETIMEDOUT). It is best effort:
// every failure is only logged. gatewayAddr is optional; when set, the route
// used to reach it is logged as well.
func LogNetworkDiagnostics(ctx context.Context, gatewayAddr string) {
	cmds := [][]string{
		{"iptables", "--version"},
		{"firewall-cmd", "--state"},
		{"nft", "--version"},
		{"ip", "-br", "addr"},
	}
	if gatewayAddr != "" {
		cmds = append(cmds, []string{"ip", "route", "get", gatewayAddr})
	}

	for _, c := range cmds {
		stdout, stderr, err := util.ExecCommand(context.TODO(), c[0], c[1:]...)
		log.DebugLog(ctx, "network diagnostics %v: stdout=%q stderr=%q err=%v",
			c, strings.TrimSpace(stdout), strings.TrimSpace(stderr), err)
	}

	// The nvme-tcp reconnect runs in a kernel worker (init netns), the
	// initial connect runs in the netns of this process. Different inodes
	// mean the gateway address must be reachable from the host netns.
	self, errSelf := os.Readlink("/proc/self/ns/net")
	initNS, errInit := os.Readlink("/proc/1/ns/net")
	log.DebugLog(ctx, "network diagnostics netns: self=%q (err=%v) pid1=%q (err=%v)",
		self, errSelf, initNS, errInit)
}

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
