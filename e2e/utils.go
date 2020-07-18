package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	. "github.com/onsi/ginkgo" // nolint
	. "github.com/onsi/gomega" // nolint
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/client/conditions"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
	e2epv "k8s.io/kubernetes/test/e2e/framework/pv"
	testutils "k8s.io/kubernetes/test/utils"
)

const (
	defaultNs     = "default"
	vaultSecretNs = "/secret/ceph-csi/" // nolint: gosec

	// rook created cephfs user
	cephfsNodePluginSecretName  = "rook-csi-cephfs-node"        // nolint: gosec
	cephfsProvisionerSecretName = "rook-csi-cephfs-provisioner" // nolint: gosec

	// rook created rbd user
	rbdNodePluginSecretName  = "rook-csi-rbd-node"        // nolint: gosec
	rbdProvisionerSecretName = "rook-csi-rbd-provisioner" // nolint: gosec

	rookTolBoxPodLabel = "app=rook-ceph-tools"
	rbdmountOptions    = "mountOptions"
)

var (
	// cli flags
	deployTimeout    int
	deployCephFS     bool
	deployRBD        bool
	testCephFS       bool
	testRBD          bool
	cephCSINamespace string
	rookNamespace    string
	ns               string
	vaultAddr        string
	poll             = 2 * time.Second
)

func initResouces() {
	ns = fmt.Sprintf("--namespace=%v", cephCSINamespace)
	vaultAddr = fmt.Sprintf("http://vault.%s.svc.cluster.local:8200", cephCSINamespace)
}

func createNamespace(c kubernetes.Interface, name string) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err := c.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil && !apierrs.IsAlreadyExists(err) {
		return err
	}

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := c.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting namespace: '%s': %v", name, err)
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			if testutils.IsRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func deleteNamespace(c kubernetes.Interface, name string) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	err := c.CoreV1().Namespaces().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		Fail(err.Error())
	}
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err = c.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			e2elog.Logf("Error getting namespace: '%s': %v", name, err)
			if testutils.IsRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}
		return false, nil
	})
}

func replaceNamespaceInTemplate(filePath string) (string, error) {
	read, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(read), "namespace: default", fmt.Sprintf("namespace: %s", cephCSINamespace)), nil
}

func waitForDaemonSets(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()
	e2elog.Logf("Waiting up to %v for all daemonsets in namespace '%s' to start", timeout, ns)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		ds, err := c.AppsV1().DaemonSets(ns).Get(context.TODO(), name, metav1.GetOptions{})
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

func waitForDeploymentComplete(name, ns string, c kubernetes.Interface, t int) error {
	var (
		deployment *apps.Deployment
		reason     string
		err        error
	)
	timeout := time.Duration(t) * time.Minute
	err = wait.PollImmediate(poll, timeout, func() (bool, error) {
		deployment, err = c.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// TODO need to check rolling update

		// When the deployment status and its underlying resources reach the
		// desired state, we're done
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}
		e2elog.Logf("deployment status: expected replica count %d running replica count %d", deployment.Status.Replicas, deployment.Status.ReadyReplicas)
		reason = fmt.Sprintf("deployment status: %#v", deployment.Status.String())
		return false, nil
	})

	if errors.Is(err, wait.ErrWaitTimeout) {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %v", name, err)
	}
	return nil
}

func getCommandInPodOpts(f *framework.Framework, c, ns string, opt *metav1.ListOptions) framework.ExecOptions {
	cmd := []string{"/bin/sh", "-c", c}
	podList, err := f.PodClientNS(ns).List(context.TODO(), *opt)
	framework.ExpectNoError(err)
	Expect(podList.Items).NotTo(BeNil())
	Expect(err).Should(BeNil())

	return framework.ExecOptions{
		Command:            cmd,
		PodName:            podList.Items[0].Name,
		Namespace:          ns,
		ContainerName:      podList.Items[0].Spec.Containers[0].Name,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}
}

