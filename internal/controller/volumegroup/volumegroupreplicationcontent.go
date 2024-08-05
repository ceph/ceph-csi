/*
Copyright 2025 The Ceph-CSI Authors.

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
package volumegroup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

type ReconcileVGRContent struct {
	client client.Client
	config ctrl.Config
	Locks  *util.VolumeLocks
}

var (
	_ reconcile.Reconciler = &ReconcileVGRContent{}
	_ ctrl.Manager         = &ReconcileVGRContent{}
)

const (
	secretNameParameterName      = "replication.storage.openshift.io/group-replication-secret-name"
	secretNamespaceParameterName = "replication.storage.openshift.io/group-replication-secret-namespace"

	volumeGroupReplicationContentResourceName = "VolumeGroupReplicationContent"
	volumeGroupReplicationClassResourceName   = "VolumeGroupReplicationClass"
)

// Init will add the ReconcileVGRContent to the list.
func Init() {
	// add ReconcileVGRContent to the list
	ctrl.ControllerList = append(ctrl.ControllerList, &ReconcileVGRContent{})
}

// Add adds the newVGRContentReconciler.
func (r *ReconcileVGRContent) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newVGRContentReconciler(mgr, config))
}

// newVGRContentReconciler returns a ReconcileVGRContent.
func newVGRContentReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcileVGRContent{
		client: mgr.GetClient(),
		config: config,
		Locks:  util.NewVolumeLocks(),
	}

	return r
}

func ensureCRDsInstalled(mgr manager.Manager) (bool, error) {
	crdsInstalled := true
	missingCRDs := []string{}

	gvk := metav1.PartialObjectMetadata{}
	gvk.SetGroupVersionKind(replicationv1alpha1.GroupVersion.WithKind(volumeGroupReplicationContentResourceName))
	_, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupVersionKind().GroupKind(), gvk.GroupVersionKind().Version)
	if err != nil {
		if !meta.IsNoMatchError(err) {
			return false, err
		}

		crdsInstalled = false
		missingCRDs = append(missingCRDs, volumeGroupReplicationContentResourceName)
	}

	gvk.SetGroupVersionKind(replicationv1alpha1.GroupVersion.WithKind(volumeGroupReplicationClassResourceName))
	_, err = mgr.GetRESTMapper().RESTMapping(gvk.GroupVersionKind().GroupKind(), gvk.GroupVersionKind().Version)
	if err != nil {
		if !meta.IsNoMatchError(err) {
			return false, err
		}

		crdsInstalled = false
		missingCRDs = append(missingCRDs, volumeGroupReplicationClassResourceName)
	}

	if !crdsInstalled {
		log.ErrorLogMsg("Required CRDs (%s) are not installed", strings.Join(missingCRDs, ", "))
	}

	return crdsInstalled, nil
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Ensure the required CRDs are installed.
	installed, err := ensureCRDsInstalled(mgr)
	if err != nil {
		return err
	}
	if !installed {
		log.ErrorLogMsg("Skipping controller creation for VolumeGroupReplicationContent. Please install the missing CRDs.")

		return nil
	}

	// Create a new controller
	c, err := controller.New(
		"vgrcontent-controller",
		mgr,
		controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to VolumeGroupReplicationContent
	err = c.Watch(source.Kind(
		mgr.GetCache(),
		&replicationv1alpha1.VolumeGroupReplicationContent{},
		&handler.TypedEnqueueRequestForObject[*replicationv1alpha1.VolumeGroupReplicationContent]{}),
	)
	if err != nil {
		return fmt.Errorf("failed to watch the changes: %w", err)
	}

	return nil
}

func (r *ReconcileVGRContent) getSecrets(
	ctx context.Context,
	name,
	namespace string,
) (map[string]string, error) {
	if name == "" || namespace == "" {
		return nil, errors.New("secret name or secret namespace is empty")
	}
	secret := &corev1.Secret{}
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("error getting secret %s in namespace %s: %w", name, namespace, err)
	}

	secrets := map[string]string{}
	for key, value := range secret.Data {
		secrets[key] = string(value)
	}

	return secrets, nil
}

func (r *ReconcileVGRContent) reconcileVGRContent(ctx context.Context, obj runtime.Object) error {
	vgrc, ok := obj.(*replicationv1alpha1.VolumeGroupReplicationContent)
	if !ok {
		return nil
	}
	if vgrc.Spec.Provisioner != r.config.DriverName {
		return nil
	}

	reqName := vgrc.Name
	groupHandle := vgrc.Spec.VolumeGroupReplicationHandle
	volumeIds := vgrc.Spec.Source.VolumeHandles

	if groupHandle == "" {
		return errors.New("volume group replication handle is empty")
	}

	vgrClass := &replicationv1alpha1.VolumeGroupReplicationClass{}
	err := r.client.Get(ctx, types.NamespacedName{Name: vgrc.Spec.VolumeGroupReplicationClassName}, vgrClass)
	if err != nil {
		return err
	}

	if ok = r.Locks.TryAcquire(groupHandle); !ok {
		return fmt.Errorf("failed to acquire lock for group handle %s", groupHandle)
	}
	defer r.Locks.Release(groupHandle)

	parameters := vgrClass.Spec.Parameters
	secretName := vgrClass.Spec.Parameters[secretNameParameterName]
	secretNamespace := vgrClass.Spec.Parameters[secretNamespaceParameterName]

	secrets, err := r.getSecrets(ctx, secretName, secretNamespace)
	if err != nil {
		return err
	}

	mgr := rbd.NewManager(r.config.InstanceID, parameters, secrets)
	defer mgr.Destroy(ctx)

	groupID, err := mgr.RegenerateVolumeGroupJournal(ctx, groupHandle, reqName, volumeIds)
	if err != nil {
		return err
	}
	if groupID != groupHandle {
		log.DebugLog(ctx, "groupHandle changed from %s to %s", groupHandle, groupID)
	}

	return nil
}

// Reconcile reconciles the VolumeGroupReplicationContent object and creates a new omap entries
// for the volume group.
func (r *ReconcileVGRContent) Reconcile(ctx context.Context,
	request reconcile.Request,
) (reconcile.Result, error) {
	vgrc := &replicationv1alpha1.VolumeGroupReplicationContent{}
	err := r.client.Get(ctx, request.NamespacedName, vgrc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	// Proceed with reconciliation only if the object is not marked for deletion.
	if vgrc.GetDeletionTimestamp().IsZero() {
		err = r.reconcileVGRContent(ctx, vgrc)
	}

	return reconcile.Result{}, err
}
