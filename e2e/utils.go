package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"k8s.io/klog"

	_ "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"                             // nolint
	_ "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned/typed/volumesnapshot/v1alpha1" // nolint
	. "github.com/onsi/ginkgo"                                                                                      // nolint
	. "github.com/onsi/gomega"                                                                                      // nolint
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/client/conditions"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
	testutils "k8s.io/kubernetes/test/utils"
)

var poll = 2 * time.Second

// type snapInfo struct {
// 	ID        int64  `json:"id"`
// 	Name      string `json:"name"`
// 	Size      int64  `json:"size"`
// 	Timestamp string `json:"timestamp"`
// }

func deployProvAsSTS(c clientset.Interface) bool {
	// kubeMinor to use deployment instead of statefulset for provisioner
	const kubeMinor = "14"
	v, err := c.Discovery().ServerVersion()
	if err != nil {
		klog.Errorf("failed to get server version with error %v", err)
		return false
	}
	if v.Minor < kubeMinor {
		return true
	}
	return false
}

func getKubeVersionToDeploy(c clientset.Interface) string {
	sts := deployProvAsSTS(c)
	version := ""
	if sts {
		version = "v1.13"
	} else {
		version = "v1.14+"
	}
	return version
}

func waitForDaemonSets(name, ns string, c clientset.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()
	e2elog.Logf("Waiting up to %v for all daemonsets in namespace '%s' to start",
		timeout, ns)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		ds, err := c.AppsV1().DaemonSets(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting daemonsets in namespace: '%s': %v", ns, err)
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
		e2elog.Logf("%d / %d pods ready in namespace '%s' in daemonset '%s' (%d seconds elapsed)", ready, dNum, ns, ds.ObjectMeta.Name, int(time.Since(start).Seconds()))
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

		// TODO need to check rolling update

		// When the deployment status and its underlying resources reach the
		// desired state, we're done
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}

		reason = fmt.Sprintf("deployment status: %#v", deployment.Status.String())
		e2elog.Logf(reason)

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

func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string) {

	cmd := []string{"/bin/sh", "-c", c}
	podList, err := f.PodClientNS(ns).List(*opt)
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
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	Expect(err).Should(BeNil())
	return stdOut, stdErr
}

func getMons(ns string, c kubernetes.Interface) []string {
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	svcList, err := c.CoreV1().Services(ns).List(opt)
	Expect(err).Should(BeNil())
	services := make([]string, 0)
	for i := range svcList.Items {
		s := fmt.Sprintf("%s.%s.svc.cluster.local:%d", svcList.Items[i].Name, svcList.Items[i].Namespace, svcList.Items[i].Spec.Ports[0].Port)
		services = append(services, s)
	}
	return services
}

func getStorageClass(path string) scv1.StorageClass {
	sc := scv1.StorageClass{}
	err := unmarshal(path, &sc)
	Expect(err).Should(BeNil())
	return sc
}

// func getSnapshotClass(path string) v1alpha1.VolumeSnapshotClass {
// 	sc := v1alpha1.VolumeSnapshotClass{}
// 	sc.Kind = "VolumeSnapshotClass"
// 	sc.APIVersion = "snapshot.storage.k8s.io/v1alpha1"
// 	err := unmarshal(path, &sc)
// 	Expect(err).Should(BeNil())
// 	return sc
// }

// func getSnapshot(path string) v1alpha1.VolumeSnapshot {
// 	sc := v1alpha1.VolumeSnapshot{}
// 	err := unmarshal(path, &sc)
// 	Expect(err).Should(BeNil())
// 	return sc
// }

func createCephfsStorageClass(c kubernetes.Interface, f *framework.Framework, enablePool bool) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "storageclass.yaml")
	sc := getStorageClass(scPath)
	sc.Parameters["fsName"] = "myfs"
	if enablePool {
		sc.Parameters["pool"] = "myfs-data0"
	}
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	fsID, stdErr := execCommandInPod(f, "ceph fsid", rookNS, &opt)
	Expect(stdErr).Should(BeEmpty())
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	sc.Parameters["clusterID"] = fsID
	_, err := c.StorageV1().StorageClasses().Create(&sc)
	Expect(err).Should(BeNil())
}

