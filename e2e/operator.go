/*
Copyright 2025 The Ceph-CSI Authors.

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
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	cephcsiv1 "github.com/ceph/ceph-csi-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubectl "k8s.io/kubernetes/test/e2e/framework/kubectl"
)

const (
	OperatorConfigName = "ceph-csi-operator-config"
)

type OperatorDeployment struct {
	DriverInfo
}

func NewRBDOperatorDeployment(c clientset.Interface) RBDDeploymentMethod {
	return &OperatorDeployment{
		DriverInfo: DriverInfo{
			clientSet:        c,
			deploymentName:   operatorRBDDeploymentName,
			daemonsetName:    operatorRBDDaemonsetName,
			driverContainers: rbdContainersName,
		},
	}
}

func NewCephFSOperatorDeployment(c clientset.Interface) CephFSDeploymentMethod {
	return &OperatorDeployment{
		DriverInfo: DriverInfo{
			clientSet:        c,
			deploymentName:   operatorCephFSDeploymentName,
			daemonsetName:    operatorCephFSDaemonsetName,
			driverContainers: cephFSContainersName,
		},
	}
}

func (r *OperatorDeployment) getPodSelector() string {
	return fmt.Sprintf("app in (%s, %s)", r.deploymentName, r.daemonsetName)
}

// applyClusterConfig applies all driver settings in a single OperatorConfig
// patch and waits for the resulting rollout. Patching them separately can let
// the generation-change wait observe an earlier patch while a later one is
// still pending, and the first volume then races the pod replacement.
func (r OperatorDeployment) applyClusterConfig(clusterName string, enableFencing bool, domainLabels []string) error {
	current, err := getOperatorConfig(cephCSINamespace)
	if err != nil {
		return err
	}

	if driverSpecMatches(current.Spec.DriverSpecDefaults, clusterName, enableFencing, domainLabels) {
		framework.Logf("operator config already has clusterName %q and enableFencing %t, skipping patch",
			clusterName, enableFencing)

		return r.waitForDriverReady(clusterName, enableFencing, domainLabels)
	}

	oldGeneration, err := getDeploymentGeneration(r.clientSet, r.deploymentName, cephCSINamespace)
	if err != nil {
		return fmt.Errorf("failed to get deployment generation before patching: %w", err)
	}

	// Build the patch from maps instead of cephcsiv1.DriverSpec: the typed
	// struct marshals non-pointer fields such as imagePullPolicy without
	// omitempty, which would reset unrelated settings in the shared config.
	driverDefaults := map[string]interface{}{
		"clusterName":   clusterName,
		"enableFencing": enableFencing,
	}
	if len(domainLabels) > 0 {
		driverDefaults["nodePlugin"] = map[string]interface{}{
			"topology": map[string]interface{}{"domainLabels": domainLabels},
		}
	}
	patch, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{"driverSpecDefaults": driverDefaults},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal operator config patch: %w", err)
	}

	command := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=merge",
		"-p",
		string(patch),
	}
	err = retryKubectlArgs(cephCSINamespace, kubectlPatch, deployTimeout, command...)
	if err != nil {
		return fmt.Errorf("failed to apply operator config: %w", err)
	}

	changed, err := waitForDeploymentGenerationChange(r.clientSet, r.deploymentName, cephCSINamespace, oldGeneration)
	if err != nil {
		return fmt.Errorf("error checking deployment %s generation: %w", r.deploymentName, err)
	}
	if !changed {
		// A change limited to the topology labels only bumps the node
		// DaemonSet, so the deployment generation stays the same.
		framework.Logf("deployment %s/%s generation unchanged after patch, waiting for the workload arguments",
			cephCSINamespace, r.deploymentName)
	}

	return r.waitForDriverReady(clusterName, enableFencing, domainLabels)
}

// waitForDriverReady waits until this driver's workload templates contain the
// requested settings and the resulting rollout has completed. It is used both
// after a patch and when the shared OperatorConfig already matched, since
// another driver may have applied the values while this driver's pods were
// still rolling out.
func (r OperatorDeployment) waitForDriverReady(clusterName string, enableFencing bool, domainLabels []string) error {
	err := r.waitForDriverArgs(clusterName, enableFencing, domainLabels)
	if err != nil {
		return err
	}

	err = waitForCSI(r.clientSet, r.deploymentName, r.daemonsetName, cephCSINamespace, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed waiting for CSI rollout: %w", err)
	}

	err = waitForLeaderLeaseTransfer(r.clientSet, cephCSINamespace)
	if err != nil {
		return fmt.Errorf("failed waiting for leader lease transfer after rollout: %w", err)
	}

	return nil
}

// waitForDriverArgs waits until the controller and node plugin templates
// contain the requested container arguments. Checking the workload templates
// instead of the OperatorConfig avoids skipping the rollout wait while the
// operator has not reconciled this driver yet.
func (r OperatorDeployment) waitForDriverArgs(clusterName string, enableFencing bool, domainLabels []string) error {
	// The controller plugin gets the cluster name and fencing args, the node
	// plugin gets fencing and topology labels (the operator does not pass the
	// cluster name to node plugin containers).
	controllerArgs := []string{fmt.Sprintf("--enable-fencing=%t", enableFencing)}
	if clusterName != "" {
		controllerArgs = append(controllerArgs, "--clustername="+clusterName)
	}
	nodeArgs := []string{fmt.Sprintf("--enable-fencing=%t", enableFencing)}
	if len(domainLabels) > 0 {
		nodeArgs = append(nodeArgs, "--domainlabels="+strings.Join(domainLabels, ","))
	}

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		deploy, err := r.clientSet.AppsV1().Deployments(cephCSINamespace).Get(ctx, r.deploymentName, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}

			return false, err
		}

		daemonset, err := r.clientSet.AppsV1().DaemonSets(cephCSINamespace).Get(ctx, r.daemonsetName, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}

			return false, err
		}

		missing := missingContainerArgs(deploy.Spec.Template.Spec.Containers, controllerArgs)
		if len(missing) == 0 {
			missing = missingContainerArgs(daemonset.Spec.Template.Spec.Containers, nodeArgs)
		}
		if len(missing) > 0 {
			framework.Logf("deployment %s/%s and daemonset %s/%s still missing args %v, waiting",
				cephCSINamespace, r.deploymentName, cephCSINamespace, r.daemonsetName, missing)

			return false, nil
		}

		return true, nil
	})
}

func missingContainerArgs(containers []v1.Container, wantArgs []string) []string {
	missing := make([]string, 0, len(wantArgs))
	for _, want := range wantArgs {
		found := false
		for i := range containers {
			if slices.Contains(containers[i].Args, want) {
				found = true

				break
			}
		}
		if !found {
			missing = append(missing, want)
		}
	}

	return missing
}

// driverSpecMatches reports whether the OperatorConfig already contains the
// requested driver settings. An empty domainLabels list means the caller does
// not care about the topology labels.
func driverSpecMatches(defaults *cephcsiv1.DriverSpec, clusterName string, enableFencing bool, domainLabels []string) bool {
	if defaults == nil {
		return false
	}

	if defaults.ClusterName == nil || *defaults.ClusterName != clusterName {
		return false
	}

	if defaults.EnableFencing == nil || *defaults.EnableFencing != enableFencing {
		return false
	}

	if len(domainLabels) == 0 {
		return true
	}

	return defaults.NodePlugin != nil && defaults.NodePlugin.Topology != nil &&
		slices.Equal(defaults.NodePlugin.Topology.DomainLabels, domainLabels)
}

func getOperatorConfig(namespace string) (*cephcsiv1.OperatorConfig, error) {
	args := []string{
		"get",
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"-o",
		"json",
	}

	out, err := e2ekubectl.RunKubectl(namespace, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator config: %w", err)
	}

	cfg := &cephcsiv1.OperatorConfig{}
	err = json.Unmarshal([]byte(out), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse operator config: %w", err)
	}

	return cfg, nil
}

func getDeploymentGeneration(c clientset.Interface, name, ns string) (int64, error) {
	d, err := c.AppsV1().Deployments(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get deployment %s/%s: %w", ns, name, err)
	}

	return d.Generation, nil
}

// waitForDeploymentGenerationChange waits briefly for the deployment's
// generation to change from oldGeneration. Returns (true, nil) if a change
// was detected, (false, nil) if the generation did not change within the
// grace period (meaning no rollout is needed), or (false, err) on error.
func waitForDeploymentGenerationChange(c clientset.Interface, name, ns string, oldGeneration int64) (bool, error) {
	const reconcileGracePeriod = 30 * time.Second

	err := wait.PollUntilContextTimeout(context.TODO(), poll, reconcileGracePeriod, true, func(ctx context.Context) (bool, error) {
		d, err := c.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, err
			}

			return false, err
		}

		if d.Generation == oldGeneration {
			framework.Logf("deployment %s/%s: generation still %d, waiting for operator reconciliation", ns, name, oldGeneration)

			return false, nil
		}

		framework.Logf("deployment %s/%s: generation changed %d → %d", ns, name, oldGeneration, d.Generation)

		return true, nil
	})

	if err != nil && (wait.Interrupted(err) || errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "would exceed context deadline")) {
		framework.Logf("deployment %s/%s: generation unchanged after %s, config already applied", ns, name, reconcileGracePeriod)

		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// csiLeasePrefix lists the name prefixes of CSI sidecar leader election
// leases. Only leases matching these prefixes are checked during transfer.
var csiLeasePrefix = []string{
	"external-attacher-leader-",
	"external-provisioner-leader-",
	"external-resizer-",
	"external-snapshotter-leader-",
	"csi-addons-",
}

func isCSILeaderLease(name string) bool {
	for _, prefix := range csiLeasePrefix {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// waitForLeaderLeaseTransfer waits until all CSI sidecar leader election
// leases in the namespace are held by a currently running pod. After a
// Recreate-strategy rollout, old pod leases survive until the TTL (~15s)
// expires — this function polls until the new pod's sidecars acquire them.
func waitForLeaderLeaseTransfer(c clientset.Interface, ns string) error {
	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		pods, err := c.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, err
		}

		runningPodNames := make(map[string]bool)
		for i := range pods.Items {
			if pods.Items[i].Status.Phase == v1.PodRunning && pods.Items[i].DeletionTimestamp == nil {
				// CSI sidecar leader election sanitizes pod names by
				// replacing dots with dashes in holder identities.
				normalized := strings.ReplaceAll(pods.Items[i].Name, ".", "-")
				runningPodNames[normalized] = true
			}
		}

		leases, err := c.CoordinationV1().Leases(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, err
		}

		for i := range leases.Items {
			lease := &leases.Items[i]
			if !isCSILeaderLease(lease.Name) {
				continue
			}

			if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
				framework.Logf("CSI lease %s/%s has no holder, waiting", ns, lease.Name)

				return false, nil
			}

			holder := *lease.Spec.HolderIdentity
			heldByRunningPod := false
			for podName := range runningPodNames {
				if strings.HasPrefix(holder, podName) {
					heldByRunningPod = true

					break
				}
			}

			if !heldByRunningPod {
				framework.Logf("CSI lease %s/%s held by %q (not a running pod), waiting for transfer",
					ns, lease.Name, holder)

				return false, nil
			}
		}

		framework.Logf("all CSI leader leases in %s held by running pods", ns)

		return true, nil
	})
}
