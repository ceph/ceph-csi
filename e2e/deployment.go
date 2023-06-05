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
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	deploymentutil "k8s.io/kubernetes/pkg/controller/deployment/util"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

// execCommandInPodWithName run command in pod using podName.
func execCommandInPodWithName(
	f *framework.Framework,
	cmdString,
	podName,
	containerName,
	nameSpace string,
) (string, string, error) {
	cmd := []string{"/bin/sh", "-c", cmdString}
	podOpt := e2epod.ExecOptions{
		Command:            cmd,
		PodName:            podName,
		Namespace:          nameSpace,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: true,
	}

	return e2epod.ExecWithOptions(f, podOpt)
}

// loadAppDeployment loads the deployment app config and return deployment
// object.
func loadAppDeployment(path string) (*appsv1.Deployment, error) {
	deploy := appsv1.Deployment{}
	if err := unmarshal(path, &deploy); err != nil {
		return nil, err
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		deploy.Spec.Template.Spec.Containers[i].ImagePullPolicy = v1.PullIfNotPresent
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
	ctx := context.TODO()
	err := clientSet.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}
	start := time.Now()
	framework.Logf("Waiting for deployment %q to be deleted", name)

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := clientSet.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			framework.Logf("%q deployment to be deleted (%d seconds elapsed)", name, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to get deployment: %w", err)
		}

		return false, nil
	})
}

// waitForDeploymentInAvailableState wait for deployment to be in Available state.
func waitForDeploymentInAvailableState(clientSet kubernetes.Interface, name, ns string, deployTimeout int) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	start := time.Now()
	framework.Logf("Waiting up to %q to be in Available state", name)

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		d, err := clientSet.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			framework.Logf("%q deployment to be Available (%d seconds elapsed)", name, int(time.Since(start).Seconds()))

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
	err = wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		deployment, err = clientSet.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			framework.Logf("deployment error: %v", err)

			return false, err
		}

		// TODO need to check rolling update

		// When the deployment status and its underlying resources reach the
		// desired state, we're done
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}
		framework.Logf(
			"deployment status: expected replica count %d running replica count %d",
			deployment.Status.Replicas,
			deployment.Status.ReadyReplicas)
		reason = fmt.Sprintf("deployment status: %#v", deployment.Status.String())

		return false, nil
	})

	if wait.Interrupted(err) {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match desired state: %w", name, err)
	}

	return nil
}

// ResourceDeployer provides a generic interface for deploying different
// resources.
type ResourceDeployer interface {
	// Do is used to create/delete a resource with kubectl.
	Do(action kubectlAction) error
}

// yamlResource reads a YAML file and creates/deletes the resource with
// kubectl.
type yamlResource struct {
	filename string

	// namespace defaults to cephCSINamespace if not set
	namespace string

	// allowMissing prevents a failure in case the file is missing.
	allowMissing bool
}

func (yr *yamlResource) Do(action kubectlAction) error {
	data, err := os.ReadFile(yr.filename)
	if err != nil {
		if os.IsNotExist(err) && yr.allowMissing {
			return nil
		}

		return fmt.Errorf("failed to read content from %q: %w", yr.filename, err)
	}

	ns := cephCSINamespace
	if yr.namespace != "" {
		ns = yr.namespace
	}

	err = retryKubectlInput(ns, action, string(data), deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to %s resource %q: %w", action, yr.filename, err)
	}

	return nil
}

// yamlResourceNamespaced takes a filename and calls
// replaceNamespaceInTemplate() on it. There are several options for adjusting
// templates, each has their own comment.
type yamlResourceNamespaced struct {
	filename  string
	namespace string

	// set the number of replicas in a Deployment to 1.
	oneReplica bool

	// enable topology support (for RBD)
	enableTopology bool
	domainLabel    string
}

func (yrn *yamlResourceNamespaced) Do(action kubectlAction) error {
	data, err := replaceNamespaceInTemplate(yrn.filename)
	if err != nil {
		return fmt.Errorf("failed to read content from %q: %w", yrn.filename, err)
	}

	if yrn.oneReplica {
		data = oneReplicaDeployYaml(data)
	}

	if yrn.enableTopology {
		data = enableTopologyInTemplate(data)
	}

	if yrn.domainLabel != "" {
		data = addTopologyDomainsToDSYaml(data, yrn.domainLabel)
	}

	err = retryKubectlInput(yrn.namespace, action, data, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to %s resource %q in namespace %q: %w", action, yrn.filename, yrn.namespace, err)
	}

	return nil
}

type rookNFSResource struct {
	f           *framework.Framework
	modules     []string
	orchBackend string
}

func (rnr *rookNFSResource) Do(action kubectlAction) error {
	if action != kubectlCreate {
		// we won't disabled modules
		return nil
	}

	for _, module := range rnr.modules {
		cmd := fmt.Sprintf("ceph mgr module enable %s", module)
		_, _, err := execCommandInToolBoxPod(rnr.f, cmd, rookNamespace)
		if err != nil {
			// depending on the Ceph/Rook version, modules are
			// enabled by default
			framework.Logf("enabling module %q failed: %v", module, err)
		}
	}

	if rnr.orchBackend != "" {
		// this is not required for all Rook versions, allow failing
		cmd := fmt.Sprintf("ceph orch set backend %s", rnr.orchBackend)
		_, _, err := execCommandInToolBoxPod(rnr.f, cmd, rookNamespace)
		if err != nil {
			framework.Logf("setting orch backend %q failed: %v", rnr.orchBackend, err)
		}
	}

	return nil
}