func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string) {
	podPot := getCommandInPodOpts(f, c, ns, opt)
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	Expect(err).Should(BeNil())
	return stdOut, stdErr
}

func execCommandInToolBoxPod(f *framework.Framework, c, ns string) (string, string) {
	opt := &metav1.ListOptions{
		LabelSelector: rookTolBoxPodLabel,
	}
	podPot := getCommandInPodOpts(f, c, ns, opt)
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	Expect(err).Should(BeNil())
	return stdOut, stdErr
}

func execCommandInPodAndAllowFail(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string) {
	podPot := getCommandInPodOpts(f, c, ns, opt)
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if err != nil {
		e2elog.Logf("command %s failed: %v", c, err)
	}
	return stdOut, stdErr
}

func getMons(ns string, c kubernetes.Interface) []string {
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	svcList, err := c.CoreV1().Services(ns).List(context.TODO(), opt)
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

func createCephfsStorageClass(c kubernetes.Interface, f *framework.Framework, enablePool bool, clusterID string) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "storageclass.yaml")
	sc := getStorageClass(scPath)
	sc.Parameters["fsName"] = "myfs"
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = cephfsProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = cephfsProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = cephfsNodePluginSecretName

	if enablePool {
		sc.Parameters["pool"] = "myfs-data0"
	}
	fsID, stdErr := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	Expect(stdErr).Should(BeEmpty())
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	if clusterID != "" {
		fsID = clusterID
	}
	sc.Namespace = cephCSINamespace
	sc.Parameters["clusterID"] = fsID
	_, err := c.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	Expect(err).Should(BeNil())
}

func createRBDStorageClass(c kubernetes.Interface, f *framework.Framework, scOptions, parameters map[string]string) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "storageclass.yaml")
	sc := getStorageClass(scPath)
	sc.Parameters["pool"] = defaultRBDPool
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

	fsID, stdErr := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	Expect(stdErr).Should(BeEmpty())
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	sc.Parameters["clusterID"] = fsID
	for k, v := range parameters {
		sc.Parameters[k] = v
	}
	sc.Namespace = cephCSINamespace

	if scOptions["volumeBindingMode"] == "WaitForFirstConsumer" {
		value := scv1.VolumeBindingWaitForFirstConsumer
		sc.VolumeBindingMode = &value
	}

	// comma separated mount options
	if opt, ok := scOptions[rbdmountOptions]; ok {
		mOpt := strings.Split(opt, ",")
		sc.MountOptions = append(sc.MountOptions, mOpt...)
	}
	_, err := c.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	Expect(err).Should(BeNil())
}

func deleteConfigMap(pluginPath string) {
	path := pluginPath + configMap
	_, err := framework.RunKubectl(cephCSINamespace, "delete", "-f", path, ns)
	if err != nil {
		e2elog.Logf("failed to delete configmap %v", err)
	}
}

func createConfigMap(pluginPath string, c kubernetes.Interface, f *framework.Framework) {
	path := pluginPath + configMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	Expect(err).Should(BeNil())

	fsID, stdErr := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	Expect(stdErr).Should(BeEmpty())
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	// get mon list
	mons := getMons(rookNamespace, c)
	conmap := []util.ClusterInfo{{
		ClusterID: fsID,
		Monitors:  mons,
	}}
	conmap[0].CephFS.SubvolumeGroup = "e2e"
	data, err := json.Marshal(conmap)
	Expect(err).Should(BeNil())
	cm.Data["config.json"] = string(data)
	cm.Namespace = cephCSINamespace
	// if the configmap is present update it,during cephcsi helm charts
	// deployment empty configmap gets created we need to override it
	_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Get(context.TODO(), cm.Name, metav1.GetOptions{})

	if err == nil {
		_, updateErr := c.CoreV1().ConfigMaps(cephCSINamespace).Update(context.TODO(), &cm, metav1.UpdateOptions{})
		Expect(updateErr).Should(BeNil())
	}
	if apierrs.IsNotFound(err) {
		_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Create(context.TODO(), &cm, metav1.CreateOptions{})
	}

	Expect(err).Should(BeNil())
}

