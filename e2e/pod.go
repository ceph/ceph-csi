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
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	frameworkPod "k8s.io/kubernetes/test/e2e/framework/pod"
)

const errRWOPConflict = "node has pod using PersistentVolumeClaim with the same name and ReadWriteOncePod access mode."

// getDaemonSetLabelSelector returns labels of daemonset given name and namespace dynamically,
// needed since labels are not same for helm and non-helm deployments.
func getDaemonSetLabelSelector(f *framework.Framework, ns, daemonSetName string) (string, error) {
	ds, err := f.ClientSet.AppsV1().DaemonSets(ns).Get(context.TODO(), daemonSetName, metav1.GetOptions{})
	if err != nil {
		framework.Logf("Error getting daemonsets with name %s in namespace %s", daemonSetName, ns)

		return "", err
	}
	s, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		framework.Logf("Error parsing %s daemonset selector in namespace %s", daemonSetName, ns)

		return "", err
	}
	framework.Logf("LabelSelector for %s daemonsets in namespace %s: %s", daemonSetName, ns, s.String())

	return s.String(), nil
}

func waitForDaemonSets(name, ns string, c kubernetes.Interface, t int) error {
	timeout := time.Duration(t) * time.Minute
	start := time.Now()
	framework.Logf("Waiting up to %v for all daemonsets in namespace '%s' to start", timeout, ns)

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		ds, err := c.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting daemonsets in namespace: '%s': %v", ns, err)
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
		framework.Logf(
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
	timeout := time.Duration(deployTimeout) * time.Minute

	var (
		podList *v1.PodList
		listErr error
	)
	err := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		podList, listErr = e2epod.PodClientNS(f, ns).List(ctx, *opt)
		if listErr != nil {
			if isRetryableAPIError(listErr) {
				return false, nil
			}

			return false, fmt.Errorf("failed to list Pods: %w", listErr)
		}

		if len(podList.Items) == 0 {
			// retry in case the pods have not been (re)started yet
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to find pod for %v: %w", opt, err)
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
	opt *metav1.ListOptions,
) (e2epod.ExecOptions, error) {
	cmd := []string{"/bin/sh", "-c", c}
	pName, cName, err := findPodAndContainerName(f, ns, cn, opt)
	if err != nil {
		return e2epod.ExecOptions{}, err
	}

	return e2epod.ExecOptions{
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

// execCommandInDaemonsetPod executes commands inside given container of a
// daemonset pod on a particular node.
//
// stderr is returned as a string, and err will be set on a failure.
func execCommandInDaemonsetPod(
	f *framework.Framework,
	c, daemonsetName, nodeName, containerName, ns string,
) (string, error) {
	podName, err := getDaemonsetPodOnNode(f, daemonsetName, nodeName, ns)
	if err != nil {
		return "", err
	}

	cmd := []string{"/bin/sh", "-c", c}
	podOpt := e2epod.ExecOptions{
		Command:       cmd,
		Namespace:     ns,
		PodName:       podName,
		ContainerName: containerName,
		CaptureStdout: true,
		CaptureStderr: true,
	}

	_ /* stdout */, stderr, err := execWithRetry(f, &podOpt)

	return stderr, err
}

// getDaemonsetPodOnNode returns the name of a daemonset pod on a particular node.
func getDaemonsetPodOnNode(f *framework.Framework, daemonsetName, nodeName, ns string) (string, error) {
	selector, err := getDaemonSetLabelSelector(f, ns, daemonsetName)
	if err != nil {
		return "", err
	}

	opt := &metav1.ListOptions{
		LabelSelector: selector,
	}
	pods, err := listPods(f, ns, opt)
	if err != nil {
		return "", err
	}

	podName := ""
	for i := range pods {
		if pods[i].Spec.NodeName == nodeName {
			podName = pods[i].Name
		}
	}
	if podName == "" {
		return "", fmt.Errorf("%s daemonset pod on node %s in namespace %s not found", daemonsetName, nodeName, ns)
	}

	return podName, nil
}

// listPods returns slice of pods matching given ListOptions and namespace.
func listPods(f *framework.Framework, ns string, opt *metav1.ListOptions) ([]v1.Pod, error) {
	podList, err := e2epod.PodClientNS(f, ns).List(context.TODO(), *opt)
	if len(podList.Items) == 0 {
		return podList.Items, fmt.Errorf("podlist for label '%s' in namespace %s is empty", opt.LabelSelector, ns)
	}

	return podList.Items, err
}

func execWithRetry(f *framework.Framework, opts *e2epod.ExecOptions) (string, string, error) {
	timeout := time.Duration(deployTimeout) * time.Minute
	var stdOut, stdErr string
	err := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		var execErr error
		stdOut, stdErr, execErr = e2epod.ExecWithOptions(f, *opts)
		if execErr != nil {
			if isRetryableAPIError(execErr) {
				return false, nil
			}

			framework.Logf("failed to execute command: %v", execErr)

			return false, fmt.Errorf("failed to execute command: %w", execErr)
		}

		return true, nil
	})

	return stdOut, stdErr, err
}

func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string, error) {
	podOpt, err := getCommandInPodOpts(f, c, ns, "", opt)
	if err != nil {
		return "", "", err
	}

	stdOut, stdErr, err := execWithRetry(f, &podOpt)
	if stdErr != "" {
		framework.Logf("stdErr occurred: %v", stdErr)
	}

	return stdOut, stdErr, err
}

func execCommandInContainer(
	f *framework.Framework, c, ns, cn string, opt *metav1.ListOptions,
) (string, string, error) {
	podOpt, err := getCommandInPodOpts(f, c, ns, cn, opt)
	if err != nil {
		return "", "", err
	}

	stdOut, stdErr, err := execWithRetry(f, &podOpt)
	if stdErr != "" {
		framework.Logf("stdErr occurred: %v", stdErr)
	}

	return stdOut, stdErr, err
}

func execCommandInContainerByPodName(
	f *framework.Framework, shellCmd, namespace, podName, containerName string,
) (string, string, error) {
	cmd := []string{"/bin/sh", "-c", shellCmd}
	execOpts := e2epod.ExecOptions{
		Command:            cmd,
		PodName:            podName,
		Namespace:          namespace,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}

	stdOut, stdErr, err := execWithRetry(f, &execOpts)
	if stdErr != "" {
		framework.Logf("stdErr occurred: %v", stdErr)
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

	stdOut, stdErr, err := execWithRetry(f, &podOpt)
	if stdErr != "" {
		framework.Logf("stdErr occurred: %v", stdErr)
	}

	return stdOut, stdErr, err
}

func execCommandInPodAndAllowFail(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string) {
	podOpt, err := getCommandInPodOpts(f, c, ns, "", opt)
	if err != nil {
		return "", err.Error()
	}

	stdOut, stdErr, err := execWithRetry(f, &podOpt)
	if err != nil {
		framework.Logf("command %s failed: %v", c, err)
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
	framework.Logf("Waiting up to %v to be in Running state", name)

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := c.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
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
				events, err := c.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.name=%s", name),
				})
				if err != nil {
					return false, err
				}
				if strings.Contains(events.String(), expectedError) {
					framework.Logf("Expected Error %q found successfully", expectedError)

					return true, err
				}
			}
		case v1.PodUnknown:
			framework.Logf(
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
	ctx := context.TODO()
	err := c.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	start := time.Now()
	framework.Logf("Waiting for pod %v to be deleted", name)

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := c.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			framework.Logf("%s app  to be deleted (%d seconds elapsed)", name, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to get app: %w", err)
		}

		return false, nil
	})
}

