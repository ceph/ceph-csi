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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
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
	invalidCommandStr = "invalid command"
	// we can always use mds rank 0, since all the clients have a session with rank-0.
	mdsRank = 0
)

// NetworkFence contains the CIDR blocks to be blocked.
type NetworkFence struct {
	Cidr     []string
	Monitors string
	cr       *util.Credentials
}

// activeClient represents the structure of an active client.
type activeClient struct {
	Inst string `json:"inst"`
}

// IPWithNonce represents the structure of an IP with nonce
// as listed by Ceph OSD blocklist.
type IPWithNonce struct {
	IP    string `json:"ip"`
	Nonce string `json:"nonce"`
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

// AddClientEviction blocks access for all the IPs in the CIDR block
// using client eviction, it also blocks the entire CIDR.
func (nf *NetworkFence) AddClientEviction(ctx context.Context) error {
	evictedIPs := make(map[string]bool)
	// fetch active clients
	activeClients, err := nf.listActiveClients(ctx)
	if err != nil {
		return err
	}
	// iterate through CIDR blocks and check if any active client matches
	for _, cidr := range nf.Cidr {
		for _, client := range activeClients {
			var clientIP string
			clientIP, err = client.fetchIP()
			if err != nil {
				return fmt.Errorf("error fetching client IP: %w", err)
			}
			// check if the clientIP is in the CIDR block
			if isIPInCIDR(ctx, clientIP, cidr) {
				var clientID int
				clientID, err = client.fetchID()
				if err != nil {
					return fmt.Errorf("error fetching client ID: %w", err)
				}
				// evict the client
				err = nf.evictCephFSClient(ctx, clientID)
				if err != nil {
					return fmt.Errorf("error evicting client %d: %w", clientID, err)
				}
				log.DebugLog(ctx, "client %d has been evicted\n", clientID)
				// add the CIDR to the list of blocklisted IPs
				evictedIPs[clientIP] = true
			}
		}
	}

	// add the range based blocklist for CIDR
	err = nf.AddNetworkFence(ctx)
	if err != nil {
		return err
	}

	return nil
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

func (nf *NetworkFence) RemoveClientEviction(ctx context.Context) error {
	// Remove the CIDR block first
	err := nf.RemoveNetworkFence(ctx)
	if err != nil {
		return err
	}

	// Get the ceph blocklist
	blocklist, err := nf.getCephBlocklist(ctx)
	if err != nil {
		return err
	}

	// For each CIDR block, remove the IPs in the blocklist
	// that fall under the CIDR with nonce
	for _, cidr := range nf.Cidr {
		hosts := nf.parseBlocklistForCIDR(ctx, blocklist, cidr)
		log.DebugLog(ctx, "parsed blocklist for CIDR %s: %+v", cidr, hosts)

		for _, host := range hosts {
			err := nf.removeCephBlocklist(ctx, host.IP, host.Nonce, false)
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

func (nf *NetworkFence) listActiveClients(ctx context.Context) ([]activeClient, error) {
	arg := []string{
		"--id", nf.cr.ID,
		"--keyfile=" + nf.cr.KeyFile,
		"-m", nf.Monitors,
	}
	// FIXME: replace the ceph command with go-ceph API in future
	cmd := []string{"tell", fmt.Sprintf("mds.%d", mdsRank), "client", "ls"}
	cmd = append(cmd, arg...)
	stdout, stdErr, err := util.ExecCommandWithTimeout(ctx, 2*time.Minute, "ceph", cmd...)
	if err != nil {
		return nil, fmt.Errorf("failed to list active clients: %w, stderr: %q", err, stdErr)
	}

	var activeClients []activeClient
	if err := json.Unmarshal([]byte(stdout), &activeClients); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return activeClients, nil
}

func (nf *NetworkFence) evictCephFSClient(ctx context.Context, clientID int) error {
	arg := []string{
		"--id", nf.cr.ID,
		"--keyfile=" + nf.cr.KeyFile,
		"-m", nf.Monitors,
	}
	// FIXME: replace the ceph command with go-ceph API in future
	cmd := []string{"tell", fmt.Sprintf("mds.%d", mdsRank), "client", "evict", fmt.Sprintf("id=%d", clientID)}
	cmd = append(cmd, arg...)
	_, stdErr, err := util.ExecCommandWithTimeout(ctx, 2*time.Minute, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to evict client %d: %w, stderr: %q", clientID, err, stdErr)
	}
	log.DebugLog(ctx, "client %s has been evicted from CephFS\n", clientID)

	return nil
}

func isIPInCIDR(ctx context.Context, ip, cidr string) bool {
	// Parse the CIDR block
	_, ipCidr, err := net.ParseCIDR(cidr)
	if err != nil {
		log.ErrorLog(ctx, "error parsing CIDR block %s: %w\n", cidr, err)

		return false
	}

	// Parse the IP address
	ipAddress := net.ParseIP(ip)
	if ipAddress == nil {
		log.ErrorLog(ctx, "error parsing IP address %s\n", ip)

		return false
	}

	// Check if the IP address is within the CIDR block
	return ipCidr.Contains(ipAddress)
}

func (ac *activeClient) fetchIP() (string, error) {
	// example: "inst": "client.4305 172.21.9.34:0/422650892",
	// then returning value will be 172.21.9.34
	return util.ParseClientIP(ac.Inst)
}

func (ac *activeClient) fetchID() (int, error) {
	// example: "inst": "client.4305 172.21.9.34:0/422650892",
	// then returning value will be 4305
	clientInfo := ac.Inst
	parts := strings.Fields(clientInfo)
	if len(parts) >= 1 {
		clientIDStr := strings.TrimPrefix(parts[0], "client.")
		clientID, err := strconv.Atoi(clientIDStr)
		if err != nil {
			return 0, fmt.Errorf("failed to convert client ID to int: %w", err)
		}

		return clientID, nil
	}

	return 0, fmt.Errorf("failed to extract client ID, incorrect format: %s", clientInfo)
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

// getCephBlocklist fetches the ceph blocklist and returns it as a string.
func (nf *NetworkFence) getCephBlocklist(ctx context.Context) (string, error) {
	arg := []string{
		"--id", nf.cr.ID,
		"--keyfile=" + nf.cr.KeyFile,
		"-m", nf.Monitors,
	}
	// FIXME: replace the ceph command with go-ceph API in future
	cmd := []string{"osd", "blocklist", "ls"}
	cmd = append(cmd, arg...)
	stdout, stdErr, err := util.ExecCommandWithTimeout(ctx, 2*time.Minute, "ceph", cmd...)
	if err != nil {
		return "", fmt.Errorf("failed to get the ceph blocklist: %w, stderr: %q", err, stdErr)
	}

	return stdout, nil
}

// parseBlocklistEntry parses a single entry from the ceph blocklist
// and returns the IPWithNonce.
func (nf *NetworkFence) parseBlocklistEntry(entry string) IPWithNonce {
	parts := strings.Fields(entry)
	if len(parts) == 0 {
		return IPWithNonce{}
	}

	ipPortNonce := strings.SplitN(parts[0], "/", 2)
	if len(ipPortNonce) != 2 {
		return IPWithNonce{}
	}

	ipPort := ipPortNonce[0]
	nonce := ipPortNonce[1]

	lastColonIndex := strings.LastIndex(ipPortNonce[0], ":")
	if lastColonIndex == -1 {
		return IPWithNonce{}
	}

	if len(ipPort) <= lastColonIndex {
		return IPWithNonce{}
	}
	ip := ipPort[:lastColonIndex]

	return IPWithNonce{IP: ip, Nonce: nonce}
}

// parseBlocklistForCIDR scans the blocklist for the given CIDR and returns
// the list of IPs that lie within the CIDR along with their nonce.
func (nf *NetworkFence) parseBlocklistForCIDR(ctx context.Context, blocklist, cidr string) []IPWithNonce {
	blocklistEntries := strings.Split(blocklist, "\n")

	matchingHosts := make([]IPWithNonce, 0)
	for _, entry := range blocklistEntries {
		entry = strings.TrimSpace(entry)

		// Skip unrelated ranged blocks and invalid entries
		if strings.Contains(entry, "cidr") || !strings.Contains(entry, "/") {
			continue
		}

		blockedHost := nf.parseBlocklistEntry(entry)
		if isIPInCIDR(ctx, blockedHost.IP, cidr) {
			matchingHosts = append(matchingHosts, blockedHost)
		}
	}

	return matchingHosts
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

		// Until is in ISO8601 format.
		until, err := time.Parse(ISO8601TimeLayout, entry.Until)
		if err != nil {
			return false, fmt.Errorf("failed to parse blocklist entry time %q: %w",
				entry.Until, err)
		}

		if until.Sub(time.Now()) <= util.AutoBlocklistTime {
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
		actual = strings.TrimSuffix(actual, ":0/128")
	}

	actualIP := net.ParseIP(actual)
	if actualIP == nil {
		return false
	}

	return expectedIP.Equal(actualIP)
}