func getSecret(path string) v1.Secret {
	sc := v1.Secret{}
	err := unmarshal(path, &sc)
	// discard corruptInputError
	if err != nil {
		var b64cie base64.CorruptInputError
		if !errors.As(err, &b64cie) {
			Expect(err).Should(BeNil())
		}
	}
	return sc
}

func createCephfsSecret(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "secret.yaml")
	sc := getSecret(scPath)

	adminKey, stdErr := execCommandInToolBoxPod(f, "ceph auth get-key client.admin", rookNamespace)
	Expect(stdErr).Should(BeEmpty())
	sc.StringData["adminID"] = "admin"
	sc.StringData["adminKey"] = adminKey
	delete(sc.StringData, "userID")
	delete(sc.StringData, "userKey")
	sc.Namespace = cephCSINamespace
	_, err := c.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sc, metav1.CreateOptions{})
	Expect(err).Should(BeNil())
}

func createRBDSecret(c kubernetes.Interface, f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "secret.yaml")
	sc := getSecret(scPath)

	adminKey, stdErr := execCommandInToolBoxPod(f, "ceph auth get-key client.admin", rookNamespace)
	Expect(stdErr).Should(BeEmpty())
	sc.StringData["userID"] = "admin"
	sc.StringData["userKey"] = adminKey
	sc.Namespace = cephCSINamespace
	_, err := c.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sc, metav1.CreateOptions{})
	Expect(err).Should(BeNil())

	err = updateSecretForEncryption(c)
	Expect(err).Should(BeNil())
}

// updateSecretForEncryption is an hack to update the secrets created by rook to
// include the encyption key
// TODO in cephcsi we need to create own users in ceph cluster and use it for E2E
func updateSecretForEncryption(c kubernetes.Interface) error {
	secrets, err := c.CoreV1().Secrets(rookNamespace).Get(context.TODO(), rbdProvisionerSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	secrets.Data["encryptionPassphrase"] = []byte("test_passphrase")

	_, err = c.CoreV1().Secrets(rookNamespace).Update(context.TODO(), secrets, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	secrets, err = c.CoreV1().Secrets(rookNamespace).Get(context.TODO(), rbdNodePluginSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	secrets.Data["encryptionPassphrase"] = []byte("test_passphrase")

	_, err = c.CoreV1().Secrets(rookNamespace).Update(context.TODO(), secrets, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func deleteResource(scPath string) {
	data, err := replaceNamespaceInTemplate(scPath)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", scPath, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Logf("failed to delete %s %v", scPath, err)
	}
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
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	Expect(err).Should(BeNil())
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
		return fmt.Errorf("delete of PVC %v failed: %v", name, err)
	}
	start := time.Now()
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		// Check that the PVC is really deleted.
		e2elog.Logf("waiting for PVC %s in state %s  to be deleted (%d seconds elapsed)", name, pvc.Status.String(), int(time.Since(start).Seconds()))
		pvc, err = c.CoreV1().PersistentVolumeClaims(nameSpace).Get(context.TODO(), name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted PVC %v failed with error other than \"not found\": %v", name, err)
		}

		// Examine the pv.ClaimRef and UID. Expect nil values.
		_, err = c.CoreV1().PersistentVolumes().Get(context.TODO(), pv.Name, metav1.GetOptions{})
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
	_, err := c.CoreV1().Pods(app.Namespace).Create(context.TODO(), app, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return waitForPodInRunningState(app.Name, app.Namespace, c, timeout)
}

func waitForPodInRunningState(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be in Running state", name)
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		pod, err := c.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case v1.PodRunning:
			return true, nil
		case v1.PodFailed, v1.PodSucceeded:
			return false, conditions.ErrPodCompleted
		}
		e2elog.Logf("%s app  is in %s phase expected to be in Running  state (%d seconds elapsed)", name, pod.Status.Phase, int(time.Since(start).Seconds()))
		return false, nil
	})
}

