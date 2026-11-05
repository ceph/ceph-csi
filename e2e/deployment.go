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
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

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
	testutils "k8s.io/kubernetes/test/utils"
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

	return e2epod.Exec(f.TContext(context.Background()), podOpt)
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

		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}

		// During a Recreate rollout the old pod stays Ready until it
		// terminates, so comparing Replicas to ReadyReplicas alone returns
		// before the new pods exist. Require the controller to have observed
		// the current generation and all replicas to be updated and ready.
		status := deployment.Status
		if status.ObservedGeneration < deployment.Generation ||
			status.UpdatedReplicas != desired ||
			status.Replicas != desired ||
			status.ReadyReplicas != desired ||
			status.AvailableReplicas != desired {
			framework.Logf(
				"deployment %s/%s not ready: generation %d (observed %d), "+
					"desired %d, replicas %d, updated %d, ready %d, available %d",
				ns, name, deployment.Generation, status.ObservedGeneration,
				desired, status.Replicas, status.UpdatedReplicas,
				status.ReadyReplicas, status.AvailableReplicas)
			reason = fmt.Sprintf("deployment status: %#v", status.String())

			return false, nil
		}

		return true, nil
	})

	if wait.Interrupted(err) {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match desired state: %w", name, err)
	}

	return nil
}

// waitForCSI waits for a CSI deployment and daemonset to be ready
// concurrently.
func waitForCSI(
	clientSet kubernetes.Interface,
	deploymentName, daemonsetName, ns string,
	timeout int,
) error {
	var eg errgroup.Group
	eg.Go(func() error {
		return waitForDeploymentComplete(
			clientSet, deploymentName, ns, timeout)
	})
	eg.Go(func() error {
		return waitForDaemonSets(
			daemonsetName, ns, clientSet, timeout)
	})
	return eg.Wait()
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

	// generic replacing of key with value
	replace map[string]string
}

func (yr *yamlResource) Do(action kubectlAction) error {
	rawData, err := os.ReadFile(yr.filename)
	if err != nil {
		if os.IsNotExist(err) && yr.allowMissing {
			return nil
		}

		return fmt.Errorf("failed to read content from %q: %w", yr.filename, err)
	}

	data := string(rawData)
	for key, value := range yr.replace {
		data = strings.ReplaceAll(data, key, value)
	}

	ns := cephCSINamespace
	if yr.namespace != "" {
		ns = yr.namespace
	}

	err = retryKubectlInput(ns, action, data, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to %s resource %q: %w", action, yr.filename, err)
	}

	return nil
}

// yamlResourceNamespaced takes a filename and calls
// replaceNamespaceInTemplate() on it. There are several options for adjusting
// templates, each has their own comment.
type yamlResourceNamespaced struct {
	filename            string
	namespace           string
	domainLabel         string
	crushLocationLabels string

	// set the number of replicas in a Deployment to 1.
	oneReplica bool

	// enable read affinity support (for RBD)
	enableReadAffinity bool

	// enable fencing
	enableFencing bool
}

