//go:build ceph_preview

package admin

import (
	ccom "github.com/ceph/go-ceph/common/commands"
	"github.com/ceph/go-ceph/internal/commands"
)

// TrashPurgeScheduleAdmin encapsulates management functions for
// ceph rbd trash purge schedules.
type TrashPurgeScheduleAdmin struct {
	conn ccom.MgrCommander
}

// TrashPurgeSchedule returns a TrashPurgeScheduleAdmin type for
// managing ceph rbd trash purge schedules.
func (ra *RBDAdmin) TrashPurgeSchedule() *TrashPurgeScheduleAdmin {
	return &TrashPurgeScheduleAdmin{conn: ra.conn}
}

// Add a new trash purge schedule to the given pool/namespace based on the
// supplied level spec.
//
// Similar To:
//
//	rbd trash purge schedule add <level_spec> <interval> <start_time>
func (tps *TrashPurgeScheduleAdmin) Add(l LevelSpec, i Interval, s StartTime) error {
	m := map[string]string{
		"prefix":     "rbd trash purge schedule add",
		"level_spec": l.spec,
		"format":     "json",
	}
	if i != NoInterval {
		m["interval"] = string(i)
	}
	if s != NoStartTime {
		m["start_time"] = string(s)
	}
	return commands.MarshalMgrCommand(tps.conn, m).NoData().End()
}

// List the trash purge schedules based on the supplied level spec.
//
// Similar To:
//
//	rbd trash purge schedule list <level_spec>
func (tps *TrashPurgeScheduleAdmin) List(l LevelSpec) ([]TrashPurgeSchedule, error) {
	m := map[string]string{
		"prefix":     "rbd trash purge schedule list",
		"level_spec": l.spec,
		"format":     "json",
	}
	return parseTrashPurgeScheduleList(
		commands.MarshalMgrCommand(tps.conn, m))
}

type trashPurgeScheduleMap map[string]trashPurgeScheduleSubsection

type trashPurgeScheduleSubsection struct {
	Name     string         `json:"name"`
	Schedule []ScheduleTerm `json:"schedule"`
}

// TrashPurgeSchedule contains values representing an entire trash purge
// schedule for a pool or namespace.
type TrashPurgeSchedule struct {
	Name        string
	LevelSpecID string
	Schedule    []ScheduleTerm
}

func parseTrashPurgeScheduleList(res commands.Response) (
	[]TrashPurgeSchedule, error) {

	var ss trashPurgeScheduleMap
	if err := res.NoStatus().Unmarshal(&ss).End(); err != nil {
		return nil, err
	}

	var sched []TrashPurgeSchedule
	for k, v := range ss {
		sched = append(sched, TrashPurgeSchedule{
			Name:        v.Name,
			LevelSpecID: k,
			Schedule:    v.Schedule,
		})
	}
	return sched, nil
}

// Remove a trash purge schedule matching the supplied arguments.
//
// Similar To:
//
//	rbd trash purge schedule remove <level_spec> <interval> <start_time>
func (tps *TrashPurgeScheduleAdmin) Remove(
	l LevelSpec, i Interval, s StartTime) error {

	m := map[string]string{
		"prefix":     "rbd trash purge schedule remove",
		"level_spec": l.spec,
		"format":     "json",
	}
	if i != NoInterval {
		m["interval"] = string(i)
	}
	if s != NoStartTime {
		m["start_time"] = string(s)
	}
	return commands.MarshalMgrCommand(tps.conn, m).NoData().End()
}

// Status returns the status of the trash purge schedule (eg. when it will
// next take place) matching the supplied level spec.
//
// Similar To:
//
//	rbd trash purge schedule status <level_spec>
func (tps *TrashPurgeScheduleAdmin) Status(l LevelSpec) ([]ScheduledPool, error) {
	m := map[string]string{
		"prefix":     "rbd trash purge schedule status",
		"level_spec": l.spec,
		"format":     "json",
	}
	return parseTrashPurgeScheduleStatus(
		commands.MarshalMgrCommand(tps.conn, m))
}

// ScheduledPool contains information about a pool or namespace with a scheduled
// trash purge and when it will next occur.
type ScheduledPool struct {
	PoolName     string `json:"pool_name"`
	PoolID       string `json:"pool_id"`
	Namespace    string `json:"namespace"`
	ScheduleTime string `json:"schedule_time"`
}

type scheduledPoolWrapper struct {
	Scheduled []ScheduledPool `json:"scheduled"`
}

func parseTrashPurgeScheduleStatus(res commands.Response) (
	[]ScheduledPool, error) {

	var spw scheduledPoolWrapper
	if err := res.NoStatus().Unmarshal(&spw).End(); err != nil {
		return nil, err
	}
	return spw.Scheduled, nil
}