func deletePod(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
	err := c.CoreV1().Pods(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	start := time.Now()
	e2elog.Logf("Waiting for pod %v to be deleted", name)
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := c.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})

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

// createPVCAndApp creates pvc and pod
// if name is not empty same will be set as pvc and app name
func createPVCAndApp(name string, f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod, pvcTimeout int) error {
	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}
	err := createPVCAndvalidatePV(f.ClientSet, pvc, pvcTimeout)
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

func createPVCAndAppBinding(pvcPath, appPath string, f *framework.Framework, pvcTimeout int) (*v1.PersistentVolumeClaim, *v1.Pod) {
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

	err = createPVCAndApp("", f, pvc, app, pvcTimeout)
	if err != nil {
		Fail(err.Error())
	}

	return pvc, app
}

func validatePVCAndAppBinding(pvcPath, appPath string, f *framework.Framework) {
	pvc, app := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	err := deletePVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}
}

type imageInfoFromPVC struct {
	imageID         string
	imageName       string
	csiVolumeHandle string
	pvName          string
}

// getImageInfoFromPVC reads volume handle of the bound PV to the passed in PVC,
// and returns imageInfoFromPVC or error
func getImageInfoFromPVC(pvcNamespace, pvcName string, f *framework.Framework) (imageInfoFromPVC, error) {
	var imageData imageInfoFromPVC

	c := f.ClientSet.CoreV1()
	pvc, err := c.PersistentVolumeClaims(pvcNamespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return imageData, err
	}

	pv, err := c.PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return imageData, err
	}

	imageIDRegex := regexp.MustCompile(`(\w+\-?){5}$`)
	imageID := imageIDRegex.FindString(pv.Spec.CSI.VolumeHandle)

	imageData = imageInfoFromPVC{
		imageID:         imageID,
		imageName:       fmt.Sprintf("csi-vol-%s", imageID),
		csiVolumeHandle: pv.Spec.CSI.VolumeHandle,
		pvName:          pv.Name,
	}
	return imageData, nil
}

func getImageMeta(rbdImageSpec, metaKey string, f *framework.Framework) (string, error) {
	cmd := fmt.Sprintf("rbd image-meta get %s %s", rbdImageSpec, metaKey)
	stdOut, stdErr := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if stdErr != "" {
		return strings.TrimSpace(stdOut), fmt.Errorf(stdErr)
	}
	return strings.TrimSpace(stdOut), nil
}

func getMountType(appName, appNamespace, mountPath string, f *framework.Framework) (string, error) {
	opt := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", appName).String(),
	}
	cmd := fmt.Sprintf("lsblk -o TYPE,MOUNTPOINT | grep '%s' | awk '{print $1}'", mountPath)
	stdOut, stdErr := execCommandInPod(f, cmd, appNamespace, &opt)
	if stdErr != "" {
		return strings.TrimSpace(stdOut), fmt.Errorf(stdErr)
	}
	return strings.TrimSpace(stdOut), nil
}

// readVaultSecret method will execute few commands to try read the secret for
// specified key from inside the vault container:
//  * authenticate with vault and ignore any stdout (we do not need output)
//  * issue get request for particular key
// resulting in stdOut (first entry in tuple) - output that contains the key
// or stdErr (second entry in tuple) - error getting the key
func readVaultSecret(key string, f *framework.Framework) (string, string) {
	loginCmd := fmt.Sprintf("vault login -address=%s sample_root_token_id > /dev/null", vaultAddr)
	readSecret := fmt.Sprintf("vault kv get -address=%s %s%s", vaultAddr, vaultSecretNs, key)
	cmd := fmt.Sprintf("%s && %s", loginCmd, readSecret)
	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}
	stdOut, stdErr := execCommandInPodAndAllowFail(f, cmd, cephCSINamespace, &opt)
	return strings.TrimSpace(stdOut), strings.TrimSpace(stdErr)
}