func (yrn *yamlResourceNamespaced) Do(action kubectlAction) error {
	data, err := replaceNamespaceInTemplate(yrn.filename)
	if err != nil {
		return fmt.Errorf("failed to read content from %q: %w", yrn.filename, err)
	}

	data = replaceLogLevelInTemplate(data)

	// disable VGS alpha feature, TODO: remove this in next release (3.14.0)
	data = disableVGSAlphaCLIArg(data)

	if yrn.oneReplica {
		data = oneReplicaDeployYaml(data)
	}

	if yrn.domainLabel != "" {
		data = addTopologyDomainsToDSYaml(data, yrn.domainLabel)
	}

	if yrn.enableReadAffinity {
		data = enableReadAffinityInTemplate(data)
	}

	if yrn.enableFencing {
		data = enableFencingInTemplate(data)
	}

	if yrn.crushLocationLabels != "" {
		data = addCrsuhLocationLabels(data, yrn.crushLocationLabels)
	}

	err = retryKubectlInput(yrn.namespace, action, data, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to %s resource %q in namespace %q: %w", action, yrn.filename, yrn.namespace, err)
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
		if slices.Contains(containers, cid[i].Name) {
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

// updateDeploymentContainers scales the deployment down to zero, applies mutate
// to the pod template's container list, scales it back to the original replica
// count and waits for the rollout to complete. It is used to add or remove a
// single container without touching the rest of the deployment. If any step
// after scale-down fails, it rolls the deployment back to the original container
// list and replica count so the shared provisioner is not left broken for later
// tests.
func updateDeploymentContainers(
	c kubernetes.Interface,
	ns,
	deploymentName string,
	mutate func([]v1.Container) []v1.Container,
	timeout int,
) error {
	ctx := context.TODO()

	err := waitForDeploymentInAvailableState(c, deploymentName, ns, timeout)
	if err != nil {
		return fmt.Errorf("deployment %s/%s did not become available yet: %w", ns, deploymentName, err)
	}

	// Capture the original pod template containers and replica count before any
	// mutation, so a failure after scale-down can restore the deployment instead
	// of leaving the shared provisioner at zero replicas or with a container
	// removed.
	original, err := c.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get deployment %s/%s: %w", ns, deploymentName, err)
	}
	originalContainers := make([]v1.Container, len(original.Spec.Template.Spec.Containers))
	copy(originalContainers, original.Spec.Template.Spec.Containers)

	// Scale down to 0 so the pod template can be updated safely.
	scale, err := c.AppsV1().Deployments(ns).GetScale(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get scale deployment %s/%s: %w", ns, deploymentName, err)
	}
	count := scale.Spec.Replicas
	// Register rollback before scaling down: an API error can occur after the
	// server has already applied the scale change.
	rollback := true
	defer func() {
		if rollback {
			if rbErr := restoreDeploymentState(c, ns, deploymentName, originalContainers, count, timeout); rbErr != nil {
				framework.Logf("failed to roll back deployment %s/%s: %v", ns, deploymentName, rbErr)
			}
		}
	}()

	scale.ResourceVersion = "" // indicate the scale update should be unconditional
	scale.Spec.Replicas = 0
	err = waitForDeploymentUpdateScale(c, ns, deploymentName, scale, timeout)
	if err != nil {
		return err
	}

	// Wait until the deployment reports the scale-down and, crucially, until the
	// old pods are actually gone. DeploymentStatus.Replicas can reach zero while
	// pods are still terminating, so a lingering container (for example the PV
	// controller) could keep reconciling while the test runs.
	err = waitForDeploymentRollout(c, ns, deploymentName, timeout)
	if err != nil {
		return err
	}
	err = waitForDeploymentPodsGone(c, ns, deploymentName, timeout)
	if err != nil {
		return err
	}

	// Update the container list.
	deployment, err := c.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get deployment %s/%s: %w", ns, deploymentName, err)
	}
	deployment.Spec.Template.Spec.Containers = mutate(deployment.Spec.Template.Spec.Containers)
	// clear creationTimestamp, generation, resourceVersion, and uid
	deployment.CreationTimestamp = metav1.Time{}
	deployment.Generation = 0
	deployment.ResourceVersion = "0"
	deployment.UID = ""
	err = waitForDeploymentUpdate(c, deployment, timeout)
	if err != nil {
		return err
	}

	// Scale back up to the original count and wait for the rollout to fully
	// complete (latest generation observed, all replicas updated and ready).
	scale.Spec.Replicas = count
	err = waitForDeploymentUpdateScale(c, ns, deploymentName, scale, timeout)
	if err != nil {
		return err
	}
	err = waitForDeploymentRollout(c, ns, deploymentName, timeout)
	if err != nil {
		return err
	}

	rollback = false

	return nil
}

// waitForDeploymentPodsGone waits until no pods matching the deployment's
// selector remain in the namespace. DeploymentStatus.Replicas can reach zero
// while pods are still terminating, so callers that must be sure the old pods
// (for example a controller that keeps reconciling) are truly gone use this
// after scaling the deployment down.
func waitForDeploymentPodsGone(c kubernetes.Interface, ns, name string, timeout int) error {
	deployment, err := c.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get deployment %s/%s: %w", ns, name, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return fmt.Errorf("invalid selector for deployment %s/%s: %w", ns, name, err)
	}

	t := time.Duration(timeout) * time.Minute
	start := time.Now()

	return wait.PollUntilContextTimeout(context.TODO(), poll, t, true, func(ctx context.Context) (bool, error) {
		pods, err := c.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("error listing pods for deployment %s/%s: %w", ns, name, err)
		}
		if len(pods.Items) == 0 {
			return true, nil
		}
		framework.Logf("waiting for %d pod(s) of deployment %s/%s to terminate (%d seconds elapsed)",
			len(pods.Items), ns, name, int(time.Since(start).Seconds()))

		return false, nil
	})
}

