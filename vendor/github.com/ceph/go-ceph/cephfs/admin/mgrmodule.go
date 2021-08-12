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
