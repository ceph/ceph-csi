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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	ctrl "github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"

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

// Lock to update the configmap.
var mapLock sync.Mutex

const (
	volumeIDMappingConfigMap = "ceph-csi-volumeid-mapping"
	volumeIDMappingkey       = "mapping.json"
)

// ReconcilePersistentVolume reconciles a PersistentVolume object.
type ReconcilePersistentVolume struct {
	client client.Client
	config ctrl.Config
}

var _ reconcile.Reconciler = &ReconcilePersistentVolume{}
var _ ctrl.ContollerManager = &ReconcilePersistentVolume{}

// Init will add the ReconcilePersistentVolume to the list.
func Init() {
	// add ReconcilePersistentVolume to the list
	ctrl.ControllerList = append(ctrl.ControllerList, ReconcilePersistentVolume{})
}

// Add adds the newPVReconciler.
func (r ReconcilePersistentVolume) Add(mgr manager.Manager, config ctrl.Config) error {
	return add(mgr, newPVReconciler(mgr, config))
}

// newReconciler returns a ReconcilePersistentVolume.
func newPVReconciler(mgr manager.Manager, config ctrl.Config) reconcile.Reconciler {
	r := &ReconcilePersistentVolume{
		client: mgr.GetClient(),
		config: config,
	}
	return r
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("persistentvolume-controller", mgr, controller.Options{MaxConcurrentReconciles: 1, Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to PersistentVolumes
	err = c.Watch(&source.Kind{Type: &corev1.PersistentVolume{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcilePersistentVolume) getCredentials(name, namespace string) (*util.Credentials, error) {
	var cr *util.Credentials

	if name == "" || namespace == "" {
		errStr := "secret name or secret namespace is empty"
		util.ErrorLogMsg(errStr)
		return nil, errors.New(errStr)
	}
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("error getting secret %s in namespace %s: %w", name, namespace, err)
	}

	credentials := map[string]string{}
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}

	cr, err = util.NewUserCredentials(credentials)
	if err != nil {
		util.ErrorLogMsg("failed to get user credentials %s", err)
		return nil, err
	}
	return cr, nil
}

func checkStaticVolume(pv *corev1.PersistentVolume) (bool, error) {
	static := false
	var err error

	staticVol := pv.Spec.CSI.VolumeAttributes["staticVolume"]
	if staticVol != "" {
		static, err = strconv.ParseBool(staticVol)
		if err != nil {
			return false, fmt.Errorf("failed to parse preProvisionedVolume: %w", err)
		}
	}
	return static, nil
}

// reconcilePV will extract the image details from the pv spec and regenerates
// the omap data.
func (r ReconcilePersistentVolume) reconcilePV(obj runtime.Object) error {
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		return nil
	}
	if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == r.config.DriverName {
		pool := pv.Spec.CSI.VolumeAttributes["pool"]
		journalPool := pv.Spec.CSI.VolumeAttributes["journalPool"]
		requestName := pv.Name
		imageName := pv.Spec.CSI.VolumeAttributes["imageName"]
		volumeHandler := pv.Spec.CSI.VolumeHandle
		secretName := ""
		secretNamespace := ""
		// check static volume
		static, err := checkStaticVolume(pv)
		if err != nil {
			return err
		}
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

		cr, err := r.getCredentials(secretName, secretNamespace)
		if err != nil {
			util.ErrorLogMsg("failed to get credentials %s", err)
			return err
		}
		defer cr.DeleteCredentials()

		rbdVolID, err := rbd.RegenerateJournal(imageName, volumeHandler, pool, journalPool, requestName, cr)
		if err != nil {
			util.ErrorLogMsg("failed to regenerate journal %s", err)
			return err
		}
		if rbdVolID != volumeHandler {
			err = r.storeVolumeIDMapping(pv.Name, volumeHandler, rbdVolID)
			if err != nil {
				util.ErrorLogMsg("failed to store volumeID mapping %s", err)
				return err
			}
		}
		// start cleanup goroutine
		go r.cleanupStaleVolumeIDMapping()
	}
	return nil
}

// removeStaleVolumeIDMapping removes the stale volumeID mapping from the
// Volume slice when the corresponding PV does not exists anymore.
func (r ReconcilePersistentVolume) removeStaleVolumeIDMapping(cm *corev1.ConfigMap) (*[]journal.Volume, error) {
	volData := []journal.Volume{}
	err := json.Unmarshal([]byte(cm.Data[volumeIDMappingkey]), &volData)
	if err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	for i, c := range volData {
		if c.DriverName != r.config.DriverName {
			continue
		}
		pv := &corev1.PersistentVolume{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: c.PVName}, pv)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// if the PV is not found remove entry
				if i > len(volData) {
					copy(volData[i:], volData[i+1:])
				}
				volData[len(volData)-1] = journal.Volume{}
				volData = volData[:len(volData)-1]
			}
		}
	}
	return &volData, nil
}