// restoreDeploymentState restores a deployment's pod template containers and
// replica count. It rolls back a partially applied updateDeploymentContainers so
// a failed test does not leave the shared provisioner broken for later specs.
func restoreDeploymentState(
	c kubernetes.Interface,
	ns,
	deploymentName string,
	containers []v1.Container,
	count int32,
	timeout int,
) error {
	ctx := context.TODO()
	deployment, err := c.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get deployment %s/%s: %w", ns, deploymentName, err)
	}
	deployment.Spec.Template.Spec.Containers = containers
	// clear creationTimestamp, generation, resourceVersion, and uid
	deployment.CreationTimestamp = metav1.Time{}
	deployment.Generation = 0
	deployment.ResourceVersion = "0"
	deployment.UID = ""
	err = waitForDeploymentUpdate(c, deployment, timeout)
	if err != nil {
		return err
	}

	scale, err := c.AppsV1().Deployments(ns).GetScale(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get scale deployment %s/%s: %w", ns, deploymentName, err)
	}
	scale.ResourceVersion = "" // indicate the scale update should be unconditional
	scale.Spec.Replicas = count

	return waitForDeploymentUpdateScale(c, ns, deploymentName, scale, timeout)
}

// waitForDeploymentRollout waits until the deployment's current spec is fully
// rolled out: the latest generation is observed and all replicas are updated,
// present and available. It reuses the upstream testutils.WaitForDeploymentComplete
// check (deploymentutil.DeploymentComplete) and works for scaling both to zero
// and back up.
func waitForDeploymentRollout(c kubernetes.Interface, ns, name string, timeout int) error {
	d, err := c.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error get deployment %s/%s: %w", ns, name, err)
	}

	return testutils.WaitForDeploymentComplete(c, d, framework.Logf, poll, time.Duration(timeout)*time.Minute)
}

// removeDeploymentContainer removes the named container from the deployment and
// returns its spec so restoreDeploymentContainer can add it back later. Returning
// the container keeps each caller's remove/restore pair self-contained, without
// sharing state across tests. It works for any container in any deployment.
func removeDeploymentContainer(
	c kubernetes.Interface,
	ns,
	deploymentName,
	containerName string,
	timeout int,
) (v1.Container, error) {
	var removed *v1.Container
	err := updateDeploymentContainers(c, ns, deploymentName,
		func(containers []v1.Container) []v1.Container {
			out := make([]v1.Container, 0, len(containers))
			for i := range containers {
				if containers[i].Name == containerName {
					cp := containers[i]
					removed = &cp

					continue
				}
				out = append(out, containers[i])
			}

			return out
		}, timeout)
	if err != nil {
		return v1.Container{}, err
	}
	if removed == nil {
		return v1.Container{}, fmt.Errorf("container %q not found in deployment %s/%s", containerName, ns, deploymentName)
	}

	return *removed, nil
}

// restoreDeploymentContainer adds the container returned by
// removeDeploymentContainer back to the deployment. It is a no-op when a
// container with the same name is already present, so it stays idempotent.
func restoreDeploymentContainer(
	c kubernetes.Interface,
	ns,
	deploymentName string,
	container v1.Container,
	timeout int,
) error {
	return updateDeploymentContainers(c, ns, deploymentName,
		func(containers []v1.Container) []v1.Container {
			for i := range containers {
				if containers[i].Name == container.Name {
					return containers // already present
				}
			}

			return append(containers, container)
		}, timeout)
}
