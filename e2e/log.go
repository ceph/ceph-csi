package e2e

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

func logsCSIPods(label string, c clientset.Interface) {
	ns := "default"
	opt := metav1.ListOptions{
		LabelSelector: label,
	}
	podList, err := c.CoreV1().Pods(ns).List(opt)
	if err != nil {
		e2elog.Logf("failed to list pods with selector %s %v", label, err)
		return
	}

	e2elog.Logf("Running kubectl logs on ceph-csi containers in %v", ns)
	for i := range podList.Items {
		kubectlLogPod(c, &podList.Items[i])
	}
}

func kubectlLogPod(c clientset.Interface, pod *v1.Pod) {
	container := pod.Spec.Containers
	for i := range container {
		logs, err := framework.GetPodLogs(c, pod.Namespace, pod.Name, container[i].Name)
		if err != nil {
			logs, err = getPreviousPodLogs(c, pod.Namespace, pod.Name, container[i].Name)
			if err != nil {
				e2elog.Logf("Failed to get logs of pod %v, container %v, err: %v", pod.Name, container[i].Name, err)
			}
		}
		e2elog.Logf("Logs of %v/%v:%v on node %v\n", pod.Namespace, pod.Name, container[i].Name, pod.Spec.NodeName)
		e2elog.Logf("STARTLOG\n\n%s\n\nENDLOG for container %v:%v:%v", logs, pod.Namespace, pod.Name, container[i].Name)
	}

}

func getPreviousPodLogs(c clientset.Interface, namespace, podName, containerName string) (string, error) {
	logs, err := c.CoreV1().RESTClient().Get().
		Resource("pods").
		Namespace(namespace).
		Name(podName).SubResource("log").
		Param("container", containerName).
		Param("previous", strconv.FormatBool(true)).
		Do().
		Raw()
	if err != nil {
		return "", err
	}
	if strings.Contains(string(logs), "Internal Error") {
		return "", fmt.Errorf("fetched log contains \"Internal Error\": %q", string(logs))
	}
	return string(logs), err
}
