package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	apierrs "k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo" // nolint
	. "github.com/onsi/gomega" // nolint
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

func getFilesinDirectory(path string) []os.FileInfo {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		framework.ExpectNoError(err)
	}
	return files
}

var poll = 2 * time.Second

func waitForDaemonSets(name, ns string, c clientset.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
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
		dNum := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		framework.Logf("%d / %d pods ready in namespace '%s' in daemonset '%s' (%d seconds elapsed)", ready, dNum, ns, ds.ObjectMeta.Name, int(time.Since(start).Seconds()))
		if ready != dNum {
			return false, nil
		}

		return true, nil
	})
}

// Waits for the deployment to complete.

func waitForDeploymentComplete(name, ns string, c clientset.Interface, t int) error {
	var (
		deployment *apps.Deployment
		reason     string
		err        error
	)
	timeout := time.Duration(t) * time.Minute
	err = wait.PollImmediate(poll, timeout, func() (bool, error) {
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

func execCommandInToolBox(f *framework.Framework, c string) string {

	cmd := []string{"/bin/sh", "-c", c}
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	podList, err := f.PodClientNS(rookNS).List(opt)
	framework.ExpectNoError(err)
	Expect(podList.Items).NotTo(BeNil())
	Expect(err).Should(BeNil())

	podPot := framework.ExecOptions{
		Command:            cmd,
		PodName:            podList.Items[0].Name,
		Namespace:          rookNS,
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

func getStorageClass(c kubernetes.Interface, path string) scv1.StorageClass {
	sc := scv1.StorageClass{}
	err := unmarshal(path, &sc)
	Expect(err).Should(BeNil())

	mons := getMons(rookNS, c)
	sc.Parameters["monitors"] = strings.Join(mons, ",")
	return sc
}

func createCephfsStorageClass(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "storageclass.yaml")
	sc := getStorageClass(c, scPath)
	sc.Parameters["pool"] = "myfs-data0"
	sc.Parameters["fsName"] = "myfs"
	fsID := execCommandInToolBox(f, "ceph fsid")
	//remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	sc.Parameters["clusterID"] = fsID
	_, err := c.StorageV1().StorageClasses().Create(&sc)
	Expect(err).Should(BeNil())
}

func createRBDStorageClass(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "storageclass.yaml")
	sc := getStorageClass(c, scPath)
	delete(sc.Parameters, "userid")
	sc.Parameters["pool"] = "replicapool"

	fsID := execCommandInToolBox(f, "ceph fsid")
	//remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	sc.Parameters["clusterID"] = fsID
	_, err := c.StorageV1().StorageClasses().Create(&sc)
	Expect(err).Should(BeNil())
}

func createConfigMap(c kubernetes.Interface, f *framework.Framework) {
	path := rbdDirPath + rbdConfigMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	Expect(err).Should(BeNil())

	fsID := execCommandInToolBox(f, "ceph fsid")
	//remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	//get mon list
	mons := getMons(rookNS, c)
	conmap := []struct {
		Clusterid string   `json:"clusterID"`
		Monitors  []string `json:"monitors"`
	}{
		{
			fsID,
			mons,
		},
	}
	data, err := json.Marshal(conmap)
	Expect(err).Should(BeNil())
	cm.Data["config.json"] = string(data)
	_, err = c.CoreV1().ConfigMaps("default").Create(&cm)
	Expect(err).Should(BeNil())
}

func getSecret(path string) v1.Secret {
	sc := v1.Secret{}
	err := unmarshal(path, &sc)
	//discard corruptInputError
	if err != nil {
		if _, ok := err.(base64.CorruptInputError); !ok {
			Expect(err).Should(BeNil())
		}
	}
	return sc
}

func createCephfsSecret(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "secret.yaml")
	sc := getSecret(scPath)
	adminKey := execCommandInToolBox(f, "ceph auth get-key client.admin")
	sc.Data["adminID"] = []byte("admin")
	sc.Data["adminKey"] = []byte(adminKey)
	delete(sc.Data, "userID")
	delete(sc.Data, "userKey")
	_, err := c.CoreV1().Secrets("default").Create(&sc)
	Expect(err).Should(BeNil())
}

func createRBDSecret(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "secret.yaml")
	sc := getSecret(scPath)
	adminKey := execCommandInToolBox(f, "ceph auth get-key client.admin")
	sc.Data["admin"] = []byte(adminKey)
	delete(sc.Data, "kubernetes")
	_, err := c.CoreV1().Secrets("default").Create(&sc)
	Expect(err).Should(BeNil())
}

func deleteSecret(scPath string) {
	_, err := framework.RunKubectl("delete", "-f", scPath)
	Expect(err).Should(BeNil())
}

func deleteStorageClass(scPath string) {
	_, err := framework.RunKubectl("delete", "-f", scPath)
	Expect(err).Should(BeNil())
}

func loadPVC(path string) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{}
	err := unmarshal(path, &pvc)
	if err != nil {
		return nil
	}
	return pvc
}

func createPVCAndvalidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, t int) error {
	timeout := time.Duration(t) * time.Minute
	pv := &v1.PersistentVolume{}
	var err error
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(pvc)
	Expect(err).Should(BeNil())
	name := pvc.Name
	start := time.Now()
	framework.Logf("Waiting up to %v to be in Bound state", pvc)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		framework.Logf("waiting for PVC %s (%d seconds elapsed)", pvc.Name, int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting pvc in namespace: '%s': %v", pvc.Namespace, err)
			if testutils.IsRetryableAPIError(err) {
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
		pv, err = c.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if apierrs.IsNotFound(err) {
			return false, nil
		}
		err = framework.WaitOnPVandPVC(c, pvc.Namespace, pv, pvc)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
}

func deletePVCAndValidatePV(c kubernetes.Interface, pvc *v1.PersistentVolumeClaim, t int) error {
	timeout := time.Duration(t) * time.Minute
	nameSpace := pvc.Namespace
	name := pvc.Name
	var err error
	framework.Logf("Deleting PersistentVolumeClaim %v on namespace %v", name, nameSpace)

	pvc, err = c.CoreV1().PersistentVolumeClaims(nameSpace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	pv, err := c.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumeClaims(nameSpace).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete of PVC %v failed: %v", name, err)
	}
	start := time.Now()
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		// Check that the PVC is really deleted.
		framework.Logf("waiting for PVC %s in state %s  to be deleted (%d seconds elapsed)", name, pvc.Status.String(), int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(nameSpace).Get(name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted PVC %v failed with error other than \"not found\": %v", name, err)
		}

		// Examine the pv.ClaimRef and UID. Expect nil values.
		_, err = c.CoreV1().PersistentVolumes().Get(pv.Name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}

		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("delete PV %v failed with error other than \"not found\": %v", pv.Name, err)
		}

		return true, nil
	})
}

func loadApp(path string) *v1.Pod {
	app := v1.Pod{}
	err := unmarshal(path, &app)
	if err != nil {
		return nil
	}
	return &app
}

func createApp(c kubernetes.Interface, app *v1.Pod, timeout int) error {
	_, err := c.CoreV1().Pods(app.Namespace).Create(app)
	if err != nil {
		return err
	}
	return waitForPodInRunningState(app.Name, app.Namespace, c, timeout)
}

func getPodName(ns string, c kubernetes.Interface, opt metav1.ListOptions) string {
	ticker := time.NewTicker(1 * time.Second)
	//TODO add stop logic
	for range ticker.C {
		podList, err := c.CoreV1().Pods(ns).List(opt)
		framework.ExpectNoError(err)
		Expect(podList.Items).NotTo(BeNil())
		Expect(err).Should(BeNil())

		if len(podList.Items) != 0 {
			return podList.Items[0].Name
		}
	}
	return ""
}

func waitForPodInRunningState(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
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

func deletePod(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
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
	data, err := utilyaml.ToJSON(f)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, obj)
	return err
}

func checkCephPods(ns string, c kubernetes.Interface, count int, t int, opt metav1.ListOptions) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		podList, err := c.CoreV1().Pods(ns).List(opt)
		if err != nil {
			return false, err
		}

		framework.Logf("pod count is %d  expected count %d (%d seconds elapsed)", len(podList.Items), count, int(time.Since(start).Seconds()))

		if len(podList.Items) >= count {
			return true, nil
		}

		return false, nil
	})

}

func validatePVCAndAppBinding(pvcPath, appPath string, f *framework.Framework) {
	pvc := loadPVC(pvcPath)
	pvc.Namespace = f.UniqueName
	framework.Logf("The PVC  template %+v", pvc)
	err := createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}

	app := loadApp(appPath)
	app.Namespace = f.UniqueName
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}

	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}
}
