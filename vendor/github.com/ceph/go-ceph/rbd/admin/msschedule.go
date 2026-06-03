//go:build !nautilus
// +build !nautilus

package admin

import (
	ccom "github.com/ceph/go-ceph/common/commands"
	"github.com/ceph/go-ceph/internal/commands"
)

// Interval of time between scheduled snapshots. Typically in the form
// <num><m,h,d>. Exact content supported is defined internally on the ceph mgr.
type Interval string

// StartTime is the time the snapshot schedule begins. Exact content supported
// is defined internally on the ceph mgr.
type StartTime string

var (
	// NoInterval indicates no specific interval.
	NoInterval = Interval("")

	// NoStartTime indicates no specific start time.
	NoStartTime = StartTime("")
)

// MirrorSnashotScheduleAdmin encapsulates management functions for
// ceph rbd mirror snapshot schedules.
type MirrorSnashotScheduleAdmin struct {
	conn ccom.MgrCommander
}

// MirrorSnashotSchedule returns a MirrorSnashotScheduleAdmin type for
// managing ceph rbd mirror snapshot schedules.
func (ra *RBDAdmin) MirrorSnashotSchedule() *MirrorSnashotScheduleAdmin {
	return &MirrorSnashotScheduleAdmin{conn: ra.conn}
}

// Add a new snapshot schedule to the given pool/image based on the supplied
// level spec.
//
// Similar To:
//
//	rbd mirror snapshot schedule add <level_spec> <interval> <start_time>
func (mss *MirrorSnashotScheduleAdmin) Add(l LevelSpec, i Interval, s StartTime) error {
	m := map[string]string{
		"prefix":     "rbd mirror snapshot schedule add",
		"level_spec": l.spec,
		"format":     "json",
	}
	if i != NoInterval {
		m["interval"] = string(i)
	}
	if s != NoStartTime {
		m["start_time"] = string(s)
	}
	return commands.MarshalMgrCommand(mss.conn, m).NoData().End()
}

// List the snapshot schedules based on the supplied level spec.
//
// Similar To:
//
//	rbd mirror snapshot schedule list <level_spec>
func (mss *MirrorSnashotScheduleAdmin) List(l LevelSpec) ([]SnapshotSchedule, error) {
	m := map[string]string{
		"prefix":     "rbd mirror snapshot schedule list",
		"level_spec": l.spec,
		"format":     "json",
	}
	return parseMirrorSnapshotScheduleList(
		commands.MarshalMgrCommand(mss.conn, m))
}

type snapshotScheduleMap map[string]snapshotScheduleSubsection

type snapshotScheduleSubsection struct {
	Name     string         `json:"name"`
	Schedule []ScheduleTerm `json:"schedule"`
}

// ScheduleTerm represents the interval and start time component of
// a snapshot schedule.
type ScheduleTerm struct {
	Interval  Interval  `json:"interval"`
	StartTime StartTime `json:"start_time"`
}

// SnapshotSchedule contains values representing an entire snapshot schedule
// for an image or pool.
type SnapshotSchedule struct {
	Name        string
	LevelSpecID string
	Schedule    []ScheduleTerm
}

func parseMirrorSnapshotScheduleList(res commands.Response) (
	[]SnapshotSchedule, error) {

	var ss snapshotScheduleMap
	if err := res.NoStatus().Unmarshal(&ss).End(); err != nil {
		return nil, err
	}

	var sched []SnapshotSchedule
	for k, v := range ss {
		sched = append(sched, SnapshotSchedule{
			Name:        v.Name,
			LevelSpecID: k,
			Schedule:    v.Schedule,
		})
	}
	return sched, nil
}

// Remove a snapshot schedule matching the supplied arguments.
//
// Similar To:
//
//	rbd mirror snapshot schedule remove <level_spec> <interval> <start_time>
func (mss *MirrorSnashotScheduleAdmin) Remove(
	l LevelSpec, i Interval, s StartTime) error {

	m := map[string]string{
		"prefix":     "rbd mirror snapshot schedule remove",
		"level_spec": l.spec,
		"format":     "json",
	}
	if i != NoInterval {
		m["interval"] = string(i)
	}
	if s != NoStartTime {
		m["start_time"] = string(s)
	}
	return commands.MarshalMgrCommand(mss.conn, m).NoData().End()
}

// Status returns the status of the snapshot (eg. when it will next take place)
// matching the supplied level spec.
//
// Similar To:
//
//	rbd mirror snapshot schedule status <level_spec>
func (mss *MirrorSnashotScheduleAdmin) Status(l LevelSpec) ([]ScheduledImage, error) {
	m := map[string]string{
		"prefix":     "rbd mirror snapshot schedule status",
		"level_spec": l.spec,
		"format":     "json",
	}
	return parseMirrorSnapshotScheduleStatus(
		commands.MarshalMgrCommand(mss.conn, m))
}

// ScheduleTime is the time a snapshot will occur.
type ScheduleTime string

// ScheduledImage contains the item scheduled and when it will next occur.
type ScheduledImage struct {
	Image        string       `json:"image"`
	ScheduleTime ScheduleTime `json:"schedule_time"`
}

type scheduledImageWrapper struct {
	ScheduledImages []ScheduledImage `json:"scheduled_images"`
}

func parseMirrorSnapshotScheduleStatus(res commands.Response) (
	[]ScheduledImage, error) {

	var siw scheduledImageWrapper
	if err := res.NoStatus().Unmarshal(&siw).End(); err != nil {
		return nil, err
	}
	return siw.ScheduledImages, nil
}
