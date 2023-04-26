//go:build !nautilus
// +build !nautilus

package admin

import (
	ccom "github.com/ceph/go-ceph/common/commands"
	"github.com/ceph/go-ceph/internal/commands"
)

// TaskAdmin encapsulates management functions for ceph rbd task operations.
type TaskAdmin struct {
	conn ccom.MgrCommander
}

// Task returns a TaskAdmin type for managing ceph rbd task operations.
func (ra *RBDAdmin) Task() *TaskAdmin {
	return &TaskAdmin{conn: ra.conn}
}

// TaskRefs contains the action name and information about the image.
type TaskRefs struct {
	Action        string `json:"action"`
	PoolName      string `json:"pool_name"`
	PoolNamespace string `json:"pool_namespace"`
	ImageName     string `json:"image_name"`
	ImageID       string `json:"image_id"`
}

// TaskResponse contains the information about the task added on an image.
type TaskResponse struct {
	Sequence      int      `json:"sequence"`
	ID            string   `json:"id"`
	Message       string   `json:"message"`
	Refs          TaskRefs `json:"refs"`
	InProgress    bool     `json:"in_progress"`
	Progress      float64  `json:"progress"`
	RetryAttempts int      `json:"retry_attempts"`
	RetryTime     string   `json:"retry_time"`
	RetryMessage  string   `json:"retry_message"`
}

func parseTaskResponse(res commands.Response) (TaskResponse, error) {
	var taskResponse TaskResponse
	err := res.NoStatus().Unmarshal(&taskResponse).End()
	return taskResponse, err
}

func parseTaskResponseList(res commands.Response) ([]TaskResponse, error) {
	var taskResponseList []TaskResponse
	err := res.NoStatus().Unmarshal(&taskResponseList).End()
	return taskResponseList, err
}

// AddFlatten adds a background task to flatten a cloned image based on the
// supplied image spec.
//
// Similar To:
//
//	rbd task add flatten <image_spec>
func (ta *TaskAdmin) AddFlatten(img ImageSpec) (TaskResponse, error) {
	m := map[string]string{
		"prefix":     "rbd task add flatten",
		"image_spec": img.spec,
		"format":     "json",
	}
	return parseTaskResponse(commands.MarshalMgrCommand(ta.conn, m))
}

// AddRemove adds a background task to remove an image based on the supplied
// image spec.
//
// Similar To:
//
//	rbd task add remove <image_spec>
func (ta *TaskAdmin) AddRemove(img ImageSpec) (TaskResponse, error) {
	m := map[string]string{
		"prefix":     "rbd task add remove",
		"image_spec": img.spec,
		"format":     "json",
	}
	return parseTaskResponse(commands.MarshalMgrCommand(ta.conn, m))
}

// AddTrashRemove adds a background task to remove an image from the trash based
// on the supplied image id spec.
//
// Similar To:
//
//	rbd task add trash remove <image_id_spec>
func (ta *TaskAdmin) AddTrashRemove(img ImageSpec) (TaskResponse, error) {
	m := map[string]string{
		"prefix":        "rbd task add trash remove",
		"image_id_spec": img.spec,
		"format":        "json",
	}
	return parseTaskResponse(commands.MarshalMgrCommand(ta.conn, m))
}

// List pending or running asynchronous tasks.
//
// Similar To:
//
//	rbd task list
func (ta *TaskAdmin) List() ([]TaskResponse, error) {
	m := map[string]string{
		"prefix": "rbd task list",
		"format": "json",
	}
	return parseTaskResponseList(commands.MarshalMgrCommand(ta.conn, m))
}

// GetTaskByID returns pending or running asynchronous task using id.
//
// Similar To:
//
//	rbd task list <task_id>
func (ta *TaskAdmin) GetTaskByID(taskID string) (TaskResponse, error) {
	m := map[string]string{
		"prefix":  "rbd task list",
		"task_id": taskID,
		"format":  "json",
	}
	return parseTaskResponse(commands.MarshalMgrCommand(ta.conn, m))
}

// Cancel a pending or running asynchronous task.
//
// Similar To:
//
//	rbd task cancel <task_id>
func (ta *TaskAdmin) Cancel(taskID string) (TaskResponse, error) {
	m := map[string]string{
		"prefix":  "rbd task cancel",
		"task_id": taskID,
		"format":  "json",
	}
	return parseTaskResponse(commands.MarshalMgrCommand(ta.conn, m))
}