func createRBDStorageClass(c kubernetes.Interface, f *framework.Framework, parameters map[string]string) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "storageclass.yaml")
	sc := getStorageClass(scPath)
	sc.Parameters["pool"] = "replicapool"
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	fsID, stdErr := execCommandInPod(f, "ceph fsid", rookNS, &opt)
	Expect(stdErr).Should(BeEmpty())
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	sc.Parameters["clusterID"] = fsID
	for k, v := range parameters {
		sc.Parameters[k] = v
	}
	_, err := c.StorageV1().StorageClasses().Create(&sc)
	Expect(err).Should(BeNil())
}

// func newSnapshotClient() (*snapClient.SnapshotV1alpha1Client, error) {
// 	config, err := framework.LoadConfig()
// 	if err != nil {
// 		return nil, fmt.Errorf("error creating client: %v", err.Error())
// 	}
// 	c, err := snapClient.NewForConfig(config)
// 	if err != nil {
// 		return nil, fmt.Errorf("error creating snapshot client: %v", err.Error())
// 	}
// 	return c, err
// }
// func createRBDSnapshotClass(f *framework.Framework) {
// 	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "snapshotclass.yaml")
// 	sc := getSnapshotClass(scPath)

// 	opt := metav1.ListOptions{
// 		LabelSelector: "app=rook-ceph-tools",
// 	}
// 	fsID := execCommandInPod(f, "ceph fsid", rookNS, &opt)
// 	// remove new line present in fsID
// 	fsID = strings.Trim(fsID, "\n")
// 	sc.Parameters["clusterID"] = fsID
// 	sclient, err := newSnapshotClient()
// 	Expect(err).Should(BeNil())
// 	_, err = sclient.VolumeSnapshotClasses().Create(&sc)
// 	Expect(err).Should(BeNil())
// }

func deleteConfiMap(pluginPath string) {
	path := pluginPath + configMap
	_, err := framework.RunKubectl("delete", "-f", path)
	if err != nil {
		e2elog.Logf("failed to delete configmap %v", err)
	}
}

func createConfigMap(pluginPath string, c kubernetes.Interface, f *framework.Framework) {
	path := pluginPath + configMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	Expect(err).Should(BeNil())
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	fsID, stdErr := execCommandInPod(f, "ceph fsid", rookNS, &opt)
	Expect(stdErr).Should(BeEmpty())
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	// get mon list
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
	// discard corruptInputError
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
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	adminKey, stdErr := execCommandInPod(f, "ceph auth get-key client.admin", rookNS, &opt)
	Expect(stdErr).Should(BeEmpty())
	sc.StringData["adminID"] = "admin"
	sc.StringData["adminKey"] = adminKey
	delete(sc.StringData, "userID")
	delete(sc.StringData, "userKey")
	_, err := c.CoreV1().Secrets("default").Create(&sc)
	Expect(err).Should(BeNil())
}

func createRBDSecret(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "secret.yaml")
	sc := getSecret(scPath)
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	adminKey, stdErr := execCommandInPod(f, "ceph auth get-key client.admin", rookNS, &opt)
	Expect(stdErr).Should(BeEmpty())
	sc.StringData["userID"] = "admin"
	sc.StringData["userKey"] = adminKey
	_, err := c.CoreV1().Secrets("default").Create(&sc)
	Expect(err).Should(BeNil())
}

func deleteResource(scPath string) {
	_, err := framework.RunKubectl("delete", "-f", scPath)
	Expect(err).Should(BeNil())
}

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
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(pvc)
	Expect(err).Should(BeNil())
	name := pvc.Name
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be in Bound state", pvc)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("waiting for PVC %s (%d seconds elapsed)", pvc.Name, int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting pvc in namespace: '%s': %v", pvc.Namespace, err)
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
	e2elog.Logf("Deleting PersistentVolumeClaim %v on namespace %v", name, nameSpace)

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
		e2elog.Logf("waiting for PVC %s in state %s  to be deleted (%d seconds elapsed)", name, pvc.Status.String(), int(time.Since(start).Seconds()))
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

