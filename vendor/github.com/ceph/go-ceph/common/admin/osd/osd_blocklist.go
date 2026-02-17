//go:build !octopus && ceph_preview

package osd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/ceph/go-ceph/internal/commands"
)

const layout = "2006-01-02T15:04:05.000000-0700"

type expireTime time.Time

type ipList struct {
	IPAddr string     `json:"addr"`
	Until  expireTime `json:"until"`
}

type networkList struct {
	Network string     `json:"range"`
	Until   expireTime `json:"until"`
}

func (et *expireTime) UnmarshalText(data []byte) error {
	t, err := time.Parse(layout, string(data))
	if err != nil {
		return err
	}

	*et = expireTime(t)

	return nil
}

func (et *expireTime) Time() time.Time {
	return time.Time(*et)
}

// Blocklist contains the address and expire value for a blocklist entry.
type Blocklist struct {
	Addr  string
	Until time.Time
}

func parseBlocklist(res response) (*[]Blocklist, error) {
	var bl []Blocklist

	dec := json.NewDecoder(bytes.NewReader(res.Body()))

	for idx := 0; dec.More(); idx++ {
		switch idx {
		case 0:
			var ip []ipList
			err := dec.Decode(&ip)
			if err != nil {
				return nil, err
			}

			for _, i := range ip {
				bl = append(bl, Blocklist{
					Addr:  i.IPAddr,
					Until: i.Until.Time(),
				})
			}
		case 1:
			var nw []networkList
			err := dec.Decode(&nw)
			if err != nil {
				return nil, err
			}

			for _, n := range nw {
				bl = append(bl, Blocklist{
					Addr:  n.Network,
					Until: n.Until.Time(),
				})
			}
		default:
			return nil, ErrInvalidArgument
		}
	}

	return &bl, nil
}

// OSDBlocklist returns the list of blocklisted clients.
//
// Similar To:
//
//	ceph osd blocklist ls
func (osda *Admin) OSDBlocklist() (*[]Blocklist, error) {
	cmd := map[string]string{
		"prefix": "osd blocklist ls",
		"format": "json",
	}

	res := commands.MarshalMonCommand(osda.conn, cmd)
	if !res.Ok() {
		return nil, res.End()
	}

	return parseBlocklist(res)
}

// AddressEntry contains the ip or network address string along with the
// optional expire value in seconds.
type AddressEntry struct {
	Addr   string
	Expire float64
}

func isValidIP(addr string) bool {
	if ip := net.ParseIP(addr); ip != nil {
		return true
	}

	return false
}

func isValidCIDR(addr string) bool {
	if _, _, err := net.ParseCIDR(addr); err == nil {
		return true
	}

	return false
}

func blocklistOpCmd(addr string, op string) (map[string]any, error) {
	m := map[string]any{"prefix": "osd blocklist",
		"blocklistop": op,
		"addr":        addr,
	}

	if !isValidIP(addr) {
		if !isValidCIDR(addr) {
			return nil, ErrInvalidArgument
		}
		m["range"] = "range"
	}

	return m, nil
}

// float is a custom type that implements the MarshalJSON interface. This is
// used to format float64 values to one decimal place. By default these get
// converted to integers in the JSON output and fail the command.
type float float64

// MarshalJSON is a custom implementation for the JSON marshaling of float.
func (f float) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.1f", float64(f))), nil
}

// OSDBlocklistAdd adds an ip address or network address in CIDR format to the
// blocklist.
//
// Similar To:
//
//	ceph osd blocklist [range] add <ip_addr|cidr_network> [expire]
func (osda *Admin) OSDBlocklistAdd(entry AddressEntry) error {
	if entry.Addr == "" {
		return ErrEmptyArgument
	}

	cmd, err := blocklistOpCmd(entry.Addr, "add")
	if err != nil {
		return err
	}

	if entry.Expire < 0 {
		return ErrInvalidArgument
	}

	if entry.Expire != 0 {
		cmd["expire"] = float(entry.Expire)
	}

	res := commands.MarshalMonCommand(osda.conn, cmd)

	return res.End()
}

// OSDBlocklistRemove removes an ip address or network address from the
// blocklist.
//
// Similar To:
//
//	ceph osd blocklist [range] rm <ip_addr|cidr_network>
func (osda *Admin) OSDBlocklistRemove(entry AddressEntry) error {
	if entry.Addr == "" {
		return ErrEmptyArgument
	}

	cmd, err := blocklistOpCmd(entry.Addr, "rm")
	if err != nil {
		return err
	}

	res := commands.MarshalMonCommand(osda.conn, cmd)

	return res.End()
}
