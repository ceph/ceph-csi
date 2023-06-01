/*
Copyright 2020 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package persistentvolume

import (
	"context"
	"errors"
	"fmt"

	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ReconcilePersistentVolume reconciles a PersistentVolume object.
type ReconcilePersistentVolume struct {
	client client.Client
	config ctrl.Config
	Locks  *util.VolumeLocks
}

var (
	_ reconcile.Reconciler = &ReconcilePersistentVolume{}
	_ ctrl.Manager         = &ReconcilePersistentVolume{}
)

// Init will add the ReconcilePersistentVolume to the list.
func Init() {
	// add ReconcilePersistentVolume to the list
	ctrl.ControllerList = append(ctrl.ControllerList, &ReconcilePersistentVolume{})
}

// Add adds the newPVReconciler.
func (r *ReconcilePersistentVolume) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newPVReconciler(mgr, config))
}

// newReconciler returns a ReconcilePersistentVolume.
func newPVReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcilePersistentVolume{
		client: mgr.GetClient(),
		config: config,
		Locks:  util.NewVolumeLocks(),
	}

	return r
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(
		"persistentvolume-controller",
		mgr,
		controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to PersistentVolumes
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.PersistentVolume{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	return nil
}

func (r *ReconcilePersistentVolume) getCredentials(
	ctx context.Context,
	name,
	namespace string,
) (*util.Credentials, error) {
	var cr *util.Credentials

	if name == "" || namespace == "" {
		errStr := "secret name or secret namespace is empty"
		log.ErrorLogMsg(errStr)

		return nil, errors.New(errStr)
	}
	secret := &corev1.Secret{}
	err := r.client.Get(ctx,
		types.NamespacedName{Name: name, Namespace: namespace},
		secret)
	if err != nil {
		return nil, fmt.Errorf("error getting secret %s in namespace %s: %w", name, namespace, err)
	}

	credentials := map[string]string{}
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}

	cr, err = util.NewUserCredentials(credentials)
	if err != nil {
		log.ErrorLogMsg("failed to get user credentials %s", err)

		return nil, err
	}

	return cr, nil
}

func checkStaticVolume(pv *corev1.PersistentVolume) bool {
	return pv.Spec.CSI.VolumeAttributes["staticVolume"] == "true"
}

// reconcilePV will extract the image details from the pv spec and regenerates
// the omap data.
func (r *ReconcilePersistentVolume) reconcilePV(ctx context.Context, obj runtime.Object) error {
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		return nil
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != r.config.DriverName {
		return nil
	}
	// PV is not attached to any PVC
	if pv.Spec.ClaimRef == nil {
		return nil
	}

	pvcNamespace := pv.Spec.ClaimRef.Namespace
	requestName := pv.Name
	volumeHandler := pv.Spec.CSI.VolumeHandle
	secretName := ""
	secretNamespace := ""
	// check static volume
	static := checkStaticVolume(pv)
	// if the volume is static, dont generate OMAP data
	if static {
		return nil
	}
	if pv.Spec.CSI.ControllerExpandSecretRef != nil {
		secretName = pv.Spec.CSI.ControllerExpandSecretRef.Name
		secretNamespace = pv.Spec.CSI.ControllerExpandSecretRef.Namespace
	} else if pv.Spec.CSI.NodeStageSecretRef != nil {
		secretName = pv.Spec.CSI.NodeStageSecretRef.Name
		secretNamespace = pv.Spec.CSI.NodeStageSecretRef.Namespace
	}

	// Take lock to process only one volumeHandle at a time.
	if ok := r.Locks.TryAcquire(pv.Spec.CSI.VolumeHandle); !ok {
		return fmt.Errorf(util.VolumeOperationAlreadyExistsFmt, pv.Spec.CSI.VolumeHandle)
	}
	defer r.Locks.Release(pv.Spec.CSI.VolumeHandle)

	cr, err := r.getCredentials(ctx, secretName, secretNamespace)
	if err != nil {
		log.ErrorLogMsg("failed to get credentials from secret %s", err)

		return err
	}
	defer cr.DeleteCredentials()

	rbdVolID, err := rbd.RegenerateJournal(
		pv.Spec.CSI.VolumeAttributes,
		pv.Spec.ClaimRef.Name,
		volumeHandler,
		requestName,
		pvcNamespace,
		r.config.ClusterName,
		r.config.SetMetadata,
		cr)
	if err != nil {
		log.ErrorLogMsg("failed to regenerate journal %s", err)

		return err
	}
	if rbdVolID != volumeHandler {
		log.DebugLog(ctx, "volumeHandler changed from %s to %s", volumeHandler, rbdVolID)
	}

	return nil
}

// Reconcile reconciles the PersistentVolume object and creates a new omap entries
// for the volume.
func (r *ReconcilePersistentVolume) Reconcile(ctx context.Context,
	request reconcile.Request,
) (reconcile.Result, error) {
	pv := &corev1.PersistentVolume{}
	err := r.client.Get(ctx, request.NamespacedName, pv)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}
	// Check if the object is under deletion
	if !pv.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	err = r.reconcilePV(ctx, pv)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
