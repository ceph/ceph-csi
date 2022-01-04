/*
Copyright 2021 The Ceph-CSI Authors.

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
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/client/conditions"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

// getDaemonSetLabelSelector returns labels of daemonset given name and namespace dynamically,
// needed since labels are not same for helm and non-helm deployments.
func getDaemonSetLabelSelector(f *framework.Framework, ns, daemonSetName string) (string, error) {
	ds, err := f.ClientSet.AppsV1().DaemonSets(ns).Get(context.TODO(), daemonSetName, metav1.GetOptions{})
	if err != nil {
		e2elog.Logf("Error getting daemonsets with name %s in namespace %s", daemonSetName, ns)

		return "", err
	}
	s, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		e2elog.Logf("Error parsing %s daemonset selector in namespace %s", daemonSetName, ns)

		return "", err
	}
	e2elog.Logf("LabelSelector for %s daemonsets in namespace %s: %s", daemonSetName, ns, s.String())

	return s.String(), nil
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
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, err
		}
		dNum := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		e2elog.Logf(
			"%d / %d pods ready in namespace '%s' in daemonset '%s' (%d seconds elapsed)",
			ready,
			dNum,
			ns,
			ds.ObjectMeta.Name,
			int(time.Since(start).Seconds()))
		if ready != dNum {
			return false, nil
		}

		return true, nil
	})
}

func findPodAndContainerName(f *framework.Framework, ns, cn string, opt *metav1.ListOptions) (string, string, error) {
	podList, err := f.PodClientNS(ns).List(context.TODO(), *opt)
	if err != nil {
		return "", "", err
	}

	if len(podList.Items) == 0 {
		return "", "", errors.New("podlist is empty")
	}

	if cn != "" {
		for i := range podList.Items {
			for j := range podList.Items[i].Spec.Containers {
				if podList.Items[i].Spec.Containers[j].Name == cn {
					return podList.Items[i].Name, cn, nil
				}
			}
		}

		return "", "", errors.New("container name not found")
	}

	return podList.Items[0].Name, podList.Items[0].Spec.Containers[0].Name, nil
}

func getCommandInPodOpts(
	f *framework.Framework,
	c, ns, cn string,
	opt *metav1.ListOptions) (framework.ExecOptions, error) {
	cmd := []string{"/bin/sh", "-c", c}
	pName, cName, err := findPodAndContainerName(f, ns, cn, opt)
	if err != nil {
		return framework.ExecOptions{}, err
	}

	return framework.ExecOptions{
		Command:            cmd,
		PodName:            pName,
		Namespace:          ns,
		ContainerName:      cName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}, nil
}

// execCommandInDaemonsetPod executes commands inside given container of a daemonset pod on a particular node.
func execCommandInDaemonsetPod(
	f *framework.Framework,
	c, daemonsetName, nodeName, containerName, ns string) (string, string, error) {
	selector, err := getDaemonSetLabelSelector(f, ns, daemonsetName)
	if err != nil {
		return "", "", err
	}

	opt := &metav1.ListOptions{
		LabelSelector: selector,
	}
	pods, err := listPods(f, ns, opt)
	if err != nil {
		return "", "", err
	}

	podName := ""
	for i := range pods {
		if pods[i].Spec.NodeName == nodeName {
			podName = pods[i].Name
		}
	}
	if podName == "" {
		return "", "", fmt.Errorf("%s daemonset pod on node %s in namespace %s not found", daemonsetName, nodeName, ns)
	}

	cmd := []string{"/bin/sh", "-c", c}
	podOpt := framework.ExecOptions{
		Command:       cmd,
		Namespace:     ns,
		PodName:       podName,
		ContainerName: containerName,
		CaptureStdout: true,
		CaptureStderr: true,
	}

	return f.ExecWithOptions(podOpt)
}

// listPods returns slice of pods matching given ListOptions and namespace.
func listPods(f *framework.Framework, ns string, opt *metav1.ListOptions) ([]v1.Pod, error) {
	podList, err := f.PodClientNS(ns).List(context.TODO(), *opt)
	if len(podList.Items) == 0 {
		return podList.Items, fmt.Errorf("podlist for label '%s' in namespace %s is empty", opt.LabelSelector, ns)
	}

	return podList.Items, err
}

func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string, error) {
	podOpt, err := getCommandInPodOpts(f, c, ns, "", opt)
	if err != nil {
		return "", "", err
	}
	stdOut, stdErr, err := f.ExecWithOptions(podOpt)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}

	return stdOut, stdErr, err
}

func execCommandInContainer(
	f *framework.Framework, c, ns, cn string, opt *metav1.ListOptions) (string, string, error) {
	podOpt, err := getCommandInPodOpts(f, c, ns, cn, opt)
	if err != nil {
		return "", "", err
	}
	stdOut, stdErr, err := f.ExecWithOptions(podOpt)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}

	return stdOut, stdErr, err
}

func execCommandInToolBoxPod(f *framework.Framework, c, ns string) (string, string, error) {
	opt := &metav1.ListOptions{
		LabelSelector: rookToolBoxPodLabel,
	}
	podOpt, err := getCommandInPodOpts(f, c, ns, "", opt)
	if err != nil {
		return "", "", err
	}
	stdOut, stdErr, err := f.ExecWithOptions(podOpt)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}

	return stdOut, stdErr, err
}

func execCommandInPodAndAllowFail(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string) {
	podOpt, err := getCommandInPodOpts(f, c, ns, "", opt)
	if err != nil {
		return "", err.Error()
	}
	stdOut, stdErr, err := f.ExecWithOptions(podOpt)
	if err != nil {
		e2elog.Logf("command %s failed: %v", c, err)
	}

	return stdOut, stdErr
}

func loadApp(path string) (*v1.Pod, error) {
	app := v1.Pod{}
	if err := unmarshal(path, &app); err != nil {
		return nil, err
	}
	for i := range app.Spec.Containers {
		app.Spec.Containers[i].ImagePullPolicy = v1.PullIfNotPresent
	}

	return &app, nil
}

func createApp(c kubernetes.Interface, app *v1.Pod, timeout int) error {
	_, err := c.CoreV1().Pods(app.Namespace).Create(context.TODO(), app, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	return waitForPodInRunningState(app.Name, app.Namespace, c, timeout, noError)
}

func createAppErr(c kubernetes.Interface, app *v1.Pod, timeout int, errString string) error {
	_, err := c.CoreV1().Pods(app.Namespace).Create(context.TODO(), app, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return waitForPodInRunningState(app.Name, app.Namespace, c, timeout, errString)
}

func waitForPodInRunningState(name, ns string, c kubernetes.Interface, t int, expectedError string) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be in Running state", name)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		pod, err := c.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get app: %w", err)
		}
		switch pod.Status.Phase {
		case v1.PodRunning:
			return true, nil
		case v1.PodFailed, v1.PodSucceeded:
			return false, conditions.ErrPodCompleted
		case v1.PodPending:
			if expectedError != "" {
				events, err := c.CoreV1().Events(ns).List(context.TODO(), metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.name=%s", name),
				})
				if err != nil {
					return false, err
				}
				if strings.Contains(events.String(), expectedError) {
					e2elog.Logf("Expected Error %q found successfully", expectedError)

					return true, err
				}
			}
		case v1.PodUnknown:
			e2elog.Logf(
				"%s app  is in %s phase expected to be in Running  state (%d seconds elapsed)",
				name,
				pod.Status.Phase,
				int(time.Since(start).Seconds()))
		}

		return false, nil
	})
}

func deletePod(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
	err := c.CoreV1().Pods(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	start := time.Now()
	e2elog.Logf("Waiting for pod %v to be deleted", name)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := c.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			e2elog.Logf("%s app  to be deleted (%d seconds elapsed)", name, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to get app: %w", err)
		}

		return false, nil
	})
}

// nolint:unparam // currently skipNotFound is always false, this can change in the future
func deletePodWithLabel(label, ns string, skipNotFound bool) error {
	err := retryKubectlArgs(
		ns,
		kubectlDelete,
		deployTimeout,
		"po",
		"-l",
		label,
		fmt.Sprintf("--ignore-not-found=%t", skipNotFound))
	if err != nil {
		e2elog.Logf("failed to delete pod %v", err)
	}

	return err
}

// calculateSHA512sum returns the sha512sum of a file inside a pod.
func calculateSHA512sum(f *framework.Framework, app *v1.Pod, filePath string, opt *metav1.ListOptions) (string, error) {
	cmd := fmt.Sprintf("sha512sum %s", filePath)
	sha512sumOut, stdErr, err := execCommandInPod(f, cmd, app.Namespace, opt)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return "", fmt.Errorf("error: sha512sum could not be calculated %v", stdErr)
	}
	// extract checksum from sha512sum output.
	checkSum := strings.Split(sha512sumOut, "")[0]
	e2elog.Logf("Calculated checksum  %s", checkSum)

	return checkSum, nil
}

// getKernelVersionFromDaemonset gets the kernel version from the specified container.
func getKernelVersionFromDaemonset(f *framework.Framework, ns, dsn, cn string) (string, error) {
	selector, err := getDaemonSetLabelSelector(f, ns, dsn)
	if err != nil {
		return "", err
	}

	opt := metav1.ListOptions{
		LabelSelector: selector,
	}

	kernelRelease, stdErr, err := execCommandInContainer(f, "uname -r", ns, cn, &opt)
	if err != nil || stdErr != "" {
		return "", err
	}

	return kernelRelease, nil
}

// recreateCSIPods delete the daemonset and deployment pods based on the selectors passed in.
func recreateCSIPods(f *framework.Framework, podLabels, daemonsetName, deploymentName string) error {
	err := deletePodWithLabel(podLabels, cephCSINamespace, false)
	if err != nil {
		return fmt.Errorf("failed to delete pods with labels (%s): %w", podLabels, err)
	}
	// wait for csi pods to come up
	err = waitForDaemonSets(daemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("timeout waiting for daemonset pods: %w", err)
	}
	err = waitForDeploymentComplete(f.ClientSet, deploymentName, cephCSINamespace, deployTimeout)
	if err != nil {
		return fmt.Errorf("timeout waiting for deployment to be in running state: %w", err)
	}

	return nil
}
