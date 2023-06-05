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
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint:golint // e2e uses Expect() and other Gomega functions
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/cloud-provider/volume/helpers"
	"k8s.io/kubernetes/test/e2e/framework"
)

func expandPVCSize(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, size string, t int) error {
	ctx := context.TODO()
	pvcName := pvc.Name
	pvcNamespace := pvc.Namespace
	updatedPVC, err := getPersistentVolumeClaim(c, pvcNamespace, pvcName)
	if err != nil {
		return fmt.Errorf("error fetching pvc %q with %w", pvcName, err)
	}
	timeout := time.Duration(t) * time.Minute

	updatedPVC.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	_, err = c.CoreV1().
		PersistentVolumeClaims(updatedPVC.Namespace).
		Update(ctx, updatedPVC, metav1.UpdateOptions{})
	Expect(err).ShouldNot(HaveOccurred())

	start := time.Now()
	framework.Logf("Waiting up to %v to be in Resized state", pvc)

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		framework.Logf("waiting for PVC %s (%d seconds elapsed)", pvcName, int(time.Since(start).Seconds()))
		updatedPVC, err = c.CoreV1().
			PersistentVolumeClaims(pvcNamespace).
			Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting pvc in namespace: '%s': %v", pvcNamespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get pvc: %w", err)
		}
		pvcConditions := updatedPVC.Status.Conditions
		if len(pvcConditions) > 0 {
			framework.Logf("pvc state %v", pvcConditions[0].Type)
			if pvcConditions[0].Type == v1.PersistentVolumeClaimResizing ||
				pvcConditions[0].Type == v1.PersistentVolumeClaimFileSystemResizePending {
				return false, nil
			}
		}

		if !updatedPVC.Status.Capacity[v1.ResourceStorage].Equal(resource.MustParse(size)) {
			framework.Logf(
				"current size in status %v,expected size %v",
				updatedPVC.Status.Capacity[v1.ResourceStorage],
				resource.MustParse(size))

			return false, nil
		}

		return true, nil
	})
}

func resizePVCAndValidateSize(pvcPath, appPath string, f *framework.Framework) error {
	size := "1Gi"
	expandSize := "10Gi"
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName

	resizePvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	resizePvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	app.Labels = map[string]string{"app": "resize-pvc"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=resize-pvc",
	}
	pvc, err = getPersistentVolumeClaim(f.ClientSet, pvc.Namespace, pvc.Name)
	if err != nil {
		return fmt.Errorf("failed to get pvc: %w", err)
	}
	if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		err = checkDirSize(app, f, &opt, size)
		if err != nil {
			return err
		}
	}

	if *pvc.Spec.VolumeMode == v1.PersistentVolumeBlock {
		err = checkDeviceSize(app, f, &opt, size)
		if err != nil {
			return err
		}
	}
	// resize PVC
	err = expandPVCSize(f.ClientSet, resizePvc, expandSize, deployTimeout)
	if err != nil {
		return err
	}
	// wait for application pod to come up after resize
	err = waitForPodInRunningState(app.Name, app.Namespace, f.ClientSet, deployTimeout, noError)
	if err != nil {
		return err
	}
	if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		err = checkDirSize(app, f, &opt, expandSize)
		if err != nil {
			return err
		}
	}

	if *pvc.Spec.VolumeMode == v1.PersistentVolumeBlock {
		err = checkDeviceSize(app, f, &opt, expandSize)
		if err != nil {
			return err
		}
	}
	err = deletePVCAndApp("", f, resizePvc, app)

	return err
}

func checkDirSize(app *v1.Pod, f *framework.Framework, opt *metav1.ListOptions, size string) error {
	cmd := getDirSizeCheckCmd(app.Spec.Containers[0].VolumeMounts[0].MountPath)

	return checkAppMntSize(f, opt, size, cmd, app.Namespace, deployTimeout)
}

func checkDeviceSize(app *v1.Pod, f *framework.Framework, opt *metav1.ListOptions, size string) error {
	cmd := getDeviceSizeCheckCmd(app.Spec.Containers[0].VolumeDevices[0].DevicePath)

	return checkAppMntSize(f, opt, size, cmd, app.Namespace, deployTimeout)
}

func getDirSizeCheckCmd(dirPath string) string {
	return fmt.Sprintf("df -h|grep %s |awk '{print $2}'", dirPath)
}

func getDeviceSizeCheckCmd(devPath string) string {
	return fmt.Sprintf("blockdev --getsize64 %s", devPath)
}

func checkAppMntSize(f *framework.Framework, opt *metav1.ListOptions, size, cmd, ns string, t int) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		framework.Logf("executing cmd %s (%d seconds elapsed)", cmd, int(time.Since(start).Seconds()))
		output, stdErr, err := execCommandInPod(f, cmd, ns, opt)
		if err != nil {
			return false, err
		}
		if stdErr != "" {
			framework.Logf("failed to execute command in app pod %v", stdErr)

			return false, nil
		}
		s := resource.MustParse(strings.TrimSpace(output))
		actualSize, err := helpers.RoundUpToGiB(s)
		if err != nil {
			return false, err
		}
		s = resource.MustParse(size)
		expectedSize, err := helpers.RoundUpToGiB(s)
		if err != nil {
			return false, err
		}
		if actualSize != expectedSize {
			framework.Logf("expected size %s found %s information", size, output)

			return false, nil
		}

		return true, nil
	})
}