// cleanupStaleVolumeIDMapping starts a periodic timer to cleanup the stale
// volumeID mapping from the configmap.
func (r ReconcilePersistentVolume) cleanupStaleVolumeIDMapping() {
	ticker := time.NewTicker(10 * time.Second) // nolint:gomnd // number specifies time.
	defer ticker.Stop()
	for {
		select { // nolint:gosimple // currently only single case is present.
		// TODO add code for signal handling to exit from infinite loop.
		case <-ticker.C:
			// Take a lock and remove stale entry from configmap
			mapLock.Lock()
			cm, err := r.fetchvolumeIDMappingConfigMap()
			if err == nil {
				volData, err := r.removeStaleVolumeIDMapping(cm)
				if err != nil {
					util.ErrorLogMsg("failed to remove stale volumeID mapping: %s", err)
				}
				err = r.updateVolumeIDMappingConfigMap(volData, cm)
				if err != nil {
					util.ErrorLogMsg("configmap update failed: %s", err)
				}
			}
		}
		// Release lock after updating configmap
		mapLock.Unlock()
	}
}

// fetchvolumeIDMappingConfigMap fetches the configmap which contains the
// volumeID mapping.
func (r ReconcilePersistentVolume) fetchvolumeIDMappingConfigMap() (*corev1.ConfigMap, error) {
	nsReq := types.NamespacedName{
		Namespace: r.config.Namespace,
		Name:      volumeIDMappingConfigMap,
	}
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), nsReq, cm)
	if err != nil {
		return cm, fmt.Errorf("configmap get failed: %w", err)
	}
	return cm, nil
}

// updateVolumeIDMappingConfigMap updates the configmap with latest volumeID
// mappings.
func (r ReconcilePersistentVolume) updateVolumeIDMappingConfigMap(volData *[]journal.Volume, cm *corev1.ConfigMap) error {
	upData, err := json.Marshal(*volData)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}
	cm.Data[volumeIDMappingkey] = string(upData)
	err = r.client.Update(context.TODO(), cm)
	if err != nil {
		return fmt.Errorf("configmap update failed: %w", err)
	}
	return nil
}

// storeVolumeIDMapping stores the new volumeID mapping in the configmap if it
// does not exists.
func (r ReconcilePersistentVolume) storeVolumeIDMapping(pvName, existingVolumeID, newVolumeID string) error {
	// take a lock to avoid concurrent update of configmap
	mapLock.Lock()
	defer mapLock.Unlock()
	vol := journal.Volume{
		ExistringVolumeID: existingVolumeID,
		NewVolumeID:       newVolumeID,
		PVName:            pvName,
		DriverName:        r.config.DriverName,
	}
	cm, err := r.fetchvolumeIDMappingConfigMap()
	if err != nil {
		return fmt.Errorf("configmap get failed: %w", err)
	}
	volData := []journal.Volume{}
	err = json.Unmarshal([]byte(cm.Data[volumeIDMappingkey]), &volData)
	if err != nil {
		return fmt.Errorf("json unmarshal failed: %w", err)
	}
	for _, v := range volData {
		if v.ExistringVolumeID == existingVolumeID {
			return nil
		}
	}
	volData = append(volData, vol)
	return r.updateVolumeIDMappingConfigMap(&volData, cm)
}

// Reconcile reconciles the PersitentVolume object and creates a new omap entries
// for the volume.
func (r *ReconcilePersistentVolume) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	pv := &corev1.PersistentVolume{}
	err := r.client.Get(context.TODO(), request.NamespacedName, pv)
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

	err = r.reconcilePV(pv)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
