// +build !luminous,!mimic

package admin

// this is the internal type used to create JSON for ceph.
// See SubVolumeGroupOptions for the type that users of the library
// interact with.
// note that the ceph json takes mode as a string.
type subVolumeGroupFields struct {
	Prefix     string `json:"prefix"`
	Format     string `json:"format"`
	VolName    string `json:"vol_name"`
	GroupName  string `json:"group_name"`
	Uid        int    `json:"uid,omitempty"`
	Gid        int    `json:"gid,omitempty"`
	Mode       string `json:"mode,omitempty"`
	PoolLayout string `json:"pool_layout,omitempty"`
}

// SubVolumeGroupOptions are used to specify optional, non-identifying, values
// to be used when creating a new subvolume group.
type SubVolumeGroupOptions struct {
	Uid        int
	Gid        int
	Mode       int
	PoolLayout string
}

func (s *SubVolumeGroupOptions) toFields(v, g string) *subVolumeGroupFields {
	return &subVolumeGroupFields{
		Prefix:     "fs subvolumegroup create",
		Format:     "json",
		VolName:    v,
		GroupName:  g,
		Uid:        s.Uid,
		Gid:        s.Gid,
		Mode:       modeString(s.Mode, false),
		PoolLayout: s.PoolLayout,
	}
}

// CreateSubVolumeGroup sends a request to create a subvolume group in a volume.
//
// Similar To:
//  ceph fs subvolumegroup create <volume> <group_name>  ...
func (fsa *FSAdmin) CreateSubVolumeGroup(volume, name string, o *SubVolumeGroupOptions) error {
	if o == nil {
		o = &SubVolumeGroupOptions{}
	}
	res := fsa.marshalMgrCommand(o.toFields(volume, name))
	return res.noData().End()
}

// ListSubVolumeGroups returns a list of subvolume groups belonging to the
// specified volume.
//
// Similar To:
//  ceph fs subvolumegroup ls cephfs <volume>
func (fsa *FSAdmin) ListSubVolumeGroups(volume string) ([]string, error) {
	res := fsa.marshalMgrCommand(map[string]string{
		"prefix":   "fs subvolumegroup ls",
		"vol_name": volume,
		"format":   "json",
	})
	return parseListNames(res)
}

// RemoveSubVolumeGroup will delete a subvolume group in a volume.
// Similar To:
//  ceph fs subvolumegroup rm <volume> <group_name>
func (fsa *FSAdmin) RemoveSubVolumeGroup(volume, name string) error {
	return fsa.rmSubVolumeGroup(volume, name, rmFlags{})
}

// ForceRemoveSubVolumeGroup will delete a subvolume group in a volume.
// Similar To:
//  ceph fs subvolumegroup rm <volume> <group_name> --force
func (fsa *FSAdmin) ForceRemoveSubVolumeGroup(volume, name string) error {
	return fsa.rmSubVolumeGroup(volume, name, rmFlags{force: true})
}

func (fsa *FSAdmin) rmSubVolumeGroup(volume, name string, o rmFlags) error {
	res := fsa.marshalMgrCommand(o.Update(map[string]string{
		"prefix":     "fs subvolumegroup rm",
		"vol_name":   volume,
		"group_name": name,
		"format":     "json",
	}))
	return res.noData().End()
}

// SubVolumeGroupPath returns the path to the subvolume from the root of the
// file system.
//
// Similar To:
//  ceph fs subvolumegroup getpath <volume> <group_name>
func (fsa *FSAdmin) SubVolumeGroupPath(volume, name string) (string, error) {
	m := map[string]string{
		"prefix":     "fs subvolumegroup getpath",
		"vol_name":   volume,
		"group_name": name,
		// ceph doesn't respond in json for this cmd (even if you ask)
	}
	return parsePathResponse(fsa.marshalMgrCommand(m))
}

// CreateSubVolumeGroupSnapshot creates a new snapshot from the source subvolume group.
//
// Similar To:
//  ceph fs subvolumegroup snapshot create <volume> <group> <name>
func (fsa *FSAdmin) CreateSubVolumeGroupSnapshot(volume, group, name string) error {
	m := map[string]string{
		"prefix":     "fs subvolumegroup snapshot create",
		"vol_name":   volume,
		"group_name": group,
		"snap_name":  name,
		"format":     "json",
	}
	return fsa.marshalMgrCommand(m).noData().End()
}

// RemoveSubVolumeGroupSnapshot removes the specified snapshot from the subvolume group.
//
// Similar To:
//  ceph fs subvolumegroup snapshot rm <volume> <group> <name>
func (fsa *FSAdmin) RemoveSubVolumeGroupSnapshot(volume, group, name string) error {
	return fsa.rmSubVolumeGroupSnapshot(volume, group, name, rmFlags{})
}

// ForceRemoveSubVolumeGroupSnapshot removes the specified snapshot from the subvolume group.
//
// Similar To:
//  ceph fs subvolumegroup snapshot rm <volume> <group> <name> --force
func (fsa *FSAdmin) ForceRemoveSubVolumeGroupSnapshot(volume, group, name string) error {
	return fsa.rmSubVolumeGroupSnapshot(volume, group, name, rmFlags{force: true})
}

func (fsa *FSAdmin) rmSubVolumeGroupSnapshot(volume, group, name string, o rmFlags) error {
	m := map[string]string{
		"prefix":     "fs subvolumegroup snapshot rm",
		"vol_name":   volume,
		"group_name": group,
		"snap_name":  name,
		"format":     "json",
	}
	return fsa.marshalMgrCommand(o.Update(m)).noData().End()
}

// ListSubVolumeGroupSnapshots returns a listing of snapshots for a given subvolume group.
//
// Similar To:
//  ceph fs subvolumegroup snapshot ls <volume> <group>
func (fsa *FSAdmin) ListSubVolumeGroupSnapshots(volume, group string) ([]string, error) {
	m := map[string]string{
		"prefix":     "fs subvolumegroup snapshot ls",
		"vol_name":   volume,
		"group_name": group,
		"format":     "json",
	}
	return parseListNames(fsa.marshalMgrCommand(m))
}
