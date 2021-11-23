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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	deploymentutil "k8s.io/kubernetes/pkg/controller/deployment/util"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

// execCommandInPodWithName run command in pod using podName.
func execCommandInPodWithName(
	f *framework.Framework,
	cmdString,
	podName,
	containerName,
	nameSpace string) (string, string, error) {
	cmd := []string{"/bin/sh", "-c", cmdString}
	podOpt := framework.ExecOptions{
		Command:            cmd,
		PodName:            podName,
		Namespace:          nameSpace,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}

	return f.ExecWithOptions(podOpt)
}

// loadAppDeployment loads the deployment app config and return deployment
// object.
func loadAppDeployment(path string) (*appsv1.Deployment, error) {
	deploy := appsv1.Deployment{}
	if err := unmarshal(path, &deploy); err != nil {
		return nil, err
	}

	return &deploy, nil
}

// createDeploymentApp creates the deployment object and waits for it to be in
// Available state.
func createDeploymentApp(clientSet kubernetes.Interface, app *appsv1.Deployment, deployTimeout int) error {
	_, err := clientSet.AppsV1().Deployments(app.Namespace).Create(context.TODO(), app, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deploy: %w", err)
	}

	return waitForDeploymentInAvailableState(clientSet, app.Name, app.Namespace, deployTimeout)
}

// deleteDeploymentApp deletes the deployment object.
func deleteDeploymentApp(clientSet kubernetes.Interface, name, ns string, deployTimeout int) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	err := clientSet.AppsV1().Deployments(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}
	start := time.Now()
	e2elog.Logf("Waiting for deployment %q to be deleted", name)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := clientSet.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			e2elog.Logf("%q deployment to be deleted (%d seconds elapsed)", name, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to get deployment: %w", err)
		}

		return false, nil
	})
}

// waitForDeploymentInAvailableState wait for deployment to be in Available state.
func waitForDeploymentInAvailableState(clientSet kubernetes.Interface, name, ns string, deployTimeout int) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	start := time.Now()
	e2elog.Logf("Waiting up to %q to be in Available state", name)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		d, err := clientSet.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			e2elog.Logf("%q deployment to be Available (%d seconds elapsed)", name, int(time.Since(start).Seconds()))

			return false, err
		}
		cond := deploymentutil.GetDeploymentCondition(d.Status, appsv1.DeploymentAvailable)

		return cond != nil, nil
	})
}

// Waits for the deployment to complete.
func waitForDeploymentComplete(clientSet kubernetes.Interface, name, ns string, deployTimeout int) error {
	var (
		deployment *appsv1.Deployment
		reason     string
		err        error
	)
	timeout := time.Duration(deployTimeout) * time.Minute
	err = wait.PollImmediate(poll, timeout, func() (bool, error) {
		deployment, err = clientSet.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			e2elog.Logf("deployment error: %v", err)

			return false, err
		}

		// TODO need to check rolling update

		// When the deployment status and its underlying resources reach the
		// desired state, we're done
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}
		e2elog.Logf(
			"deployment status: expected replica count %d running replica count %d",
			deployment.Status.Replicas,
			deployment.Status.ReadyReplicas)
		reason = fmt.Sprintf("deployment status: %#v", deployment.Status.String())

		return false, nil
	})

	if errors.Is(err, wait.ErrWaitTimeout) {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match desired state: %w", name, err)
	}

	return nil
}
