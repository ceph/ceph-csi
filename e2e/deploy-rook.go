package e2e

import (
	"fmt"
	"strings"

	. "github.com/onsi/gomega" // nolint
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	rookURL = "https://raw.githubusercontent.com/rook/rook/$version/cluster/examples/kubernetes/ceph"
)

var rookNS = "rook-ceph"

func formRookURL(version string) {
	rookURL = strings.Replace(rookURL, "$version", version, 1)
}

func getK8sClient() kubernetes.Interface {
	framework.Logf("Creating a kubernetes client")
	client, err := framework.LoadClientset()
	Expect(err).Should(BeNil())
	return client

}

func deployCommon() {
	commonPath := fmt.Sprintf("%s/%s", rookURL, "common.yaml")
	framework.RunKubectlOrDie("create", "-f", commonPath)
}

func createFileSystem(c kubernetes.Interface) {
	commonPath := fmt.Sprintf("%s/%s", rookURL, "filesystem-test.yaml")
	framework.RunKubectlOrDie("create", "-f", commonPath)
	opt := &metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mds",
	}
	err := checkCephPods(rookNS, c, 1, deployTimeout, opt)
	Expect(err).Should(BeNil())
}

func createRBDPool() {
	commonPath := fmt.Sprintf("%s/%s", rookURL, "pool-test.yaml")
	framework.RunKubectlOrDie("create", "-f", commonPath)
}
func deleteFileSystem() {
	commonPath := fmt.Sprintf("%s/%s", rookURL, "filesystem-test.yaml")
	framework.RunKubectl("delete", "-f", commonPath)
}

func deleteRBDPool() {
	commonPath := fmt.Sprintf("%s/%s", rookURL, "pool-test.yaml")
	framework.RunKubectl("delete", "-f", commonPath)
}

func deployOperator(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "operator.yaml")

	framework.RunKubectlOrDie("create", "-f", opPath)
	err := waitForDaemonSets("rook-ceph-agent", rookNS, c, deployTimeout)
	Expect(err).Should(BeNil())
	err = waitForDaemonSets("rook-discover", rookNS, c, deployTimeout)
	Expect(err).Should(BeNil())
	err = waitForDeploymentComplete("rook-ceph-operator", rookNS, c, deployTimeout)
	Expect(err).Should(BeNil())
}

func deployCluster(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "cluster-test.yaml")
	framework.RunKubectlOrDie("create", "-f", opPath)
	opt := &metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	err := checkCephPods(rookNS, c, 1, deployTimeout, opt)
	Expect(err).Should(BeNil())
}

func deployToolBox(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "toolbox.yaml")
	framework.RunKubectlOrDie("create", "-f", opPath)
	opt := &metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}

	name := getPodName(rookNS, c, opt)
	err := waitForPodInRunningState(name, rookNS, c, deployTimeout)
	Expect(err).Should(BeNil())
}

func deployRook() {
	c := getK8sClient()
	deployCommon()
	deployOperator(c)
	deployCluster(c)
	deployToolBox(c)
}

func tearDownRook() {
	opPath := fmt.Sprintf("%s/%s", rookURL, "cluster-test.yaml")
	framework.Cleanup(opPath, rookNS, "app=rook-ceph-mon")
	opPath = fmt.Sprintf("%s/%s", rookURL, "toolbox.yaml")
	framework.Cleanup(opPath, rookNS, "app=rook-ceph-tools")

	opPath = fmt.Sprintf("%s/%s", rookURL, "operator.yaml")
	// TODO need to add selector for cleanup validation
	framework.Cleanup(opPath, rookNS)
	commonPath := fmt.Sprintf("%s/%s", rookURL, "common.yaml")
	framework.RunKubectl("delete", "-f", commonPath)
}