func validateEncryptedPVCAndAppBinding(pvcPath, appPath, kms string, f *framework.Framework) {
	pvc, app := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)

	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		Fail(err.Error())
	}
	rbdImageSpec := fmt.Sprintf("%s/%s", defaultRBDPool, imageData.imageName)
	encryptedState, err := getImageMeta(rbdImageSpec, ".rbd.csi.ceph.com/encrypted", f)
	if err != nil {
		Fail(err.Error())
	}
	Expect(encryptedState).To(Equal("encrypted"))

	volumeMountPath := app.Spec.Containers[0].VolumeMounts[0].MountPath
	mountType, err := getMountType(app.Name, app.Namespace, volumeMountPath, f)
	if err != nil {
		Fail(err.Error())
	}
	Expect(mountType).To(Equal("crypt"))

	if kms == "vault" {
		// check new passphrase created
		_, stdErr := readVaultSecret(imageData.csiVolumeHandle, f)
		if stdErr != "" {
			Fail(fmt.Sprintf("failed to read passphrase from vault: %s", stdErr))
		}
	}

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}

	if kms == "vault" {
		// check new passphrase created
		stdOut, _ := readVaultSecret(imageData.csiVolumeHandle, f)
		if stdOut != "" {
			Fail(fmt.Sprintf("passphrase found in vault while should be deleted: %s", stdOut))
		}
	}
}

func deletePodWithLabel(label, ns string, skipNotFound bool) error {
	_, err := framework.RunKubectl(cephCSINamespace, "delete", "po", "-l", label, fmt.Sprintf("--ignore-not-found=%t", skipNotFound), fmt.Sprintf("--namespace=%s", ns))
	if err != nil {
		e2elog.Logf("failed to delete pod %v", err)
	}
	return err
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

func deleteBackingCephFSVolume(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr := execCommandInToolBoxPod(f, "ceph fs subvolume rm myfs "+imageData.imageName+" e2e", rookNamespace)
	Expect(stdErr).Should(BeEmpty())

	if stdErr != "" {
		return fmt.Errorf("error deleting backing volume %s", imageData.imageName)
	}
	return nil
}

func checkSubvolumeSnapshotExists(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr := execCommandInToolBoxPod(f, "ceph fs subvolume snapshot ls myfs "+imageData.imageName+" e2e", rookNamespace)
	Expect(stdErr).Should(BeEmpty())

	if stdErr != "" {
		return fmt.Errorf("error checking backing subvolume %s snapshot", imageData.imageName)
	}
	return nil
}

func listRBDImages(f *framework.Framework) []string {
	stdout, stdErr := execCommandInToolBoxPod(f, fmt.Sprintf("rbd ls --pool=%s --format=json", defaultRBDPool), rookNamespace)
	Expect(stdErr).Should(BeEmpty())
	var imgInfos []string

	err := json.Unmarshal([]byte(stdout), &imgInfos)
	if err != nil {
		Fail(err.Error())
	}
	return imgInfos
}

func checkDataPersist(pvcPath, appPath string, f *framework.Framework) error {
	data := "checking data persist"
	pvc, err := loadPVC(pvcPath)
	if pvc == nil {
		return err
	}

	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-data"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
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

func deleteBackingRBDImage(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("rbd rm %s --pool=%s", imageData.imageName, defaultRBDPool)
	execCommandInToolBoxPod(f, cmd, rookNamespace)
	return nil
}

func deletePool(name string, cephfs bool, f *framework.Framework) {
	var cmds = []string{}
	if cephfs {
		// ceph fs fail
		// ceph fs rm myfs --yes-i-really-mean-it
		// ceph osd pool delete myfs-metadata myfs-metadata
		// --yes-i-really-mean-it
		// ceph osd pool delete myfs-data0 myfs-data0
		// --yes-i-really-mean-it
		cmds = append(cmds, fmt.Sprintf("ceph fs fail %s", name),
			fmt.Sprintf("ceph fs rm %s --yes-i-really-mean-it", name),
			fmt.Sprintf("ceph osd pool delete %s-metadata %s-metadata --yes-i-really-really-mean-it", name, name),
			fmt.Sprintf("ceph osd pool delete %s-data0 %s-data0 --yes-i-really-really-mean-it", name, name))
	} else {
		// ceph osd pool delete replicapool replicapool
		// --yes-i-really-mean-it
		cmds = append(cmds, fmt.Sprintf("ceph osd pool delete %s %s --yes-i-really-really-mean-it", name, name))
	}

	for _, cmd := range cmds {
		// discard stdErr as some commands prints warning in strErr
		execCommandInToolBoxPod(f, cmd, rookNamespace)
	}
}

func pvcDeleteWhenPoolNotFound(pvcPath string, cephfs bool, f *framework.Framework) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName

	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}
	if cephfs {
		err = deleteBackingCephFSVolume(f, pvc)
		if err != nil {
			return err
		}
		// delete cephfs filesystem
		deletePool("myfs", cephfs, f)
	} else {
		err = deleteBackingRBDImage(f, pvc)
		if err != nil {
			return err
		}
		// delete rbd pool
		deletePool(defaultRBDPool, cephfs, f)
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	return err
}

func checkMountOptions(pvcPath, appPath string, f *framework.Framework, mountFlags []string) error {
	pvc, err := loadPVC(pvcPath)
	if pvc == nil {
		return err
	}

	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-mount-opt"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=validate-mount-opt",
	}

	cmd := fmt.Sprintf("mount |grep %s", app.Spec.Containers[0].VolumeMounts[0].MountPath)
	data, stdErr := execCommandInPod(f, cmd, app.Namespace, &opt)
	Expect(stdErr).Should(BeEmpty())
	for _, f := range mountFlags {
		if !strings.Contains(data, f) {
			return fmt.Errorf("mount option %s not found in %s", f, data)
		}
	}

	err = deletePVCAndApp("", f, pvc, app)
	return err
}

