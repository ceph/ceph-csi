package rbdbackup

import (
	"context"
	"fmt"
	"github.com/ceph/ceph-csi/internal/util/log"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	rbdv1 "github.com/ceph/ceph-csi/api/rbd/v1"
	cephctl "github.com/ceph/ceph-csi/internal/controller"
	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/controller/utils"
	"github.com/ceph/ceph-csi/internal/util"
)

type ReconcileRBDBackup struct {
	client  client.Client
	config  ctrl.Config
	locks   *util.VolumeLocks
	taskCtl *cephctl.TaskController
}

// Init will add the ReconcileRBDBackup to the list.
func Init() {
	// add ReconcileRBDBackup to the list
	ctrl.ControllerList = append(ctrl.ControllerList, &ReconcileRBDBackup{})
}

func (r *ReconcileRBDBackup) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newRBDBackupReconciler(mgr, config))
}

func newRBDBackupReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcileRBDBackup{
		client:  mgr.GetClient(),
		config:  config,
		locks:   util.NewVolumeLocks(),
		taskCtl: cephctl.NewTaskController(),
	}

	return r
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(
		"rbdbackup-controller",
		mgr,
		controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to RBDBackup
	err = c.Watch(&source.Kind{Type: &rbdv1.RBDBackup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	return nil
}

func (r *ReconcileRBDBackup) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	bk := &rbdv1.RBDBackup{}
	err := r.client.Get(ctx, request.NamespacedName, bk)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	taskName := request.NamespacedName.String()
	// Check if the object is under deletion
	if !bk.GetDeletionTimestamp().IsZero() {
		if r.taskCtl.ContainTask(taskName) {
			taskJob := r.taskCtl.GetTask(taskName)
			if taskJob.Running() {
				taskJob.Stop()
			}
			r.taskCtl.DeleteTask(taskName)
		}
		controllerutil.RemoveFinalizer(bk, utils.RBDFinalizer)
		err := r.client.Update(ctx, bk)
		if err != nil {
			log.ErrorLogMsg("failed to update %s %s", taskName, err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if bk.Status.Phase == rbdv1.BKPRBDStatusDone || bk.Status.Phase == rbdv1.BKPRBDStatusFailed {
		return reconcile.Result{}, nil
	}

	if !r.taskCtl.ContainTask(taskName) {
		log.UsefulLog(ctx, "%s create", bk.Name)
		cr, err := utils.GetCredentials(ctx, r.client, r.config.SecretName, r.config.SecretNamespace)
		if err != nil {
			log.ErrorLogMsg("failed to get credentials from secret %s", err)
		}
		monitors, _, err := util.FetchMappedClusterIDAndMons(ctx, r.config.ClusterId)
		if err != nil {
			log.ErrorLogMsg(err.Error())
		}

		taskJob := NewBackupTask(ctx, bk, r.locks, cr, monitors, r.config.ClusterId)
		err = r.taskCtl.StartTask(taskName, taskJob)
		if err != nil {
			klog.Errorf("backup %s failed %s@%s err %v", bk.Name, bk.Spec.VolumeName,
				bk.Spec.SnapshotName, err)
			err = r.UpdateBkpStatus(bk, rbdv1.BKPRBDStatusFailed)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	} else {
		taskJob := r.taskCtl.GetTask(taskName)
		if taskJob.Running() {
			log.UsefulLog(ctx, "%s running", bk.Name)
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		} else if taskJob.Success() {
			log.UsefulLog(ctx, "%s success", bk.Name)
			klog.Infof("backup %s done %s@%s", bk.Name, bk.Spec.VolumeName, bk.Spec.SnapshotName)
			r.taskCtl.DeleteTask(taskName)
			err = r.UpdateBkpStatus(bk, rbdv1.BKPRBDStatusDone)
		} else {
			log.UsefulLog(ctx, "%s fail", bk.Name)
			klog.Errorf("backup %s failed %s@%s err %v", bk.Name, bk.Spec.VolumeName,
				bk.Spec.SnapshotName, taskJob.Error())
			r.taskCtl.DeleteTask(taskName)
			err = r.UpdateBkpStatus(bk, rbdv1.BKPRBDStatusFailed)
		}
		if err != nil {
			log.ErrorLogMsg("failed to update status %s %s", taskName, err)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRBDBackup) UpdateBkpStatus(backup *rbdv1.RBDBackup, phase rbdv1.RBDBackupStatusPhase) (err error) {
	// controllerutil.AddFinalizer(backupCopy, utils.RBDFinalizer)
	backup.Status.Phase = phase
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.client.Update(context.TODO(), backup)
	})
}
