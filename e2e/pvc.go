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
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epv "k8s.io/kubernetes/test/e2e/framework/pv"
)

func loadPVC(path string) (*v1.PersistentVolumeClaim, error) {
	pvc := &v1.PersistentVolumeClaim{}
	err := unmarshal(path, &pvc)
	if err != nil {
		return nil, err
	}

	return pvc, err
}

func createPVCAndvalidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, t int) error {
	timeout := time.Duration(t) * time.Minute
	ctx := context.TODO()
	pv := &v1.PersistentVolume{}
	var err error
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pvc: %w", err)
	}
	if timeout == 0 {
		return nil
	}
	name := pvc.Name
	namespace := pvc.Namespace
	start := time.Now()
	framework.Logf("Waiting up to %v to be in Bound state", pvc)

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		framework.Logf("waiting for PVC %s (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting pvc %q in namespace %q: %v", name, namespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get pvc: %w", err)
		}

		if pvc.Spec.VolumeName == "" {
			return false, nil
		}

		pv, err = c.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get pv: %w", err)
		}
		err = e2epv.WaitOnPVandPVC(
			ctx,
			c,
			&framework.TimeoutContext{ClaimBound: timeout, PVBound: timeout},
			namespace,
			pv,
			pvc)
		if err != nil {
			return false, fmt.Errorf("failed to wait for the pv and pvc to bind: %w", err)
		}

		return true, nil
	})
}

func createPVCAndPV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume) error {
	_, err := c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pvc: %w", err)
	}
	_, err = c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pv: %w", err)
	}

	return err
}

func deletePVCAndPV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume, t int) error {
	ctx := context.TODO()
	err := c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pvc: %w", err)
	}
	err = c.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pv: %w", err)
	}

	timeout := time.Duration(t) * time.Minute
	start := time.Now()

	pvcToDelete := pvc
	err = wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		// Check that the PVC is deleted.
		framework.Logf(
			"waiting for PVC %s in state %s to be deleted (%d seconds elapsed)",
			pvcToDelete.Name,
			pvcToDelete.Status.String(),
			int(time.Since(start).Seconds()))
		pvcToDelete, err = c.CoreV1().
			PersistentVolumeClaims(pvc.Namespace).
			Get(ctx, pvc.Name, metav1.GetOptions{})
		if err == nil {
			if pvcToDelete.Status.Phase == "" {
				// this is unexpected, an empty Phase is not defined
				// FIXME: see https://github.com/ceph/ceph-csi/issues/1874
				framework.Logf("PVC %s is in a weird state: %s", pvcToDelete.Name, pvcToDelete.String())
			}

			return false, nil
		}
		if isRetryableAPIError(err) {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf(
				"get on deleted PVC %v failed with error other than \"not found\": %w",
				pvc.Name,
				err)
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to poll: %w", err)
	}

	start = time.Now()
	pvToDelete := pv

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		// Check that the PV is deleted.
		framework.Logf(
			"waiting for PV %s in state %s to be deleted (%d seconds elapsed)",
			pvToDelete.Name,
			pvToDelete.Status.String(),
			int(time.Since(start).Seconds()))

		pvToDelete, err = c.CoreV1().PersistentVolumes().Get(ctx, pv.Name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if isRetryableAPIError(err) {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("delete PV %v failed with error other than \"not found\": %w", pv.Name, err)
		}

		return true, nil
	})
}

// getPersistentVolumeClaim returns the PersistentVolumeClaim with the given
// name in the given namespace and retries if there is any API error.
func getPersistentVolumeClaim(c kubernetes.Interface, namespace, name string) (*v1.PersistentVolumeClaim, error) {
	var pvc *v1.PersistentVolumeClaim
	var err error
	timeout := time.Duration(deployTimeout) * time.Minute
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		1*time.Second,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			pvc, err = c.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				framework.Logf("Error getting pvc %q in namespace %q: %v", name, namespace, err)
				if isRetryableAPIError(err) {
					return false, nil
				}

				return false, fmt.Errorf("failed to get pvc: %w", err)
			}

			return true, err
		})

	return pvc, err
}

// getPersistentVolume returns the PersistentVolume with the given
// name and retries if there is any API error.
func getPersistentVolume(c kubernetes.Interface, name string) (*v1.PersistentVolume, error) {
	var pv *v1.PersistentVolume
	var err error
	timeout := time.Duration(deployTimeout) * time.Minute
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		1*time.Second,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			pv, err = c.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				framework.Logf("Error getting pv %q: %v", name, err)
				if isRetryableAPIError(err) {
					return false, nil
				}

				return false, fmt.Errorf("failed to get pv: %w", err)
			}

			return true, err
		})

	return pv, err
}

func getPVCAndPV(
	c kubernetes.Interface,
	pvcName, pvcNamespace string,
) (*v1.PersistentVolumeClaim, *v1.PersistentVolume, error) {
	pvc, err := getPersistentVolumeClaim(c, pvcNamespace, pvcName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get PVC: %w", err)
	}
	pv, err := getPersistentVolume(c, pvc.Spec.VolumeName)
	if err != nil {
		return pvc, nil, fmt.Errorf("failed to get PV: %w", err)
	}

	return pvc, pv, nil
}

func deletePVCAndValidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, t int) error {
	timeout := time.Duration(t) * time.Minute
	nameSpace := pvc.Namespace
	name := pvc.Name
	ctx := context.TODO()
	var err error
	framework.Logf("Deleting PersistentVolumeClaim %v on namespace %v", name, nameSpace)

	pvc, err = getPersistentVolumeClaim(c, nameSpace, name)
	if err != nil {
		return fmt.Errorf("failed to get pvc: %w", err)
	}
	pv, err := getPersistentVolume(c, pvc.Spec.VolumeName)
	if err != nil {
		return fmt.Errorf("failed to get pv: %w", err)
	}

	err = c.CoreV1().PersistentVolumeClaims(nameSpace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete of PVC %v failed: %w", name, err)
	}
	start := time.Now()

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		// Check that the PVC is really deleted.
		framework.Logf(
			"waiting for PVC %s in state %s to be deleted (%d seconds elapsed)",
			name,
			pvc.Status.String(),
			int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(nameSpace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			framework.Logf("PVC %s (status: %s) has not been deleted yet, rechecking...", name, pvc.Status)

			return false, nil
		}
		if isRetryableAPIError(err) {
			framework.Logf("failed to verify deletion of PVC %s (status: %s): %v", name, pvc.Status, err)

			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted PVC %v failed with error other than \"not found\": %w", name, err)
		}

		// Examine the pv.ClaimRef and UID. Expect nil values.
		oldPV, err := c.CoreV1().PersistentVolumes().Get(ctx, pv.Name, metav1.GetOptions{})
		if err == nil {
			framework.Logf("PV %s (status: %s) has not been deleted yet, rechecking...", pv.Name, oldPV.Status)

			return false, nil
		}
		if isRetryableAPIError(err) {
			framework.Logf("failed to verify deletion of PV %s (status: %s): %v", pv.Name, oldPV.Status, err)

			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("delete PV %v failed with error other than \"not found\": %w", pv.Name, err)
		}

		return true, nil
	})
}

// getBoundPV returns a PV details.
func getBoundPV(client kubernetes.Interface, pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolume, error) {
	// Get new copy of the claim
	claim, err := getPersistentVolumeClaim(client, pvc.Namespace, pvc.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get pvc: %w", err)
	}

	// Get the bound PV
	pv, err := client.CoreV1().PersistentVolumes().Get(context.TODO(), claim.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pv: %w", err)
	}

	return pv, nil
}

func checkPVSelectorValuesForPVC(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	pv, err := getBoundPV(f.ClientSet, pvc)
	if err != nil {
		return err
	}

	if len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms) == 0 {
		return errors.New("found empty NodeSelectorTerms in PV")
	}

	rFound := false
	zFound := false
	for _, expression := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions {
		switch expression.Key {
		case nodeCSIRegionLabel:
			if rFound {
				return errors.New("found multiple occurrences of topology key for region")
			}
			rFound = true
			if expression.Values[0] != regionValue {
				return errors.New("topology value for region label mismatch")
			}
		case nodeCSIZoneLabel:
			if zFound {
				return errors.New("found multiple occurrences of topology key for zone")
			}
			zFound = true
			if expression.Values[0] != zoneValue {
				return errors.New("topology value for zone label mismatch")
			}
		default:
			return errors.New("unexpected key in node selector terms found in PV")
		}
	}

	return nil
}

func getMetricsForPVC(f *framework.Framework, pvc *v1.PersistentVolumeClaim, t int) error {
	kubelet, err := getKubeletIP(f.ClientSet)
	if err != nil {
		return err
	}

	// kubelet needs to be started with --read-only-port=10255
	cmd := fmt.Sprintf("curl --silent 'http://%s:10255/metrics'", kubelet)

	// retry as kubelet does not immediately have the metrics available
	timeout := time.Duration(t) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
		if err != nil {
			framework.Logf("failed to get metrics for pvc %q (%v): %v", pvc.Name, err, stdErr)

			return false, nil
		}
		if stdOut == "" {
			framework.Logf("no metrics received from kubelet on IP %s", kubelet)

			return false, nil
		}

		namespace := fmt.Sprintf("namespace=%q", pvc.Namespace)
		name := fmt.Sprintf("persistentvolumeclaim=%q", pvc.Name)

		for _, line := range strings.Split(stdOut, "\n") {
			if !strings.HasPrefix(line, "kubelet_volume_stats_") {
				continue
			}
			if strings.Contains(line, namespace) && strings.Contains(line, name) {
				// TODO: validate metrics if possible
				framework.Logf("found metrics for pvc %s/%s: %s", pvc.Namespace, pvc.Name, line)

				return true, nil
			}
		}

		framework.Logf("no metrics found for pvc %s/%s", pvc.Namespace, pvc.Name)

		return false, nil
	})
}
