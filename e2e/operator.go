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
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
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
			driverContainers: []string{cephFSContainerName},
		},
	}
}

func (r *OperatorDeployment) getPodSelector() string {
	return fmt.Sprintf("app in (%s, %s, %s, %s, %s)", helmRBDPodsLabel, helmCephFSPodsLabel, helmNFSPodsLabel,
		r.deploymentName, r.daemonsetName)
}

func (OperatorDeployment) setClusterName(value string) error {
	command := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=merge",
		"-p",
		fmt.Sprintf(`{"spec": {"driverSpecDefaults": {"clusterName": %q}}}`, value),
	}

	// Patch the operator config
	err := retryKubectlArgs(cephCSINamespace, kubectlPatch, deployTimeout, command...)
	if err != nil {
		return fmt.Errorf("failed to set cluster name: %w", err)
	}

	return nil
}

func (r OperatorDeployment) setEnableFencing(enable bool) error {
	oldGeneration, err := getDeploymentGeneration(r.clientSet, r.deploymentName, cephCSINamespace)
	if err != nil {
		return fmt.Errorf("failed to get deployment generation before patching: %w", err)
	}

	command := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=merge",
		"-p",
		fmt.Sprintf(`{"spec": {"driverSpecDefaults": {"enableFencing": %t}}}`, enable),
	}

	err = retryKubectlArgs(cephCSINamespace, kubectlPatch, deployTimeout, command...)
	if err != nil {
		return fmt.Errorf("failed to set enable fencing: %w", err)
	}

	changed, err := waitForDeploymentGenerationChange(r.clientSet, r.deploymentName, cephCSINamespace, oldGeneration)
	if err != nil {
		return fmt.Errorf("error checking deployment %s generation: %w", r.deploymentName, err)
	}

	if !changed {
		return nil
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

func (OperatorDeployment) setDomainLabels(labels []string) error {
	// Define the patch operations
	patchOps := []map[string]interface{}{
		{"op": "add", "path": "/spec/driverSpecDefaults/nodePlugin", "value": map[string]interface{}{}},
		{"op": "add", "path": "/spec/driverSpecDefaults/nodePlugin/topology", "value": map[string]interface{}{}},
		{"op": "add", "path": "/spec/driverSpecDefaults/nodePlugin/topology/domainLabels", "value": labels},
	}

	// Serialize to JSON
	patchJSON, err := json.Marshal(patchOps)
	if err != nil {
		return fmt.Errorf("failed to marshal patch JSON: %w", err)
	}

	command := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=json",
		"-p",
		string(patchJSON),
	}

	// Patch the operator config
	err = retryKubectlArgs(cephCSINamespace, kubectlPatch, deployTimeout, command...)
	if err != nil {
		return fmt.Errorf("failed to set domain labels: %w", err)
	}

	return nil
}
