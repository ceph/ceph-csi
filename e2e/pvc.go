package e2e

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
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
	pv := &v1.PersistentVolume{}
	var err error
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	if timeout == 0 {
		return nil
	}
	name := pvc.Name
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be in Bound state", pvc)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("waiting for PVC %s (%d seconds elapsed)", pvc.Name, int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting pvc in namespace: '%s': %v", pvc.Namespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		if pvc.Spec.VolumeName == "" {
			return false, nil
		}

		pv, err = c.CoreV1().PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if apierrs.IsNotFound(err) {
			return false, nil
		}
		err = e2epv.WaitOnPVandPVC(c, pvc.Namespace, pv, pvc)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
}

func createPVCAndPV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume) error {
	_, err := c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	_, err = c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	return err
}

func deletePVCAndPV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume, t int) error {
	err := c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	err = c.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	timeout := time.Duration(t) * time.Minute
	start := time.Now()

	pvcToDelete := pvc
	err = wait.PollImmediate(poll, timeout, func() (bool, error) {
		// Check that the PVC is deleted.
		e2elog.Logf("waiting for PVC %s in state %s to be deleted (%d seconds elapsed)", pvcToDelete.Name, pvcToDelete.Status.String(), int(time.Since(start).Seconds()))
		pvcToDelete, err = c.CoreV1().PersistentVolumeClaims(pvcToDelete.Namespace).Get(context.TODO(), pvcToDelete.Name, metav1.GetOptions{})
		if err == nil {
			if pvcToDelete.Status.Phase == "" {
				// this is unexpected, an empty Phase is not defined
				// FIXME: see https://github.com/ceph/ceph-csi/issues/1874
				e2elog.Logf("PVC %s is in a weird state: %s", pvcToDelete.Name, pvcToDelete.String())
			}
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted PVC %v failed with error other than \"not found\": %w", pvc.Name, err)
		}

		return true, nil
	})
	if err != nil {
		return err
	}

	start = time.Now()
	pvToDelete := pv
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		// Check that the PV is deleted.
		e2elog.Logf("waiting for PV %s in state %s to be deleted (%d seconds elapsed)", pvToDelete.Name, pvToDelete.Status.String(), int(time.Since(start).Seconds()))

		pvToDelete, err = c.CoreV1().PersistentVolumes().Get(context.TODO(), pvToDelete.Name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}

		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("delete PV %v failed with error other than \"not found\": %w", pv.Name, err)
		}

		return true, nil
	})
}

func getPVCAndPV(c kubernetes.Interface, pvcName, pvcNamespace string) (*v1.PersistentVolumeClaim, *v1.PersistentVolume, error) {
	pvc, err := c.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get PVC with error %v", err)
	}
	pv, err := c.CoreV1().PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return pvc, nil, fmt.Errorf("failed to delete PV with error %v", err)
	}
	return pvc, pv, nil
}

func deletePVCAndValidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, t int) error {
	timeout := time.Duration(t) * time.Minute
	nameSpace := pvc.Namespace
	name := pvc.Name
	var err error
	e2elog.Logf("Deleting PersistentVolumeClaim %v on namespace %v", name, nameSpace)

	pvc, err = c.CoreV1().PersistentVolumeClaims(nameSpace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	pv, err := c.CoreV1().PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumeClaims(nameSpace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete of PVC %v failed: %w", name, err)
	}
	start := time.Now()
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		// Check that the PVC is really deleted.
		e2elog.Logf("waiting for PVC %s in state %s to be deleted (%d seconds elapsed)", name, pvc.Status.String(), int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(nameSpace).Get(context.TODO(), name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted PVC %v failed with error other than \"not found\": %w", name, err)
		}

		// Examine the pv.ClaimRef and UID. Expect nil values.
		_, err = c.CoreV1().PersistentVolumes().Get(context.TODO(), pv.Name, metav1.GetOptions{})
		if err == nil {
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
	claim, err := client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvc.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Get the bound PV
	pv, err := client.CoreV1().PersistentVolumes().Get(context.TODO(), claim.Spec.VolumeName, metav1.GetOptions{})
	return pv, err
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
