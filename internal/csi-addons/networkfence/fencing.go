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

package networkfence

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	osdAdmin "github.com/ceph/go-ceph/common/admin/osd"
	"github.com/csi-addons/spec/lib/go/fence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	ISO8601TimeLayout = "2006-01-02T15:04:05.000000-0700"
	// BlockListCoolDownPeriod defines the time duration
	// after which a blocklist entry can be removed.
	// TODO: Make this configurable.
	blockListCoolDownPeriod = 5 * time.Minute
	invalidCommandStr       = "invalid command"
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

// RemoveNetworkFence unblocks access for all the IPs in the IP range mentioned via the CIDR block
// using a network fence.
// Unfencing one of the protocols(CephFS or RBD) suggests the node is expected to be recovered, so
// both CephFS and RBD are expected to work again too.
// example:
// Create RBD NetworkFence CR for one IP 10.10.10.10
// Created CephFS NetworkFence CR for IP range but above IP comes in the Range
// Delete the CephFS Network Fence CR to unblocklist the IP
// So now the IP (10.10.10.10) is (un)blocklisted and can be used by both protocols.
func (nf *NetworkFence) RemoveNetworkFence(ctx context.Context) error {
	hasBlocklistRangeSupport := true
	// for each CIDR block, convert it into a range of IPs so as to undo blocklisting operation.
	for _, cidr := range nf.Cidr {
		// try range blocklist cmd, if invalid fallback to
		// iterating through IP range.
		if hasBlocklistRangeSupport {
			err := nf.removeCephBlocklist(ctx, cidr, "", true)
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
			// 0 is used as nonce here to tell ceph
			// to remove the blocklist entry matching: <host>:0/0
			// it is same as telling ceph to remove just the IP
			// without specifying any port or nonce with it.
			err := nf.removeCephBlocklist(ctx, host, "0", false)
			if err != nil {
				return err
			}
		}
	}

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

// addCephBlocklist adds an IP to ceph osd blocklist.
func (nf *NetworkFence) addCephBlocklist(ctx context.Context, ip string, useRange bool) error {
	return util.AddCephBlocklist(ctx, nf.Monitors, nf.cr, ip, useRange)
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
		cidrList = append(cidrList, cidr.GetCidr())
	}
	if len(cidrList) < 1 {
		return nil, errors.New("the CIDR cannot be empty")
	}

	return cidrList, nil
}

// removeCephBlocklist removes an IP from ceph osd blocklist.
// the value of nonce is ignored if useRange is true.
func (nf *NetworkFence) removeCephBlocklist(ctx context.Context, ip, nonce string, useRange bool) error {
	return util.RemoveCephBlocklist(ctx, nf.Monitors, nf.cr, ip, nonce, useRange)
}

// GetFenceClients fetches the ceph cluster ID and the client address that need to be fenced
// It also auto-unfences client if necessary conditions are met.
func GetFenceClients(
	ctx context.Context,
	req *fence.GetFenceClientsRequest,
	enableFencing bool,
) (*fence.GetFenceClientsResponse, error) {
	options := req.GetParameters()
	clusterID, err := util.GetClusterID(options)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	cr, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()

	monitors, _ /* clusterID*/, err := util.GetMonsAndClusterID(ctx, clusterID, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get the cluster ID of the ceph cluster.
	conn := &util.ClusterConnection{}
	err = conn.Connect(monitors, cr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to connect to MONs %q: %s", monitors, err)
	}
	defer conn.Destroy()

	fsID, err := conn.GetFSID()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get ceph id: %s", err)
	}

	address, err := conn.GetAddrs()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get client address: %s", err)
	}

	// The example address we get is 10.244.0.1:0/2686266785 from
	// which we need to extract the IP address.
	addr, err := util.ParseClientIP(address)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse client address: %s", err)
	}

	cidr, err := util.ConvertIPToCIDR(addr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert IP to CIDR: %s", err)
	}

	if enableFencing {
		err = autoUnfenceClientOnMatch(ctx, conn, addr)
		if err != nil {
			log.ErrorLog(ctx, "failed to auto unfence client: %s", err)

			return nil, status.Errorf(codes.Internal,
				"failed to unfence client: %s", err)
		}
	}

	resp := &fence.GetFenceClientsResponse{
		Clients: []*fence.ClientDetails{
			{
				Id: fsID,
				Addresses: []*fence.CIDR{
					{
						Cidr: cidr,
					},
				},
			},
		},
	}

	return resp, nil
}

