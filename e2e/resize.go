package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega" // nolint
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/cloud-provider/volume/helpers"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

func expandPVCSize(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, size string, t int) error {
	pvcName := pvc.Name
	updatedPVC := pvc.DeepCopy()
	var err error

	updatedPVC, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error fetching pvc %q with %w", pvcName, err)
	}
	timeout := time.Duration(t) * time.Minute

	updatedPVC.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	_, err = c.CoreV1().PersistentVolumeClaims(updatedPVC.Namespace).Update(context.TODO(), updatedPVC, metav1.UpdateOptions{})
	Expect(err).Should(BeNil())

	start := time.Now()
	e2elog.Logf("Waiting up to %v to be in Resized state", pvc)
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("waiting for PVC %s (%d seconds elapsed)", updatedPVC.Name, int(time.Since(start).Seconds()))
		updatedPVC, err = c.CoreV1().PersistentVolumeClaims(updatedPVC.Namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting pvc in namespace: '%s': %v", updatedPVC.Namespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}
		pvcConditions := updatedPVC.Status.Conditions
		if len(pvcConditions) > 0 {
			e2elog.Logf("pvc state %v", pvcConditions[0].Type)
			if pvcConditions[0].Type == v1.PersistentVolumeClaimResizing || pvcConditions[0].Type == v1.PersistentVolumeClaimFileSystemResizePending {
				return false, nil
			}
		}

		if !updatedPVC.Status.Capacity[v1.ResourceStorage].Equal(resource.MustParse(size)) {
			e2elog.Logf("current size in status %v,expected size %v", updatedPVC.Status.Capacity[v1.ResourceStorage], resource.MustParse(size))
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

	pvc, err = f.ClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvc.Name, metav1.GetOptions{})
	if err != nil {
		return err
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
	err = waitForPodInRunningState(app.Name, app.Namespace, f.ClientSet, deployTimeout)
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

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("executing cmd %s (%d seconds elapsed)", cmd, int(time.Since(start).Seconds()))
		output, stdErr, err := execCommandInPod(f, cmd, ns, opt)
		if err != nil {
			return false, err
		}
		if stdErr != "" {
			e2elog.Logf("failed to execute command in app pod %v", stdErr)
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
			e2elog.Logf("expected size %s found %s information", size, output)
			return false, nil
		}
		return true, nil
	})
}
