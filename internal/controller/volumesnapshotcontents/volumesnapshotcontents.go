package volumesnapshotcontents

import (
	"context"
	"fmt"
	"github.com/ceph/ceph-csi/internal/util/log"
	"time"

	"github.com/ceph/ceph-csi/internal/rbd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/controller/utils"
	"github.com/ceph/ceph-csi/internal/util"

	volsnapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
)

const (
	needToUpdateLabel = "ecx.ctcdn.cn/need-to-update"
	snapshotSizeLabel = "ecx.ctcdn.cn/snapshot-size"
)

type ReconcileVolumeSnapshotContents struct {
	client client.Client
	config ctrl.Config
	locks  *util.VolumeLocks
}

// Init will add the ReconcileVolumeSnapshotContents to the list.
func Init() {
	// add ReconcileVolumeSnapshotContents to the list
	ctrl.ControllerList = append(ctrl.ControllerList, &ReconcileVolumeSnapshotContents{})
}

func (r *ReconcileVolumeSnapshotContents) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newVolumeSnapshotContentsReconciler(mgr, config))
}

func newVolumeSnapshotContentsReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcileVolumeSnapshotContents{
		client: mgr.GetClient(),
		config: config,
	}
	return r
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(
		"volumesnapshotcontents-controller",
		mgr,
		controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to VolumeSnapshotContents
	err = c.Watch(&source.Kind{Type: &volsnapv1.VolumeSnapshotContent{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	return nil
}

func (r *ReconcileVolumeSnapshotContents) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// 是否计算屏蔽快照大小
	if r.config.DisableSnapSize {
		return reconcile.Result{}, nil
	}
	volSnapshotContent := &volsnapv1.VolumeSnapshotContent{}
	err := r.client.Get(ctx, request.NamespacedName, volSnapshotContent)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}
	// Check if the object is under deletion
	if !volSnapshotContent.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	if volSnapshotContent.Status == nil {
		return reconcile.Result{}, nil
	}
	if volSnapshotContent.Status.ReadyToUse == nil {
		return reconcile.Result{}, nil
	}
	if *volSnapshotContent.Status.ReadyToUse != true {
		return reconcile.Result{}, nil
	}
	if volSnapshotContent.Spec.Source.VolumeHandle == nil {
		log.DebugLog(ctx, "%s volSnapshotContent.Spec.Source.VolumeHandle is empty", request.NamespacedName)
		return reconcile.Result{}, nil
	}
	if volSnapshotContent.Status.SnapshotHandle == nil {
		log.DebugLog(ctx, "%s volSnapshotContent.Status.SnapshotHandle is empty", request.NamespacedName)
		return reconcile.Result{}, nil
	}
	if _, ok := volSnapshotContent.Labels[needToUpdateLabel]; !ok {
		return reconcile.Result{}, nil
	}
	if volSnapshotContent.Labels[needToUpdateLabel] == "false" {
		return reconcile.Result{}, nil
	}
	volID := *volSnapshotContent.Spec.Source.VolumeHandle
	snapID := *volSnapshotContent.Status.SnapshotHandle
	log.DebugLog(ctx, "%s begin to update snap size, vol:%s snap:%s", request.NamespacedName, volID, snapID)
	cr, err := utils.GetCredentials(ctx, r.client, r.config.SecretName, r.config.SecretNamespace)
	if err != nil {
		log.ErrorLogMsg("%s failed to get credentials from secret err:%s", request.NamespacedName, err.Error())
		return reconcile.Result{}, nil
	}
	defer cr.DeleteCredentials()
	monitors, _, err := util.FetchMappedClusterIDAndMons(ctx, r.config.ClusterId)
	if err != nil {
		log.ErrorLog(ctx, "%s FetchMappedClusterIDAndMons failed, err:%s", request.NamespacedName, err.Error())
		return reconcile.Result{}, err
	}
	//  根据volume id 生成 vol
	var vi util.CSIIdentifier
	err = vi.DecomposeCSIID(volID)
	if err != nil {
		log.ErrorLog(ctx, "%s error decoding volume ID (%s) (%s)", request.NamespacedName, err, volID)
		return reconcile.Result{}, nil
	}
	pool, err := util.GetPoolName(monitors, cr, vi.LocationID)
	if err != nil {
		log.ErrorLog(ctx, "%s GetPoolName failed (%s) (%s)", request.NamespacedName, err, volID)
		return reconcile.Result{}, nil
	}
	vol := rbd.NewRbdVol(pool, monitors, r.config.ClusterId, "csi-vol-"+vi.ObjectUUID)
	//  根据snap id  获取 snapshotName
	var snapvi util.CSIIdentifier
	err = snapvi.DecomposeCSIID(snapID)
	if err != nil {
		log.ErrorLog(ctx, "error decoding snapshot ID (%s) (%s)", err, volID)
		return reconcile.Result{}, nil
	}
	snapshotName := "csi-snap-" + snapvi.ObjectUUID
	// 更新大小
	if err = rbd.UpdateSize(ctx, vol, cr, snapshotName); err != nil {
		log.ErrorLog(ctx, "%s UpdateSize failed, err:%s", request.NamespacedName, err.Error())
		return reconcile.Result{RequeueAfter: 30 * time.Second}, err
	}

	return reconcile.Result{}, nil
}
