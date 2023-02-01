/*
Copyright 2021 The Ceph-CSI Authors.

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

package e2e

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

func validateBiggerCloneFromPVC(f *framework.Framework,
	pvcPath,
	appPath,
	pvcClonePath,
	appClonePath string,
) error {
	const (
		size    = "1Gi"
		newSize = "2Gi"
	)
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}
	label := make(map[string]string)
	pvc.Namespace = f.UniqueName
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	app, err := loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app: %w", err)
	}
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}
	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create pvc and application: %w", err)
	}

	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		framework.Failf("failed to load PVC: %v", err)
	}
	pvcClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = pvc.Name
	pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(newSize)
	appClone, err := loadApp(appClonePath)
	if err != nil {
		framework.Failf("failed to load application: %v", err)
	}
	appClone.Namespace = f.UniqueName
	appClone.Labels = label
	err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create pvc clone and application: %w", err)
	}
	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return fmt.Errorf("failed to delete pvc and application: %w", err)
	}
	if pvcClone.Spec.VolumeMode == nil || *pvcClone.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		err = checkDirSize(appClone, f, &opt, newSize)
		if err != nil {
			return err
		}
	}

	if pvcClone.Spec.VolumeMode != nil && *pvcClone.Spec.VolumeMode == v1.PersistentVolumeBlock {
		err = checkDeviceSize(appClone, f, &opt, newSize)
		if err != nil {
			return err
		}
	}
	err = deletePVCAndApp("", f, pvcClone, appClone)
	if err != nil {
		return fmt.Errorf("failed to delete pvc and application: %w", err)
	}

	return nil
}
