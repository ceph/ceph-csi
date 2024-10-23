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
	"encoding/json"
	"fmt"

	clientset "k8s.io/client-go/kubernetes"
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

func (OperatorDeployment) setEnableMetadata(value bool) error {
	command := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=merge",
		"-p",
		fmt.Sprintf(`{"spec": {"driverSpecDefaults": {"enableMetadata": %t}}}`, value),
	}

	// Patch the operator config
	err := retryKubectlArgs(cephCSINamespace, kubectlPatch, deployTimeout, command...)
	if err != nil {
		return err
	}

	return nil
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
