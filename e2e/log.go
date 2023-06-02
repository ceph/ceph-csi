/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	frameworkPod "k8s.io/kubernetes/test/e2e/framework/pod"
)

func logsCSIPods(label string, c clientset.Interface) {
	opt := metav1.ListOptions{
		LabelSelector: label,
	}
	podList, err := c.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
	if err != nil {
		framework.Logf("failed to list pods with selector %s %v", label, err)

		return
	}

	for i := range podList.Items {
		kubectlLogPod(c, &podList.Items[i])
	}
}

// source: https://github.com/kubernetes/kubernetes/blob/master/test/e2e/framework/kubectl/kubectl_utils.go
func kubectlLogPod(c clientset.Interface, pod *v1.Pod) {
	container := pod.Spec.Containers
	for i := range container {
		logs, err := frameworkPod.GetPodLogs(context.TODO(), c, pod.Namespace, pod.Name, container[i].Name)
		if err != nil {
			logs, err = getPreviousPodLogs(c, pod.Namespace, pod.Name, container[i].Name)
			if err != nil {
				framework.Logf("Failed to get logs of pod %v, container %v, err: %v", pod.Name, container[i].Name, err)
			}
		}
		framework.Logf("Logs of %v/%v:%v on node %v\n", pod.Namespace, pod.Name, container[i].Name, pod.Spec.NodeName)
		framework.Logf("STARTLOG\n\n%s\n\nENDLOG for container %v:%v:%v", logs, pod.Namespace, pod.Name, container[i].Name)
	}
}

func getPreviousPodLogs(c clientset.Interface, namespace, podName, containerName string) (string, error) {
	logs, err := c.CoreV1().RESTClient().Get().
		Resource("pods").
		Namespace(namespace).
		Name(podName).SubResource("log").
		Param("container", containerName).
		Param("previous", strconv.FormatBool(true)).
		Do(context.TODO()).
		Raw()
	if err != nil {
		return "", err
	}
	if strings.Contains(string(logs), "Internal Error") {
		return "", fmt.Errorf("fetched log contains \"Internal Error\": %q", string(logs))
	}

	return string(logs), err
}