// autoUnfenceClientOnMatch removes the client address from the blocklist
// if it matches an entry with 'until' time less than or equal to
// AutoBlocklistTime duration.
func autoUnfenceClientOnMatch(
	ctx context.Context,
	conn *util.ClusterConnection,
	addr string,
) error {
	blocklistAdmin, err := conn.GetOSDAdmin()
	if err != nil {
		return err
	}

	list, err := blocklistAdmin.OSDBlocklist()
	if err != nil {
		return err
	}

	foundMatch, err := containsMatchingBlockListEntry(list, addr)
	if err != nil {
		return err
	}
	if !foundMatch {
		return nil
	}
	clientCIDR, err := util.ConvertIPToCIDR(addr)
	if err != nil {
		return fmt.Errorf("failed to convert IP to CIDR: %w", err)
	}

	log.DebugLog(ctx, "auto-unfencing client with address %q", addr)
	entry := osdAdmin.AddressEntry{
		Addr: clientCIDR,
	}

	return blocklistAdmin.OSDBlocklistRemove(entry)
}

// containsMatchingBlockListEntry checks if the provided address exists in the blocklist
// with a valid expiry time less than or equal to AutoBlocklistTime duration.
func containsMatchingBlockListEntry(
	blocklist *[]osdAdmin.Blocklist,
	addr string,
) (bool, error) {
	for _, entry := range *blocklist {
		if !matchEntry(entry.Addr, addr) {
			continue
		}

		timeLeftUntilExpire := time.Until(entry.Until)
		// Check if the blocklist entry is eligible for auto-unfencing if
		// 1. the time left until expiry is less than or equal to AutoBlocklistTime,
		// 2. the blocklist was done at least blockListCoolDownPeriod seconds (5 Minutes) ago.
		if timeLeftUntilExpire <= util.AutoBlocklistTime {
			if timeLeftUntilExpire > (util.AutoBlocklistTime - blockListCoolDownPeriod) {
				// still in cool-down period
				return false, fmt.Errorf("blocklist entry %q is still in cool-down period for %s",
					entry.Addr, (timeLeftUntilExpire - (util.AutoBlocklistTime - blockListCoolDownPeriod)))
			}

			return true, nil
		}
	}

	return false, nil
}

// matchEntry checks if the actual address matches the expected address along
// with the matching suffix (":0/32" for IPv4 and ":0/128" for IPv6).
func matchEntry(actual, expected string) bool {
	expectedIP := net.ParseIP(expected)
	if expectedIP == nil {
		return false
	}
	isIPv4 := expectedIP.To4() != nil

	// The actual address returned by ceph contains a weird ":0" which is not valid
	// cidr format and therefore explicitly handled below while matching
	// the cidr suffix("/32" or "/128").
	// example:
	// blocked cidr range = "192.168.1.1/32"
	// ceph blocklist entry = "192.168.1.1:0/32"
	// expected = "192.168.1.1"
	if isIPv4 {
		// for ipv4 address, strip the :0/32 suffix if present
		actual = strings.TrimSuffix(actual, ":0/32")
	} else {
		// for ipv6 address, strip the :0/128 suffix if present
		// Ceph returns IPv6 blocklist entries in bracket format: [fd98::9]:0/128
		actual = strings.TrimSuffix(actual, ":0/128")
		actual = strings.Trim(actual, "[]")
	}

	actualIP := net.ParseIP(actual)
	if actualIP == nil {
		return false
	}

	return expectedIP.Equal(actualIP)
}
