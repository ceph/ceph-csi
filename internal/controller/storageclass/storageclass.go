package storageclass

import (
	"context"
	"fmt"
	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/controller/utils"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
	"github.com/robfig/cron"
	scv1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"os/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

const (
	rbdDefaultName   = "rbd.csi.ceph.com"
	DefaultTrashTime = "0 0 1 * * ?"
)

// ReconcileStorageClass reconciles a storageclass object.
type ReconcileStorageClass struct {
	client  client.Client
	config  ctrl.Config
	cron    *cron.Cron
	poolMap map[string]time.Time
}

var (
	_ reconcile.Reconciler = &ReconcileStorageClass{}
	_ ctrl.Manager         = &ReconcileStorageClass{}
)

// Init will add the ReconcileStorageClass to the list.
func Init() {
	// add ReconcileStorageClass to the list
	ctrl.ControllerList = append(ctrl.ControllerList, ReconcileStorageClass{})
}

// Add adds the newSCReconciler.
func (r ReconcileStorageClass) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newSCReconciler(mgr, config))
}

// newReconciler returns a ReconcileStorageClass.
func newSCReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcileStorageClass{
		client:  mgr.GetClient(),
		config:  config,
		cron:    cron.New(),
		poolMap: make(map[string]time.Time),
	}
	return r
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(
		"storageclass-controller",
		mgr,
		controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to StorageClass
	err = c.Watch(&source.Kind{Type: &scv1.StorageClass{}}, &handler.EnqueueRequestForObject{}, &CephPredicate{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	return nil
}

// Reconcile reconciles the storageclass object
func (r *ReconcileStorageClass) Reconcile(ctx context.Context,
	request reconcile.Request) (reconcile.Result, error) {
	log.UsefulLog(ctx, "req: %v", request.NamespacedName)
	sc := &scv1.StorageClass{}
	err := r.client.Get(ctx, request.NamespacedName, sc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	cr, err := utils.GetCredentials(ctx, r.client, r.config.SecretName, r.config.SecretNamespace)
	if err != nil {
		log.ErrorLogMsg("failed to get credentials from secret %v", err)
		return reconcile.Result{}, err
	}
	defer cr.DeleteCredentials()
	monitors, _, err := util.GetMonsAndClusterID(ctx, r.config.ClusterId, false)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%v)", err)
		return reconcile.Result{}, err
	}
	poolName := sc.Parameters["pool"]
	next, ok := r.poolMap[poolName]
	schedule, err := cron.Parse(r.config.ScheduleSpec)
	if err != nil {
		log.ErrorLog(ctx, "cron.Parse failed: %v", err)
		return reconcile.Result{}, err
	}
	if !ok {
		log.UsefulLog(ctx, "add trash schedule for %s, %s", poolName, r.config.ScheduleSpec)
		loc := time.Now().Location()
		next = schedule.Next(time.Now().In(loc))
		r.poolMap[poolName] = next
	}

	log.UsefulLog(ctx, "next trash schedule for %s, %v", poolName, next)

	if next.Before(time.Now()) {
		log.UsefulLog(context.TODO(), "start trash purge %s %s", poolName, monitors)
		args := buildTrashPurgeArgs(poolName, monitors, cr)
		log.UsefulLog(context.TODO(), "trash purge: %v", args)
		cmd := exec.Command("bash", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("rbd: could not purge the pool %v cmd %v output: %s, err: %s, exit code: %d",
				poolName, args, string(out), err.Error(), cmd.ProcessState.ExitCode())
			log.ErrorLogMsg(err.Error())
			if cmd.ProcessState.ExitCode() != 2 {
				return reconcile.Result{}, err
			} else {
				return reconcile.Result{}, nil
			}
		}
		loc := time.Now().Location()
		next = schedule.Next(time.Now().In(loc))
		r.poolMap[poolName] = next
	}
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

type CephPredicate struct {
}

func (c *CephPredicate) Create(event event.CreateEvent) bool {
	sc, ok := event.Object.(*scv1.StorageClass)
	if !ok {
		return false
	}
	return sc.Provisioner == rbdDefaultName
}

func (c *CephPredicate) Delete(event event.DeleteEvent) bool {
	sc, ok := event.Object.(*scv1.StorageClass)
	if !ok {
		return false
	}
	return sc.Provisioner == rbdDefaultName
}

func (c *CephPredicate) Update(event event.UpdateEvent) bool {
	sc, ok := event.ObjectNew.(*scv1.StorageClass)
	if !ok {
		return false
	}
	return sc.Provisioner == rbdDefaultName
}

func (c *CephPredicate) Generic(event event.GenericEvent) bool {
	sc, ok := event.Object.(*scv1.StorageClass)
	if !ok {
		return false
	}
	return sc.Provisioner == rbdDefaultName
}

func buildTrashPurgeArgs(pool string, monitor string, cr *util.Credentials) []string {
	cmd := fmt.Sprintf("%s %s --id %s --keyfile=%s -m %s %s", utils.RBDVolCmd, utils.RBDTrashPurgeArg,
		cr.ID, cr.KeyFile, monitor, pool)
	var RBDPurgeArg []string
	RBDPurgeArg = append(RBDPurgeArg, "-c", cmd)
	return RBDPurgeArg
}
