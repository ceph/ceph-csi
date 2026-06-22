//go:build !(nautilus || octopus || pacific || quincy) && ceph_preview

package nfs

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ceph/go-ceph/internal/commands"
)

var errUnknownApplyState = errors.New("apply returned unknown state")

// applyRes is used to parse the result from "nfs export apply" which can
// modify multiple exports with a single call. Each export that was (attempted
// to be) modified has JSON response with "pseudo" and "state" in it.
// ApplyExportInfo() modifies only a single export, but the returned response
// is still formatted in a JSON list.
type applyRes []*struct {
	Pseudo string `json:"pseudo"`
	State  string `json:"state"`
}

func parseApplyResults(res commands.Response) error {
	results := applyRes{}
	if err := res.NoStatus().Unmarshal(&results).End(); err != nil {
		return err
	}
	for _, ar := range results {
		if ar.State == "added" || ar.State == "updated" {
			// succes, nothing to do for this ar
			continue
		}
		return fmt.Errorf("%w %s for pseudo %s", errUnknownApplyState, ar.State, ar.Pseudo)
	}
	return nil
}

// ApplyExportInfo will create or update an existing NFS export.
//
// Similar To:
//
//	ceph nfs export apply
func (nfsa *Admin) ApplyExportInfo(clusterID string, info ExportInfo) error {
	buf, err := json.Marshal(info)
	if err != nil {
		return err
	}
	m := map[string]string{
		"prefix":     "nfs export apply",
		"format":     "json",
		"cluster_id": clusterID,
	}
	return parseApplyResults(commands.MarshalMgrCommandWithBuffer(nfsa.conn, m, buf))
}