func loadApp(path string) (*v1.Pod, error) {
	app := v1.Pod{}
	err := unmarshal(path, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

func createApp(c kubernetes.Interface, app *v1.Pod, timeout int) error {
	_, err := c.CoreV1().Pods(app.Namespace).Create(app)
	if err != nil {
		return err
	}
	return waitForPodInRunningState(app.Name, app.Namespace, c, timeout)
}

func getPodName(ns string, c kubernetes.Interface, opt *metav1.ListOptions) string {
	ticker := time.NewTicker(1 * time.Second)
	// TODO add stop logic
	for range ticker.C {
		podList, err := c.CoreV1().Pods(ns).List(*opt)
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
	e2elog.Logf("Waiting up to %v to be in Running state", name)
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
		e2elog.Logf("%s app  is  in %s phase expected to be in Running  state (%d seconds elapsed)", name, pod.Status.Phase, int(time.Since(start).Seconds()))
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
	e2elog.Logf("Waiting for pod %v to be deleted", name)
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := c.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})

		if apierrs.IsNotFound(err) {
			return true, nil
		}
		e2elog.Logf("%s app  to be deleted (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
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

func checkCephPods(ns string, c kubernetes.Interface, count, t int, opt *metav1.ListOptions) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		podList, err := c.CoreV1().Pods(ns).List(*opt)
		if err != nil {
			return false, err
		}

		e2elog.Logf("pod count is %d  expected count %d (%d seconds elapsed)", len(podList.Items), count, int(time.Since(start).Seconds()))

		if len(podList.Items) >= count {
			return true, nil
		}

		return false, nil
	})

}

// createPVCAndApp creates pvc and pod
// if name is not empty same will be set as pvc and app name
func createPVCAndApp(name string, f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error {

	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}
	err := createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}
	err = createApp(f.ClientSet, app, deployTimeout)
	return err
}

// deletePVCAndApp delete pvc and pod
// if name is not empty same will be set as pvc and app name
func deletePVCAndApp(name string, f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error {

	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}

	err := deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	return err
}

func validatePVCAndAppBinding(pvcPath, appPath string, f *framework.Framework) {
	pvc, err := loadPVC(pvcPath)
	if pvc == nil {
		Fail(err.Error())
	}
	pvc.Namespace = f.UniqueName
	e2elog.Logf("The PVC  template %+v", pvc)

	app, err := loadApp(appPath)
	if err != nil {
		Fail(err.Error())
	}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}
}

func validateNormalUserPVCAccess(pvcPath string, f *framework.Framework) {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		Fail(err.Error())
	}
	pvc.Namespace = f.UniqueName
	pvc.Name = f.UniqueName
	e2elog.Logf("The PVC  template %+v", pvc)
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}
	var user int64 = 2000
	app := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-run-as-non-root",
			Namespace: f.UniqueName,
			Labels: map[string]string{
				"app": "pod-run-as-non-root",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "write-pod",
					Image:   "alpine",
					Command: []string{"/bin/sleep", "999999"},
					SecurityContext: &v1.SecurityContext{
						RunAsUser: &user,
					},
					VolumeMounts: []v1.VolumeMount{
						{
							MountPath: "/target",
							Name:      "target",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "target",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
							ReadOnly:  false},
					},
				},
			},
		},
	}

	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=pod-run-as-non-root",
	}
	execCommandInPod(f, "echo testing > /target/testing", app.Namespace, &opt)

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}

	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		Fail(err.Error())
	}
}

// func createSnapshot(snap *v1alpha1.VolumeSnapshot, t int) error {

// 	sclient, err := newSnapshotClient()
// 	if err != nil {
// 		return err
// 	}
// 	_, err = sclient.VolumeSnapshots(snap.Namespace).Create(snap)
// 	if err != nil {
// 		return err
// 	}
// 	e2elog.Logf("snapshot with name %v created in %v namespace", snap.Name, snap.Namespace)

