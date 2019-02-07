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

var rook = []string{"operator.yaml", "cluster.yaml", "toolbox.yaml"}

func formRookURL(version string) {
	rookURL = strings.Replace(rookURL, "version", version, 1)
}

func getK8sClient() kubernetes.Interface {
	framework.Logf("Creating a kubernetes client")
	//TODO fix err
	client, _ := framework.LoadClientset()
	return client

}
func deployOperator(c kubernetes.Interface) {
	//opPath := fmt.Sprintf("%s/%s", rookURL, "operator.yaml")

	//TODO need to fix this, currently am using coreos
	opPath := "https://raw.githubusercontent.com/Madhu-1/rook/fix-misspell/cluster/examples/kubernetes/ceph/operator.yaml"
	framework.RunKubectlOrDie("create", "-f", opPath)
	waitForDaemonSets("rook-ceph-agent", "rook-ceph-system", c, 5*time.Minute)
	waitForDaemonSets("rook-discover", "rook-ceph-system", c, 5*time.Minute)
	waitForDeploymentComplete("rook-ceph-operator", "rook-ceph-system", c, 5*time.Minute)
}

func deployCluster(c kubernetes.Interface) {
	opPath := fmt.Sprintf("%s/%s", rookURL, "cluster.yaml")
	framework.RunKubectlOrDie("create", "-f", opPath)
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	checkMonPods("rook-ceph", c, 3, 5*time.Minute, opt)
	opt = metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mgr",
	}
	checkMonPods("rook-ceph", c, 3, 5*time.Minute, opt)
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
	deployOperator(c)
	deployCluster(c)
	deployToolBox(c)
}

func tearDownRook() {
	opPath := fmt.Sprintf("%s/%s", rookURL, "cluster.yaml")
	framework.Cleanup(opPath, "rook-ceph-system", "app=rook-ceph-mgr", "app=rook-ceph-mon")
	opPath = fmt.Sprintf("%s/%s", rookURL, "toolbox.yaml")
	framework.Cleanup(opPath, "rook-ceph-system", "app=rook-ceph-tools")

	opPath = fmt.Sprintf("%s/%s", rookURL, "operator.yaml")
	//TODO need to add selector for cleanup validation
	framework.Cleanup(opPath, "rook-ceph-system")

}