func createNodeLabel(f *framework.Framework, labelKey, labelValue string) {
	// NOTE: This makes all nodes (in a multi-node setup) in the test take
	//       the same label values, which is fine for the test
	nodes, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	Expect(err).Should(BeNil())
	for i := range nodes.Items {
		framework.AddOrUpdateLabelOnNode(f.ClientSet, nodes.Items[i].Name, labelKey, labelValue)
	}
}

func deleteNodeLabel(c kubernetes.Interface, labelKey string) {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	Expect(err).Should(BeNil())
	for i := range nodes.Items {
		framework.RemoveLabelOffNode(c, nodes.Items[i].Name, labelKey)
	}
}

func checkNodeHasLabel(c kubernetes.Interface, labelKey, labelValue string) {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	Expect(err).Should(BeNil())
	for i := range nodes.Items {
		framework.ExpectNodeHasLabel(c, nodes.Items[i].Name, labelKey, labelValue)
	}
}

func getPVCImageInfoInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) (string, error) {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return "", err
	}

	stdOut, stdErr := execCommandInToolBoxPod(f, "rbd info "+pool+"/"+imageData.imageName, rookNamespace)
	Expect(stdErr).Should(BeEmpty())

	e2elog.Logf("found image %s in pool %s", imageData.imageName, pool)

	return stdOut, nil
}

func checkPVCImageInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	_, err := getPVCImageInfoInPool(f, pvc, pool)

	return err
}

func checkPVCDataPoolForImageInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool, dataPool string) error {
	stdOut, err := getPVCImageInfoInPool(f, pvc, pool)
	if err != nil {
		return err
	}

	if !strings.Contains(stdOut, "data_pool: "+dataPool) {
		return fmt.Errorf("missing data pool value in image info, got info (%s)", stdOut)
	}

	return nil
}

func checkPVCImageJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr := execCommandInToolBoxPod(f, "rados listomapkeys -p "+pool+" csi.volume."+imageData.imageID, rookNamespace)
	Expect(stdErr).Should(BeEmpty())

	e2elog.Logf("found image journal %s in pool %s", "csi.volume."+imageData.imageID, pool)

	return nil
}

func checkPVCCSIJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr := execCommandInToolBoxPod(f, "rados getomapval -p "+pool+" csi.volumes.default csi.volume."+imageData.pvName, rookNamespace)
	Expect(stdErr).Should(BeEmpty())

	e2elog.Logf("found CSI journal entry %s in pool %s", "csi.volume."+imageData.pvName, pool)

	return nil
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

func checkPVSelectorValuesForPVC(f *framework.Framework, pvc *v1.PersistentVolumeClaim) {
	pv, err := getBoundPV(f.ClientSet, pvc)
	if err != nil {
		Fail(err.Error())
	}

	if len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms) == 0 {
		Fail("Found empty NodeSelectorTerms in PV")
	}

	rFound := false
	zFound := false
	for _, expression := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions {
		switch expression.Key {
		case nodeCSIRegionLabel:
			if rFound {
				Fail("Found multiple occurrences of topology key for region")
			}
			rFound = true
			if expression.Values[0] != regionValue {
				Fail("Topology value for region label mismatch")
			}
		case nodeCSIZoneLabel:
			if zFound {
				Fail("Found multiple occurrences of topology key for zone")
			}
			zFound = true
			if expression.Values[0] != zoneValue {
				Fail("Topology value for zone label mismatch")
			}
		default:
			Fail("Unexpected key in node selector terms found in PV")
		}
	}
}

func addTopologyDomainsToDSYaml(template, labels string) string {
	return strings.ReplaceAll(template, "# - \"--domainlabels=failure-domain/region,failure-domain/zone\"",
		"- \"--domainlabels="+labels+"\"")
}

// createCustomConfigMap provides multiple clusters information.
func createCustomConfigMap(c kubernetes.Interface, pluginPath string, subvolgrpInfo map[string]string) {
	path := pluginPath + configMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	Expect(err).Should(BeNil())

	// get mon list
	mons := getMons(rookNamespace, c)
	// get clusterIDs
	var clusterID []string
	for key := range subvolgrpInfo {
		clusterID = append(clusterID, key)
	}
	conmap := []util.ClusterInfo{
		{
			ClusterID: clusterID[0],
			Monitors:  mons,
		},
		{
			ClusterID: clusterID[1],
			Monitors:  mons,
		}}
	for i := 0; i < len(subvolgrpInfo); i++ {
		conmap[i].CephFS.SubvolumeGroup = subvolgrpInfo[clusterID[i]]
	}
	data, err := json.Marshal(conmap)
	Expect(err).Should(BeNil())
	cm.Data["config.json"] = string(data)
	cm.Namespace = cephCSINamespace
	// since a configmap is already created, update the existing configmap
	_, updateErr := c.CoreV1().ConfigMaps(cephCSINamespace).Update(context.TODO(), &cm, metav1.UpdateOptions{})
	Expect(updateErr).Should(BeNil())
}

// validateSubvolumegroup validates whether subvolumegroup is present.
func validateSubvolumegroup(f *framework.Framework, subvolgrp string) error {
	cmd := fmt.Sprintf("ceph fs subvolumegroup getpath myfs %s", subvolgrp)
	stdOut, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	Expect(err).Should(BeEmpty())
	if err != "" {
		return fmt.Errorf("error subvolumegroup %s doesn't exist", subvolgrp)
	}
	expectedGrpPath := "/volumes/" + subvolgrp
	stdOut = strings.TrimSpace(stdOut)
	if stdOut != expectedGrpPath {
		return fmt.Errorf("error unexpected group path. Found: %s", stdOut)
	}
	return nil
}