// 	timeout := time.Duration(t) * time.Minute
// 	name := snap.Name
// 	start := time.Now()
// 	e2elog.Logf("Waiting up to %v to be in Ready state", snap)

// 	return wait.PollImmediate(poll, timeout, func() (bool, error) {
// 		e2elog.Logf("waiting for snapshot %s (%d seconds elapsed)", snap.Name, int(time.Since(start).Seconds()))
// 		snaps, err := sclient.VolumeSnapshots(snap.Namespace).Get(name, metav1.GetOptions{})
// 		if err != nil {
// 			e2elog.Logf("Error getting snapshot in namespace: '%s': %v", snap.Namespace, err)
// 			if testutils.IsRetryableAPIError(err) {
// 				return false, nil
// 			}
// 			if apierrs.IsNotFound(err) {
// 				return false, nil
// 			}
// 			return false, err
// 		}
// 		if snaps.Status.ReadyToUse {
// 			return true, nil
// 		}
// 		return false, nil
// 	})
// }

// func deleteSnapshot(snap *v1alpha1.VolumeSnapshot, t int) error {
// 	sclient, err := newSnapshotClient()
// 	if err != nil {
// 		return err
// 	}
// 	err = sclient.VolumeSnapshots(snap.Namespace).Delete(snap.Name, &metav1.DeleteOptions{})
// 	if err != nil {
// 		return err
// 	}

// 	timeout := time.Duration(t) * time.Minute
// 	name := snap.Name
// 	start := time.Now()
// 	e2elog.Logf("Waiting up to %v to be deleted", snap)

// 	return wait.PollImmediate(poll, timeout, func() (bool, error) {
// 		e2elog.Logf("deleting snapshot %s (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
// 		_, err := sclient.VolumeSnapshots(snap.Namespace).Get(name, metav1.GetOptions{})
// 		if err == nil {
// 			return false, nil
// 		}

// 		if !apierrs.IsNotFound(err) {
// 			return false, fmt.Errorf("get on deleted snapshot %v failed with error other than \"not found\": %v", name, err)
// 		}

// 		return true, nil
// 	})
// }

func listRBDImages(f *framework.Framework) []string {

	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	stdout, stdErr := execCommandInPod(f, "rbd ls --pool=replicapool --format=json", rookNS, &opt)
	Expect(stdErr).Should(BeEmpty())
	var imgInfos []string

	err := json.Unmarshal([]byte(stdout), &imgInfos)
	if err != nil {
		Fail(err.Error())
	}
	return imgInfos
}

// func listSnapshots(f *framework.Framework, pool, imageName string) ([]snapInfo, error) {
// 	opt := metav1.ListOptions{
// 		LabelSelector: "app=rook-ceph-tools",
// 	}
// 	command := fmt.Sprintf("rbd snap ls %s/%s --format=json", pool, imageName)
// 	stdout := execCommandInPod(f, command, rookNS, &opt)

// 	var snapInfos []snapInfo

// 	err := json.Unmarshal([]byte(stdout), &snapInfos)
// 	return snapInfos, err
// }

func checkDataPersist(pvcPath, appPath string, f *framework.Framework) error {
	data := "checking data persist"
	pvc, err := loadPVC(pvcPath)
	if pvc == nil {
		return err
	}
	pvc.Namespace = f.UniqueName
	e2elog.Logf("The PVC  template %+v", pvc)

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-data"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=validate-data",
	}
	// write data to PVC
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

	execCommandInPod(f, fmt.Sprintf("echo %s > %s", data, filePath), app.Namespace, &opt)

	// delete app
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}
	// recreate app and check data persist
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	persistData, stdErr := execCommandInPod(f, fmt.Sprintf("cat %s", filePath), app.Namespace, &opt)
	Expect(stdErr).Should(BeEmpty())
	if !strings.Contains(persistData, data) {
		return fmt.Errorf("data not persistent expected data %s received data %s  ", data, persistData)
	}

	err = deletePVCAndApp("", f, pvc, app)
	return err
}
