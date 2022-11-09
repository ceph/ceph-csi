package rbdbackup

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util/log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	rbdv1 "github.com/ceph/ceph-csi/api/rbd/v1"
	"github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/controller/utils"
	"github.com/ceph/ceph-csi/internal/util"
)

type BackupTask struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	backup     *rbdv1.RBDBackup
	locks      *util.VolumeLocks
	cr         *util.Credentials
	monitor    string
	clusterId  string
	cmd        *exec.Cmd
	buf        bytes.Buffer
	isRunning  bool
}

func NewBackupTask(ctx context.Context, backup *rbdv1.RBDBackup,
	locks *util.VolumeLocks, cr *util.Credentials, monitor string, clusterId string) controller.TaskJob {
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	return &BackupTask{ctx: cancelCtx, cancelFunc: cancelFunc, backup: backup, locks: locks,
		cr: cr, monitor: monitor, clusterId: clusterId}
}

func (b *BackupTask) Running() bool {
	return b.isRunning
}

func (b *BackupTask) Success() bool {
	return !b.Running() && b.cmd.ProcessState.Success() && !strings.Contains(b.buf.String(), "error")
}

func (b *BackupTask) Start() error {
	ctx := context.TODO()
	poolName := b.backup.Spec.Pool
	snapshotName := b.backup.Spec.SnapshotName
	// Take lock to process only one snapshotHandle at a time.
	if ok := b.locks.TryAcquire(snapshotName); !ok {
		return fmt.Errorf(util.VolumeOperationAlreadyExistsFmt, snapshotName)
	}
	defer b.locks.Release(snapshotName)
	args, err := b.buildVolumeBackupArgs(b.backup.Spec.BackupDest, poolName, snapshotName, b.monitor, b.cr)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(b.ctx, "bash", args...)
	log.UsefulLog(ctx, "backup command: %v", args)
	cmd.Stdout = &b.buf
	cmd.Stderr = &b.buf
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true, // 使子进程拥有自己的 pgid，等同于子进程的 pid
		Pdeathsig: syscall.SIGTERM,
	}
	err = cmd.Start()
	if err != nil {
		err = fmt.Errorf("rbd: could not backup the volume %v cmd %v err: %s", snapshotName, args, err.Error())
		log.ErrorLogMsg(err.Error())
	}
	b.isRunning = true
	go func() {
		cmd.Wait()
		b.isRunning = false
		log.UsefulLog(ctx, fmt.Sprintf("%s %s", snapshotName, b.buf.String()))
	}()
	b.cmd = cmd
	return err
}

func (b *BackupTask) Stop() {
	b.cr.DeleteCredentials()
	b.cancelFunc()
}

func (b *BackupTask) Error() error {
	return fmt.Errorf(b.buf.String())
}

func (b *BackupTask) buildVolumeBackupArgs(backupDest string, pool string, image string,
	monitor string, cr *util.Credentials) ([]string, error) {
	var RBDVolArg []string
	bkpAddr := strings.Split(backupDest, ":")
	if len(bkpAddr) != 2 {
		return RBDVolArg, fmt.Errorf("rbd: invalid backup server address %s", backupDest)
	}

	vol := rbd.NewRbdVol(pool, b.monitor, b.clusterId, image)
	snapshot, err := vol.GetSnapshotName(cr)
	if err != nil {
		return RBDVolArg, err
	}

	timeout := os.Getenv("TIMEOUT")
	var timeoutInt int
	timeoutInt, err = strconv.Atoi(timeout)
	if err != nil {
		timeoutInt = 30
	}
	remote := fmt.Sprintf(" | gzip | nc -w %d -v %s %s", timeoutInt, bkpAddr[0], bkpAddr[1])
	cmd := fmt.Sprintf("%s %s %s/%s --id %s --keyfile=%s -m %s - %s", utils.RBDVolCmd, utils.RBDExportDiffArg,
		pool, snapshot, cr.ID, cr.KeyFile, monitor, remote)

	RBDVolArg = append(RBDVolArg, "-c", cmd)

	return RBDVolArg, nil
}
