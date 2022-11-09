package controller

type TaskJob interface {
	Running() bool
	Success() bool
	Start() error
	Stop()
	Error() error
}

type InUseError struct {
	Err string
}

func (i InUseError) Error() string {
	return i.Err
}

type TaskController struct {
	jobs map[string]TaskJob
}

func NewTaskController() *TaskController {
	return &TaskController{jobs: make(map[string]TaskJob)}
}

func (t *TaskController) ContainTask(name string) bool {
	return t.jobs[name] != nil
}

func (t *TaskController) StartTask(name string, job TaskJob) (err error) {
	err = job.Start()
	if err != nil {
		job.Stop()
	} else {
		t.jobs[name] = job
	}
	return
}

func (t *TaskController) StopTask(name string) {
	job := t.jobs[name]
	if job != nil {
		job.Stop()
	}
}

func (t *TaskController) GetTask(name string) TaskJob {
	return t.jobs[name]
}

func (t *TaskController) DeleteTask(name string) {
	t.StopTask(name)
	delete(t.jobs, name)
}
