package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
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

func getAdminCreds(f *framework.Framework, c string) string {

	ns := "rook-ceph"
	cmd := []string{"/bin/sh", "-c", c}
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	podList, err := f.PodClientNS(ns).List(opt)
	framework.ExpectNoError(err)
	Expect(podList).NotTo(BeNil())
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

func createPVC(c kubernetes.Interface, path string, timeout time.Duration) error {
	pvc := v1.PersistentVolumeClaim{}
	err := unmarshal(path, &pvc)
	Expect(err).Should(BeNil())
	_, err = c.CoreV1().PersistentVolumeClaims("default").Create(&pvc)
	Expect(err).Should(BeNil())
	name := pvc.Name
	start := time.Now()
	framework.Logf("Waiting up to %v to be in Bound state", pvc)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		pv, err := c.CoreV1().PersistentVolumeClaims(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting daemonsets in namespace: '%s': %v", ns, err)
			if testutils.IsRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}

		framework.Logf("%s pvc  is  in %s state expected to be in Bound  state (%d seconds elapsed)", name, pv.Status.String(), int(time.Since(start).Seconds()))
		if pv.Status.String() != "Bound" {
			return false, nil
		}

		return true, nil
	})
}

func createApp(c kubernetes.Interface, path string, timeout time.Duration) error {
	app := v1.Pod{}
	err := unmarshal(path, &app)
	Expect(err).Should(BeNil())
	_, err = c.CoreV1().Pods("default").Create(&app)
	name := app.Name
	start := time.Now()
	framework.Logf("Waiting up to %v to be in Bound state", app.Name)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		ap, err := c.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting daemonsets in namespace: '%s': %v", ns, err)
			if testutils.IsRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}

		framework.Logf("%s app  is  in %s state expected to be in Bound  state (%d seconds elapsed)", name, ap.Status.String(), int(time.Since(start).Seconds()))
		if ap.Status.String() != "Running" {
			return false, nil
		}

		return true, nil
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
