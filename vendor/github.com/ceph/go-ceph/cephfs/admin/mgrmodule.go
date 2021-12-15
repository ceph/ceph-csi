package admin

import (
	"github.com/ceph/go-ceph/internal/commands"
)

const mirroring = "mirroring"

// EnableModule will enable the specified manager module.
//
// Similar To:
//  ceph mgr module enable <module> [--force]
func (fsa *FSAdmin) EnableModule(module string, force bool) error {
	m := map[string]string{
		"prefix": "mgr module enable",
		"module": module,
		"format": "json",
	}
	if force {
		m["force"] = "--force"
	}
	// Why is this _only_ part of the mon command json? You'd think a mgr
	// command would be available as a MgrCommand but I couldn't figure it out.
	return commands.MarshalMonCommand(fsa.conn, m).NoData().End()
}

// DisableModule will disable the specified manager module.
//
// Similar To:
//  ceph mgr module disable <module>
func (fsa *FSAdmin) DisableModule(module string) error {
	m := map[string]string{
		"prefix": "mgr module disable",
		"module": module,
		"format": "json",
	}
	return commands.MarshalMonCommand(fsa.conn, m).NoData().End()
}

// EnableMirroringModule will enable the mirroring module for cephfs.
//
// Similar To:
//  ceph mgr module enable mirroring [--force]
func (fsa *FSAdmin) EnableMirroringModule(force bool) error {
	return fsa.EnableModule(mirroring, force)
}

// DisableMirroringModule will disable the mirroring module for cephfs.
//
// Similar To:
//  ceph mgr module disable mirroring
func (fsa *FSAdmin) DisableMirroringModule() error {
	return fsa.DisableModule(mirroring)
}

type moduleInfo struct {
	EnabledModules []string `json:"enabled_modules"`
	//DisabledModules []string `json:"disabled_modules"`
	// DisabledModules is documented in ceph as a list of string
	// but that's not what comes back from the server (on pacific).
	// Since we don't need this today, we're just going to ignore
	// it, but if we ever want to support this for external consumers
	// we'll need to figure out the real structure of this.
}

func parseModuleInfo(res response) (*moduleInfo, error) {
	m := &moduleInfo{}
	if err := res.NoStatus().Unmarshal(m).End(); err != nil {
		return nil, err
	}
	return m, nil
}

// listModules returns moduleInfo or error. it is not exported because
// this is really not a cephfs specific thing but we needed it
// for cephfs tests. maybe lift it somewhere else someday.
func (fsa *FSAdmin) listModules() (*moduleInfo, error) {
	m := map[string]string{
		"prefix": "mgr module ls",
		"format": "json",
	}
	return parseModuleInfo(commands.MarshalMonCommand(fsa.conn, m))
}
