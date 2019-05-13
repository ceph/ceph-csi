package e2e

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	rookURL = "https://raw.githubusercontent.com/rook/rook/version/cluster/examples/kubernetes/ceph"
)

var rook = []string{"common.yaml", "operator.yaml", "cluster.yaml", "toolbox.yaml"}

func formRookURL(version string) {
	rookURL = strings.Replace(rookURL, "version", version, 1)
}

func getK8sClient() kubernetes.Interface {
	framework.Logf("Creating a kubernetes client")
	client, _ := framework.LoadClientset()
	return client

}

func deployCommon(c kubernetes.Interface) {
	commonPath := fmt.Sprintf("%s/%s", rookURL, "common.yaml")
	framework.RunKubectlOrDie("create", "-f", commonPath)
}

func deployOperator(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "operator.yaml")

	framework.RunKubectlOrDie("create", "-f", opPath)
	waitForDaemonSets("rook-ceph-agent", "rook-ceph", c, 5*time.Minute)
	waitForDaemonSets("rook-discover", "rook-ceph", c, 5*time.Minute)
	waitForDeploymentComplete("rook-ceph-operator", "rook-ceph", c, 5*time.Minute)
}

func deployCluster(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "cluster.yaml")
	framework.RunKubectlOrDie("create", "-f", opPath)
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	checkCephPods("rook-ceph", c, 1, 5*time.Minute, opt)
	//opt = metav1.ListOptions{
	//	LabelSelector: "app=rook-ceph-mgr",
	//}
	//checkCephPods("rook-ceph", c, 1, 5*time.Minute, opt)
}

func deployToolBox(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "toolbox.yaml")
	framework.RunKubectlOrDie("create", "-f", opPath)
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}

	name := getPodName("rook-ceph", c, opt)

	waitForPodInRunningState(name, "rook-ceph", c, 5*time.Minute)
}

func deployRook() {
	c := getK8sClient()
	deployCommon(c)
	deployOperator(c)
	deployCluster(c)
	deployToolBox(c)
}

func tearDownRook() {
	opPath := fmt.Sprintf("%s/%s", rookURL, "cluster.yaml")
	framework.Cleanup(opPath, "rook-ceph", "app=rook-ceph-mgr", "app=rook-ceph-mon")
	opPath = fmt.Sprintf("%s/%s", rookURL, "toolbox.yaml")
	framework.Cleanup(opPath, "rook-ceph", "app=rook-ceph-tools")

	opPath = fmt.Sprintf("%s/%s", rookURL, "operator.yaml")
	//TODO need to add selector for cleanup validation
	framework.Cleanup(opPath, "rook-ceph")
}