func waitForDeploymentUpdateScale(
	c kubernetes.Interface,
	ns,
	deploymentName string,
	scale *autoscalingv1.Scale,
	timeout int,
) error {
	t := time.Duration(timeout) * time.Minute
	start := time.Now()
	err := wait.PollUntilContextTimeout(context.TODO(), poll, t, true, func(ctx context.Context) (bool, error) {
		scaleResult, upsErr := c.AppsV1().Deployments(ns).UpdateScale(ctx,
			deploymentName, scale, metav1.UpdateOptions{})
		if upsErr != nil {
			if isRetryableAPIError(upsErr) {
				return false, nil
			}
			framework.Logf(
				"Deployment UpdateScale %s/%s has not completed yet (%d seconds elapsed)",
				ns, deploymentName, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("error update scale deployment %s/%s: %w", ns, deploymentName, upsErr)
		}
		if scaleResult.Spec.Replicas != scale.Spec.Replicas {
			framework.Logf("scale result not matching for deployment %s/%s, desired scale %d, got %d",
				ns, deploymentName, scale.Spec.Replicas, scaleResult.Spec.Replicas)

			return false, fmt.Errorf("error scale not matching in deployment %s/%s: %w", ns, deploymentName, upsErr)
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed update scale deployment %s/%s: %w", ns, deploymentName, err)
	}

	return nil
}

func waitForDeploymentUpdate(
	c kubernetes.Interface,
	deployment *appsv1.Deployment,
	timeout int,
) error {
	t := time.Duration(timeout) * time.Minute
	start := time.Now()
	err := wait.PollUntilContextTimeout(context.TODO(), poll, t, true, func(ctx context.Context) (bool, error) {
		_, upErr := c.AppsV1().Deployments(deployment.Namespace).Update(
			ctx, deployment, metav1.UpdateOptions{})
		if upErr != nil {
			if isRetryableAPIError(upErr) {
				return false, nil
			}
			framework.Logf(
				"Deployment Update %s/%s has not completed yet (%d seconds elapsed)",
				deployment.Namespace, deployment.Name, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("error updating deployment %s/%s: %w",
				deployment.Namespace, deployment.Name, upErr)
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed update deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}

	return nil
}

// contains check if slice contains string.
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}

	return false
}

func waitForContainersArgsUpdate(
	c kubernetes.Interface,
	ns,
	deploymentName,
	key,
	value string,
	containers []string,
	timeout int,
) error {
	framework.Logf("waiting for deployment updates %s/%s", ns, deploymentName)
	ctx := context.TODO()

	// wait for the deployment to be available
	err := waitForDeploymentInAvailableState(c, deploymentName, ns, deployTimeout)
	if err != nil {
		return fmt.Errorf("deployment %s/%s did not become available yet: %w", ns, deploymentName, err)
	}

	// Scale down to 0.
	scale, err := c.AppsV1().Deployments(ns).GetScale(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get scale deployment %s/%s: %w", ns, deploymentName, err)
	}
	count := scale.Spec.Replicas
	scale.ResourceVersion = "" // indicate the scale update should be unconditional
	scale.Spec.Replicas = 0
	err = waitForDeploymentUpdateScale(c, ns, deploymentName, scale, timeout)
	if err != nil {
		return err
	}

	// Update deployment.
	deployment, err := c.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get deployment %s/%s: %w", ns, deploymentName, err)
	}
	cid := deployment.Spec.Template.Spec.Containers // cid: read as containers in deployment
	for i := range cid {
		if contains(containers, cid[i].Name) {
			match := false
			for j, ak := range cid[i].Args {
				if ak == key {
					// do replacement of value
					match = true
					cid[i].Args[j] = fmt.Sprintf("--%s=%s", key, value)

					break
				}
			}
			if !match {
				// append a new key value
				cid[i].Args = append(cid[i].Args, fmt.Sprintf("--%s=%s", key, value))
			}
			deployment.Spec.Template.Spec.Containers[i].Args = cid[i].Args
		}
	}
	// clear creationTimestamp, generation, resourceVersion, and uid
	deployment.CreationTimestamp = metav1.Time{}
	deployment.Generation = 0
	deployment.ResourceVersion = "0"
	deployment.UID = ""
	err = waitForDeploymentUpdate(c, deployment, timeout)
	if err != nil {
		return err
	}

	// Scale up to count.
	scale.Spec.Replicas = count
	err = waitForDeploymentUpdateScale(c, ns, deploymentName, scale, timeout)
	if err != nil {
		return err
	}

	// wait for scale to become count
	t := time.Duration(timeout) * time.Minute
	start := time.Now()
	err = wait.PollUntilContextTimeout(ctx, poll, t, true, func(ctx context.Context) (bool, error) {
		deploy, getErr := c.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
		if getErr != nil {
			if isRetryableAPIError(getErr) {
				return false, nil
			}
			framework.Logf(
				"Deployment Get %s/%s has not completed yet (%d seconds elapsed)",
				ns, deploymentName, int(time.Since(start).Seconds()))

			return false, fmt.Errorf("error getting deployment %s/%s: %w", ns, deploymentName, getErr)
		}
		if deploy.Status.Replicas != count {
			framework.Logf("Expected deployment %s/%s replicas %d, got %d", ns, deploymentName, count, deploy.Status.Replicas)

			return false, fmt.Errorf("error expected deployment %s/%s replicas %d, got %d",
				ns, deploymentName, count, deploy.Status.Replicas)
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed getting deployment %s/%s: %w", ns, deploymentName, err)
	}

	return nil
}