//nolint:unparam // currently skipNotFound is always false, this can change in the future
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
		framework.Logf("failed to delete pod %v", err)
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
	framework.Logf("Calculated checksum  %s", checkSum)

	return checkSum, nil
}

func appendToFileInContainer(
	f *framework.Framework,
	app *v1.Pod,
	filePath,
	toAppend string,
	opt *metav1.ListOptions,
) error {
	cmd := fmt.Sprintf("echo %q >> %s", toAppend, filePath)
	_, stdErr, err := execCommandInPod(f, cmd, app.Namespace, opt)
	if err != nil {
		return fmt.Errorf("could not append to file %s: %w ; stderr: %s", filePath, err, stdErr)
	}
	if stdErr != "" {
		return fmt.Errorf("could not append to file %s: %v", filePath, stdErr)
	}

	return nil
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

// validateRWOPPodCreation validates the second pod creation failure scenario with RWOP pvc.
func validateRWOPPodCreation(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	app *v1.Pod,
	baseAppName string,
) error {
	var err error
	// create one more  app with same PVC
	name := fmt.Sprintf("%s%d", f.UniqueName, deployTimeout)
	app.Name = name

	err = createAppErr(f.ClientSet, app, deployTimeout, errRWOPConflict)
	if err != nil {
		return fmt.Errorf("application should not go to running state due to RWOP access mode: %w", err)
	}

	err = deletePod(name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}

	app.Name = baseAppName
	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return fmt.Errorf("failed to delete PVC and application: %w", err)
	}

	return nil
}

// verifySeLinuxMountOption verifies the SeLinux context MountOption added to PV.Spec.MountOption
// is successfully used by nodeplugin during mounting by checking for its presence in the
// nodeplugin container logs.
func verifySeLinuxMountOption(
	f *framework.Framework,
	pvcPath, appPath, daemonSetName, cn, ns string,
) error {
	mountOption := "context=\"system_u:object_r:container_file_t:s0:c0,c1\""

	// create PVC
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load pvc: %w", err)
	}
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create PVC: %w", err)
	}
	// modify PV spec.MountOptions
	pv, err := getBoundPV(f.ClientSet, pvc)
	if err != nil {
		return fmt.Errorf("failed to get PV: %w", err)
	}
	pv.Spec.MountOptions = []string{mountOption}

	// update PV
	_, err = f.ClientSet.CoreV1().PersistentVolumes().Update(context.TODO(), pv, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update pv: %w", err)
	}

	app, err := loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load application: %w", err)
	}
	app.Namespace = f.UniqueName
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}

	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), app.Name, metav1.GetOptions{})
	if err != nil {
		framework.Logf("Error occurred getting pod %s in namespace %s", app.Name, f.UniqueName)

		return fmt.Errorf("failed to get pod: %w", err)
	}

	nodepluginPodName, err := getDaemonsetPodOnNode(f, daemonSetName, pod.Spec.NodeName, ns)
	if err != nil {
		return fmt.Errorf("failed to get daemonset pod on node: %w", err)
	}
	logs, err := frameworkPod.GetPodLogs(context.TODO(), f.ClientSet, ns, nodepluginPodName, cn)
	if err != nil {
		return fmt.Errorf("failed to get pod logs from container %s/%s/%s : %w", ns, nodepluginPodName, cn, err)
	}

	if !strings.Contains(logs, mountOption) {
		return fmt.Errorf("mount option %s not found in logs: %s", mountOption, logs)
	}

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return fmt.Errorf("failed to delete PVC and application: %w", err)
	}

	return nil
}
