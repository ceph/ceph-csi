package rbdrestore

import (
	"context"
	"fmt"
	"github.com/ceph/ceph-csi/internal/util/log"
	"time"

	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	rbdv1 "github.com/ceph/ceph-csi/api/rbd/v1"
	cephctl "github.com/ceph/ceph-csi/internal/controller"
	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/controller/utils"
	"github.com/ceph/ceph-csi/internal/util"
)

type ReconcileRBDRestore struct {
	client  client.Client
	config  ctrl.Config
	locks   *util.VolumeLocks
	taskCtl *cephctl.TaskController
}

// Init will add the ReconcileRBDRestore to the list.
func Init() {
	// add ReconcileRBDRestore to the list
	ctrl.ControllerList = append(ctrl.ControllerList, &ReconcileRBDRestore{})
}

func (r *ReconcileRBDRestore) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newRBDRestoreReconciler(mgr, config))
}

func newRBDRestoreReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcileRBDRestore{
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
		"rbdrestore-controller",
		mgr,
		controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to RBDRestore
	err = c.Watch(&source.Kind{Type: &rbdv1.RBDRestore{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	return nil
}

func (r *ReconcileRBDRestore) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rs := &rbdv1.RBDRestore{}
	err := r.client.Get(ctx, request.NamespacedName, rs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}
	taskName := request.NamespacedName.String()
	// Check if the object is under deletion
	if !rs.GetDeletionTimestamp().IsZero() {
		if r.taskCtl.ContainTask(taskName) {
			taskJob := r.taskCtl.GetTask(taskName)
			if taskJob.Running() {
				taskJob.Stop()
			}
			r.taskCtl.DeleteTask(taskName)
		}
		controllerutil.RemoveFinalizer(rs, utils.RBDFinalizer)
		err := r.client.Update(ctx, rs)
		if err != nil {
			log.ErrorLogMsg("failed to update %s %s", taskName, err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if rs.Status.Phase == rbdv1.RSTRBDStatusDone || rs.Status.Phase == rbdv1.RSTRBDStatusFailed {
		return reconcile.Result{}, nil
	}

	defer func() {
		if err != nil {
			r.taskCtl.DeleteTask(taskName)
		}
	}()

	if !r.taskCtl.ContainTask(taskName) {

		cr, err := utils.GetCredentials(ctx, r.client, r.config.SecretName, r.config.SecretNamespace)
		if err != nil {
			log.ErrorLogMsg("failed to get credentials from secret %s", err)
		}
		monitors, _, err := util.FetchMappedClusterIDAndMons(ctx, r.config.ClusterId)
		if err != nil {
			log.ErrorLogMsg(err.Error())
		}

		taskJob := NewRestoreTask(ctx, rs, r.locks, cr, monitors, r.config.ClusterId)
		err = r.taskCtl.StartTask(taskName, taskJob)
		if err != nil {

			if _, ok := err.(cephctl.InUseError); ok {
				klog.Info("image already inuse, requeue and retry")
				return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
			}

			klog.Errorf("restore %s failed %s err %v", rs.Name, rs.Spec.ImageName, err)
			err = r.UpdateRspStatus(rs, rbdv1.RSTRBDStatusFailed)
		}
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	} else {
		taskJob := r.taskCtl.GetTask(taskName)
		if taskJob.Running() {
			log.UsefulLog(ctx, "%s running", rs.Name)
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		} else if taskJob.Success() {
			log.UsefulLog(ctx, "%s success", rs.Name)
			klog.Infof("restore %s done %s", rs.Name, rs.Spec.ImageName)
			err = r.UpdateRspStatus(rs, rbdv1.RSTRBDStatusDone)
		} else {
			log.UsefulLog(ctx, "%s fail", rs.Name)
			klog.Errorf("restore %s failed %s err %v", rs.Name, rs.Spec.ImageName, taskJob.Error())
			err = r.UpdateRspStatus(rs, rbdv1.RSTRBDStatusFailed)
		}
		if err != nil {
			log.ErrorLogMsg("failed to update status %s %s", taskName, err)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRBDRestore) UpdateRspStatus(restore *rbdv1.RBDRestore, phase rbdv1.RBDRestoreStatusPhase) (err error) {
	// controllerutil.AddFinalizer(restoreCopy, utils.RBDFinalizer)
	restore.Status.Phase = phase
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.client.Update(context.TODO(), restore)
	})
}
