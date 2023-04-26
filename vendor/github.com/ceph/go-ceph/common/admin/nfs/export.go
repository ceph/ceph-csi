//go:build !(nautilus || octopus)
// +build !nautilus,!octopus

package nfs

import (
	"errors"

	"github.com/ceph/go-ceph/internal/commands"
)

// SquashMode indicates the kind of user-id squashing performed on an export.
type SquashMode string

// src: https://github.com/nfs-ganesha/nfs-ganesha/blob/next/src/config_samples/export.txt
const (
	// NoneSquash performs no id squashing.
	NoneSquash SquashMode = "None"
	// RootSquash performs squashing of root user (with any gid).
	RootSquash SquashMode = "Root"
	// AllSquash performs squashing of all users.
	AllSquash SquashMode = "All"
	// RootIDSquash performs squashing of root uid/gid.
	RootIDSquash SquashMode = "RootId"
	// NoRootSquash is equivalent to NoneSquash
	NoRootSquash = NoneSquash
	// Unspecifiedquash
	Unspecifiedquash SquashMode = ""
)

var (
	errNoExportInfo = errors.New("No export info found")
)

// SecType indicates the kind of security/authentication to be used by an export.
type SecType string

// src: https://github.com/nfs-ganesha/nfs-ganesha/blob/next/src/config_samples/export.txt
const (
	SysSec   SecType = "sys"
	NoneSec  SecType = "none"
	Krb5Sec  SecType = "krb5"
	Krb5iSec SecType = "krb5i"
	Krb5pSec SecType = "krb5p"
)

// CephFSExportSpec is used to specify the parameters used to create a new
// CephFS based export.
type CephFSExportSpec struct {
	FileSystemName string     `json:"fsname"`
	ClusterID      string     `json:"cluster_id"`
	PseudoPath     string     `json:"pseudo_path"`
	Path           string     `json:"path,omitempty"`
	ReadOnly       bool       `json:"readonly"`
	ClientAddr     []string   `json:"client_addr,omitempty"`
	Squash         SquashMode `json:"squash,omitempty"`
	SecType        []SecType  `json:"sectype,omitempty"`
}

// ExportResult is returned along with newly created exports.
type ExportResult struct {
	Bind           string `json:"bind"`
	FileSystemName string `json:"fs"`
	Path           string `json:"path"`
	ClusterID      string `json:"cluster"`
	Mode           string `json:"mode"`
}

type cephFSExportFields struct {
	Prefix string `json:"prefix"`
	Format string `json:"format"`

	CephFSExportSpec
}

// FSALInfo describes NFS-Ganesha specific FSAL properties of an export.
type FSALInfo struct {
	Name           string `json:"name"`
	UserID         string `json:"user_id"`
	FileSystemName string `json:"fs_name"`
}

// ClientInfo describes per-client parameters of an export.
type ClientInfo struct {
	Addresses  []string   `json:"addresses"`
	AccessType string     `json:"access_type"`
	Squash     SquashMode `json:"squash"`
}

// ExportInfo describes an NFS export.
type ExportInfo struct {
	ExportID      int64        `json:"export_id"`
	Path          string       `json:"path"`
	ClusterID     string       `json:"cluster_id"`
	PseudoPath    string       `json:"pseudo"`
	AccessType    string       `json:"access_type"`
	Squash        SquashMode   `json:"squash"`
	SecurityLabel bool         `json:"security_label"`
	Protocols     []int        `json:"protocols"`
	Transports    []string     `json:"transports"`
	FSAL          FSALInfo     `json:"fsal"`
	Clients       []ClientInfo `json:"clients"`
	SecType       []SecType    `json:"sectype"`
}

func parseExportResult(res commands.Response) (*ExportResult, error) {
	r := &ExportResult{}
	if err := res.NoStatus().Unmarshal(r).End(); err != nil {
		return nil, err
	}
	return r, nil
}

func parseExportsList(res commands.Response) ([]ExportInfo, error) {
	l := []ExportInfo{}
	if err := res.NoStatus().Unmarshal(&l).End(); err != nil {
		return nil, err
	}
	return l, nil
}

func parseExportInfo(res commands.Response) (ExportInfo, error) {
	i := ExportInfo{}
	// different versions of ceph may return nothing or empty json.
	// detect these cases and return a specific error
	if res.NoStatus().EmptyBody().Ok() {
		return i, errNoExportInfo
	}
	if err := res.NoStatus().Unmarshal(&i).End(); err != nil {
		return i, err
	}
	return i, nil
}

// CreateCephFSExport will create a new NFS export for a CephFS file system.
//
// Similar To:
//
//	ceph nfs export create cephfs
func (nfsa *Admin) CreateCephFSExport(spec CephFSExportSpec) (
	*ExportResult, error) {
	// ---
	f := &cephFSExportFields{
		Prefix:           "nfs export create cephfs",
		Format:           "json",
		CephFSExportSpec: spec,
	}
	return parseExportResult(commands.MarshalMgrCommand(nfsa.conn, f))
}

const delSucc = "Successfully deleted export"

// RemoveExport will remove an NFS export based on the pseudo-path of the export.
//
// Similar To:
//
//	ceph nfs export rm
func (nfsa *Admin) RemoveExport(clusterID, pseudoPath string) error {
	m := map[string]string{
		"prefix":      "nfs export rm",
		"format":      "json",
		"cluster_id":  clusterID,
		"pseudo_path": pseudoPath,
	}
	return (commands.MarshalMgrCommand(nfsa.conn, m).
		FilterBodyPrefix(delSucc).NoData().End())
}

// ListDetailedExports will return a list of exports with details.
//
// Similar To:
//
//	ceph nfs export ls --detailed
func (nfsa *Admin) ListDetailedExports(clusterID string) ([]ExportInfo, error) {
	/*
		NOTE: there is no simple list because based on a quick reading of the code
		in ceph, the details fetching should not be significantly slower with
		details than without, and since this is an API call not a CLI its easy
		enough to ignore the details you don't care about. If I'm wrong, and
		we discover a major perf. difference in the future we can always add a new
		simpler list-without-details function.
	*/
	m := map[string]string{
		"prefix":     "nfs export ls",
		"detailed":   "true",
		"format":     "json",
		"cluster_id": clusterID,
	}
	return parseExportsList(commands.MarshalMgrCommand(nfsa.conn, m))
}

// ExportInfo will return a structure describing the export specified by it's
// pseudo-path.
//
// Similar To:
//
//	ceph nfs export info
func (nfsa *Admin) ExportInfo(clusterID, pseudoPath string) (ExportInfo, error) {
	m := map[string]string{
		"prefix":      "nfs export info",
		"format":      "json",
		"cluster_id":  clusterID,
		"pseudo_path": pseudoPath,
	}
	return parseExportInfo(commands.MarshalMgrCommand(nfsa.conn, m))
}

/*
TODO?

'nfs export apply': cluster_id: str, inbuf: str
"""Create or update an export by `-i <json_or_ganesha_export_file>`"""


'nfs export create rgw':
	   bucket: str,
	   cluster_id: str,
	   pseudo_path: str,
	   readonly: Optional[bool] = False,
	   client_addr: Optional[List[str]] = None,
	   squash: str = 'none',
"""Create an RGW export"""
*/
