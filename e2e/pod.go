package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/client/conditions"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

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
		deployment *appsv1.Deployment
		reason     string
		err        error
	)
	timeout := time.Duration(t) * time.Minute
	err = wait.PollImmediate(poll, timeout, func() (bool, error) {
		deployment, err = c.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			// a StatusError is not marked as 'retryable', but we want to retry anyway
			if isRetryableAPIError(err) || strings.Contains(err.Error(), "etcdserver: request timed out") {
				// hide API-server timeouts, so that PollImmediate() retries
				e2elog.Logf("deployment error: %v", err)
				return false, nil
			}
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
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %w", name, err)
	}
	return nil
}

func getCommandInPodOpts(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (framework.ExecOptions, error) {
	cmd := []string{"/bin/sh", "-c", c}
	podList, err := f.PodClientNS(ns).List(context.TODO(), *opt)
	framework.ExpectNoError(err)
	if len(podList.Items) == 0 {
		return framework.ExecOptions{}, errors.New("podlist is empty")
	}
	if err != nil {
		return framework.ExecOptions{}, err
	}
	return framework.ExecOptions{
		Command:            cmd,
		PodName:            podList.Items[0].Name,
		Namespace:          ns,
		ContainerName:      podList.Items[0].Spec.Containers[0].Name,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}, nil
}

func execCommandInPod(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string, error) {
	podPot, err := getCommandInPodOpts(f, c, ns, opt)
	if err != nil {
		return "", "", err
	}
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	return stdOut, stdErr, err
}

func execCommandInToolBoxPod(f *framework.Framework, c, ns string) (string, string, error) {
	opt := &metav1.ListOptions{
		LabelSelector: rookTolBoxPodLabel,
	}
	podPot, err := getCommandInPodOpts(f, c, ns, opt)
	if err != nil {
		return "", "", err
	}
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if stdErr != "" {
		e2elog.Logf("stdErr occurred: %v", stdErr)
	}
	return stdOut, stdErr, err
}

func execCommandInPodAndAllowFail(f *framework.Framework, c, ns string, opt *metav1.ListOptions) (string, string) {
	podPot, err := getCommandInPodOpts(f, c, ns, opt)
	if err != nil {
		return "", err.Error()
	}
	stdOut, stdErr, err := f.ExecWithOptions(podPot)
	if err != nil {
		e2elog.Logf("command %s failed: %v", c, err)
	}
	return stdOut, stdErr
}

func loadApp(path string) (*v1.Pod, error) {
	app := v1.Pod{}
	err := unmarshal(path, &app)
	if err != nil {
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

func deletePodWithLabel(label, ns string, skipNotFound bool) error {
	_, err := framework.RunKubectl(ns, "delete", "po", "-l", label, fmt.Sprintf("--ignore-not-found=%t", skipNotFound))
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
