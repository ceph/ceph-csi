/*
Copyright 2022 The Ceph-CSI Authors.
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

package networkfence

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/csi-addons/spec/lib/go/fence"
)

const (
	blocklistTime     = "157784760"
	invalidCommandStr = "invalid command"
)

// NetworkFence contains the CIDR blocks to be blocked.
type NetworkFence struct {
	Cidr     []string
	Monitors string
	cr       *util.Credentials
}

// NewNetworkFence returns a networkFence struct object from the Network fence/unfence request.
func NewNetworkFence(
	ctx context.Context,
	cr *util.Credentials,
	cidrs []*fence.CIDR,
	fenceOptions map[string]string,
) (*NetworkFence, error) {
	var err error
	nwFence := &NetworkFence{}

	nwFence.Cidr, err = GetCIDR(cidrs)
	if err != nil {
		return nil, fmt.Errorf("failed to get list of CIDRs: %w", err)
	}

	clusterID, err := util.GetClusterID(fenceOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch clusterID: %w", err)
	}

	nwFence.Monitors, _, err = util.GetMonsAndClusterID(ctx, clusterID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitors for clusterID %q: %w", clusterID, err)
	}

	nwFence.cr = cr

	return nwFence, nil
}

// addCephBlocklist adds an IP to ceph osd blocklist.
func (nf *NetworkFence) addCephBlocklist(ctx context.Context, ip string, useRange bool) error {
	arg := []string{
		"--id", nf.cr.ID,
		"--keyfile=" + nf.cr.KeyFile,
		"-m", nf.Monitors,
	}
	// TODO: add blocklist till infinity.
	// Currently, ceph does not provide the functionality to blocklist IPs
	// for infinite time. As a workaround, add a blocklist for 5 YEARS to
	// represent infinity from ceph-csi side.
	// At any point in this time, the IPs can be unblocked by an UnfenceClusterReq.
	// This needs to be updated once ceph provides functionality for the same.
	cmd := []string{"osd", "blocklist"}
	if useRange {
		cmd = append(cmd, "range")
	}
	cmd = append(cmd, "add", ip, blocklistTime)
	cmd = append(cmd, arg...)
	_, stdErr, err := util.ExecCommand(ctx, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to blocklist IP %q: %w stderr: %q", ip, err, stdErr)
	}
	log.DebugLog(ctx, "blocklisted IP %q successfully", ip)

	return nil
}

// AddNetworkFence blocks access for all the IPs in the IP range mentioned via the CIDR block
// using a network fence.
func (nf *NetworkFence) AddNetworkFence(ctx context.Context) error {
	hasBlocklistRangeSupport := true
	// for each CIDR block, convert it into a range of IPs so as to perform blocklisting operation.
	for _, cidr := range nf.Cidr {
		// try range blocklist cmd, if invalid fallback to
		// iterating through IP range.
		if hasBlocklistRangeSupport {
			err := nf.addCephBlocklist(ctx, cidr, true)
			if err == nil {
				continue
			}
			if !strings.Contains(err.Error(), invalidCommandStr) {
				return fmt.Errorf("failed to add blocklist range %q: %w", cidr, err)
			}
			hasBlocklistRangeSupport = false
		}
		// fetch the list of IPs from a CIDR block
		hosts, err := getIPRange(cidr)
		if err != nil {
			return fmt.Errorf("failed to convert CIDR block %s to corresponding IP range: %w", cidr, err)
		}

		// add ceph blocklist for each IP in the range mentioned by the CIDR
		for _, host := range hosts {
			err = nf.addCephBlocklist(ctx, host, false)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// getIPRange returns a list of IPs from the IP range
// corresponding to a CIDR block.
func getIPRange(cidr string) ([]string, error) {
	var hosts []string
	netIP, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	for ip := netIP.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		hosts = append(hosts, ip.String())
	}

	return hosts, nil
}

// incIP is an helper function for getIPRange() for incrementing
// IP values to return all IPs in a range.
func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// Cidrs is a list of CIDR structs.
type Cidrs []*fence.CIDR

// GetCIDR converts a CIDR struct list to a list.
func GetCIDR(cidrs Cidrs) ([]string, error) {
	var cidrList []string
	for _, cidr := range cidrs {
		cidrList = append(cidrList, cidr.Cidr)
	}
	if len(cidrList) < 1 {
		return nil, errors.New("the CIDR cannot be empty")
	}

	return cidrList, nil
}

// removeCephBlocklist removes an IP from ceph osd blocklist.
func (nf *NetworkFence) removeCephBlocklist(ctx context.Context, ip string, useRange bool) error {
	arg := []string{
		"--id", nf.cr.ID,
		"--keyfile=" + nf.cr.KeyFile,
		"-m", nf.Monitors,
	}
	cmd := []string{"osd", "blocklist"}
	if useRange {
		cmd = append(cmd, "range")
	}
	cmd = append(cmd, "rm", ip)
	cmd = append(cmd, arg...)

	_, stdErr, err := util.ExecCommand(ctx, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to unblock IP %q: %v %w", ip, stdErr, err)
	}
	log.DebugLog(ctx, "unblocked IP %q successfully", ip)

	return nil
}

// RemoveNetworkFence unblocks access for all the IPs in the IP range mentioned via the CIDR block
// using a network fence.
func (nf *NetworkFence) RemoveNetworkFence(ctx context.Context) error {
	hasBlocklistRangeSupport := true
	// for each CIDR block, convert it into a range of IPs so as to undo blocklisting operation.
	for _, cidr := range nf.Cidr {
		// try range blocklist cmd, if invalid fallback to
		// iterating through IP range.
		if hasBlocklistRangeSupport {
			err := nf.removeCephBlocklist(ctx, cidr, true)
			if err == nil {
				continue
			}
			if !strings.Contains(err.Error(), invalidCommandStr) {
				return fmt.Errorf("failed to remove blocklist range %q: %w", cidr, err)
			}
			hasBlocklistRangeSupport = false
		}
		// fetch the list of IPs from a CIDR block
		hosts, err := getIPRange(cidr)
		if err != nil {
			return fmt.Errorf("failed to convert CIDR block %s to corresponding IP range", cidr)
		}
		// remove ceph blocklist for each IP in the range mentioned by the CIDR
		for _, host := range hosts {
			err := nf.removeCephBlocklist(ctx, host, false)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
