package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	apierrs "k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/client/conditions"
	"k8s.io/kubernetes/test/e2e/framework"
	testutils "k8s.io/kubernetes/test/utils"
)

var ns = "rook-ceph"

func getCephfsTemp() []os.FileInfo {
	file, err := ioutil.ReadDir(cephfsDirPath)
	if err != nil {
		framework.ExpectNoError(err)
	}
	return file
}

// How often to Poll pods, nodes and claims.
var poll = 2 * time.Second

func waitForDaemonSets(name, ns string, c clientset.Interface, timeout time.Duration) error {
	start := time.Now()
	framework.Logf("Waiting up to %v for all daemonsets in namespace '%s' to start",
		timeout, ns)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		ds, err := c.AppsV1().DaemonSets(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting daemonsets in namespace: '%s': %v", ns, err)
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			if testutils.IsRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}

		framework.Logf("%d / %d pods ready in namespace '%s' in daemonset '%s' (%d seconds elapsed)", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled, ns, ds.ObjectMeta.Name, int(time.Since(start).Seconds()))
		if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
			return false, nil
		}

		return true, nil
	})
}

// Waits for the deployment to complete.

func waitForDeploymentComplete(name, ns string, c clientset.Interface, pOut time.Duration) error {
	var (
		deployment *apps.Deployment
		reason     string
	)

	err := wait.PollImmediate(poll, pOut, func() (bool, error) {
		var err error
		deployment, err = c.AppsV1().Deployments(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		//TODO need to check rolling update

		// When the deployment status and its underlying resources reach the
		// desired state, we're done
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}

		reason = fmt.Sprintf("deployment status: %#v", deployment.Status)
		framework.Logf(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %v", name, err)
	}
	return nil
}

func getAdminCreds(f *framework.Framework, c string) string {

	ns := "rook-ceph"
	cmd := []string{"/bin/sh", "-c", c}
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	podList, err := f.PodClientNS(ns).List(opt)
	framework.ExpectNoError(err)
	Expect(podList.Items).NotTo(BeNil())
	Expect(err).Should(BeNil())

	podPot := framework.ExecOptions{
		Command:            cmd,
		PodName:            podList.Items[0].Name,
		Namespace:          ns,
		ContainerName:      podList.Items[0].Spec.Containers[0].Name,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	Expect(err).Should(BeNil())
	Expect(stdErr).Should(BeEmpty())
	return stdOut
}

func getMons(ns string, c kubernetes.Interface) []string {
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	svcList, err := c.CoreV1().Services(ns).List(opt)
	Expect(err).Should(BeNil())
	services := make([]string, 0)
	for _, svc := range svcList.Items {
		s := fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, svc.Spec.Ports[0].Port)
		services = append(services, s)
	}
	return services
}

func createStorageClass(c kubernetes.Interface) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "storageclass.yaml")
	sc := scv1.StorageClass{}
	err := unmarshal(scPath, &sc)
	Expect(err).Should(BeNil())

	mons := getMons(ns, c)
	sc.Parameters["monitors"] = strings.Join(mons, ",")
	_, err = c.StorageV1().StorageClasses().Create(&sc)
	Expect(err).Should(BeNil())
}

func createSecret(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "secret.yaml")
	sc := v1.Secret{}
	err := unmarshal(scPath, &sc)
	Expect(err).Should(BeNil())

	adminID := getAdminCreds(f, "echo -n admin|base64")
	adminKey := getAdminCreds(f, "ceph auth get-key client.admin|base64")
	sc.Data["adminID"] = []byte(adminID)
	sc.Data["adminKey"] = []byte(adminKey)
	delete(sc.Data, "userID")
	delete(sc.Data, "userKey")
	_, err = c.CoreV1().Secrets("default").Create(&sc)
	Expect(err).Should(BeNil())
}

func deleteSecret() {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "secret.yaml")
	framework.RunKubectl("delete", "-f", scPath)
}

func deleteSc() {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "storageclass.yaml")
	framework.RunKubectl("delete", "-f", scPath)
}

func loadPVC(path string) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{}
	err := unmarshal(path, &pvc)
	if err != nil {
		return nil
	}
	return pvc
}

func createPVCAndvalidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, timeout time.Duration) error {
	pv := &v1.PersistentVolume{}
	var err error
	_, err = c.CoreV1().PersistentVolumeClaims("default").Create(pvc)
	Expect(err).Should(BeNil())
	name := pvc.Name
	start := time.Now()
	framework.Logf("Waiting up to %v to be in Bound state", pvc)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		framework.Logf("waiting for PVC %s (%d seconds elapsed)", pvc.Name, int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting pvc in namespace: '%s': %v", ns, err)
			if testutils.IsRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		pv, err = c.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if apierrs.IsNotFound(err) {
			return false, nil
		}
		return true, nil
	})

	err = framework.WaitOnPVandPVC(c, ns, pv, pvc)
	return err
}

func deletePVCAndValidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, timeout time.Duration) error {
	framework.Logf("Deleting PersistentVolumeClaim %v to trigger PV Recycling", pvc.Name)

	pv, err := c.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumeClaims(ns).Delete(pvc.Name, &metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Delete of PVC %v failed: %v", pvc.Name, err)
	}

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		// Check that the PVC is really deleted.
		pvc, err = c.CoreV1().PersistentVolumeClaims(ns).Get(pvc.Name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("Get on deleted PVC %v failed with error other than \"not found\": %v", pvc.Name, err)
		}

		// Examine the pv.ClaimRef and UID. Expect nil values.
		pv, err = c.CoreV1().PersistentVolumes().Get(pv.Name, metav1.GetOptions{})
		if err == nil {
			return false, err
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("deleted PV %v failed with error other than \"not found\": %v", pv.Name, err)
		}
		if pv.Spec.ClaimRef != nil && len(pv.Spec.ClaimRef.UID) > 0 {
			crJSON, _ := json.Marshal(pv.Spec.ClaimRef)
			return false, fmt.Errorf("Expected PV %v's ClaimRef to be nil, or the claimRef's UID to be blank. Instead claimRef is: %v", pv.Name, string(crJSON))
		}
		return true, nil
	})

	return nil

}

func loadApp(path string) *v1.Pod {
	app := v1.Pod{}
	err := unmarshal(path, &app)
	if err != nil {
		return nil
	}
	return &app
}
func createApp(c kubernetes.Interface, app *v1.Pod, timeout time.Duration) error {
	_, err := c.CoreV1().Pods("default").Create(app)
	if err != nil {
		return err
	}
	return waitForPodInRunningState(app.Name, app.Namespace, c, timeout)
}

func getPodName(ns string, c kubernetes.Interface, opt metav1.ListOptions) string {

	ticker := time.NewTicker(1 * time.Second)
	//TODO add stop logic
	for {
		select {
		case <-ticker.C:
			podList, err := c.CoreV1().Pods(ns).List(opt)
			framework.ExpectNoError(err)
			Expect(podList.Items).NotTo(BeNil())
			Expect(err).Should(BeNil())

			if len(podList.Items) != 0 {
				return podList.Items[0].Name
			}
		}

	}
}
func waitForPodInRunningState(name, ns string, c kubernetes.Interface, timeout time.Duration) error {
	start := time.Now()
	framework.Logf("Waiting up to %v to be in Running state", name)
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		pod, err := c.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case v1.PodRunning:
			return true, nil
		case v1.PodFailed, v1.PodSucceeded:
			return false, conditions.ErrPodCompleted
		}
		framework.Logf("%s app  is  in %s phase expected to be in Running  state (%d seconds elapsed)", name, pod.Status.Phase, int(time.Since(start).Seconds()))
		return false, nil
	})
}

func deletePod(name, ns string, c kubernetes.Interface, timeout time.Duration) error {
	err := c.CoreV1().Pods(ns).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	start := time.Now()
	framework.Logf("Waiting for pod %v to be deleted", name)
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := c.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})

		if apierrs.IsNotFound(err) {
			return true, nil
		}
		framework.Logf("%s app  to be deleted (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
		if err != nil {
			return false, err
		}
		return false, nil
	})
}
func unmarshal(fileName string, obj interface{}) error {
	f, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	data, err := utilyaml.ToJSON([]byte(f))
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, obj)
	return err
}

//TODO what should be the count of mon pods?
//how to get the mon count for pod validation?
func checkMonPods(ns string, c kubernetes.Interface, count int, timeout time.Duration, opt metav1.ListOptions) error {
	start := time.Now()

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		podList, err := c.CoreV1().Pods(ns).List(opt)
		if err != nil {
			return false, err
		}

		framework.Logf("mon pod  count is %d  expected count %d (%d seconds elapsed)", len(podList.Items), count, int(time.Since(start).Seconds()))

		if len(podList.Items) == count {
			return true, nil
		}

		return false, nil
	})

}
