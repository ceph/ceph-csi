/*
Copyright 2026 The Ceph-CSI Authors.

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
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	nvmeofProvisioner     = "csi-nvmeofplugin-provisioner.yaml"
	nvmeofProvisionerSCC  = "csi-provisioner-scc.yaml"
	nvmeofProvisionerRBAC = "csi-provisioner-rbac.yaml"
	nvmeofNodePlugin      = "csi-nvmeofplugin.yaml"
	nvmeofNodePluginSCC   = "csi-nodeplugin-scc.yaml"
	nvmeofNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	nvmeofDirPath         = deployPath + "/nvmeof/kubernetes/"
	nvmeofExamplePath     = examplePath + "/nvmeof/"
	nvmeofDeploymentName  = "csi-nvmeofplugin-provisioner"
	nvmeofDaemonsetName   = "csi-nvmeofplugin"
	nvmeofContainerName   = "csi-nvmeofplugin"
)

func createORDeleteNVMeoFResources(action kubectlAction) {
	cephConfigFile := getConfigFile(cephConfconfigMap, deployPath, examplePath)
	var resources []ResourceDeployer

	if isOpenShift {
		resources = append(resources,
			&yamlResource{
				filename: nvmeofDirPath + nvmeofProvisionerSCC,
				replace: map[string]string{
					":default:": ":" + cephCSINamespace + ":",
				},
			},
			&yamlResource{
				filename: nvmeofDirPath + nvmeofNodePluginSCC,
				replace: map[string]string{
					":default:": ":" + cephCSINamespace + ":",
				},
			},
		)
	}

	resources = append(resources,
		// shared resources
		&yamlResource{
			filename: nvmeofDirPath + csiDriverObject,
		},
		&yamlResource{
			filename:     cephConfigFile,
			allowMissing: true,
		},
		// dependencies for provisioner
		&yamlResourceNamespaced{
			filename:  nvmeofDirPath + nvmeofProvisionerRBAC,
			namespace: cephCSINamespace,
		},
		// the provisioner itself
		&yamlResourceNamespaced{
			filename:   nvmeofDirPath + nvmeofProvisioner,
			namespace:  cephCSINamespace,
			oneReplica: true,
		},
		// dependencies for the node-plugin
		&yamlResourceNamespaced{
			filename:  nvmeofDirPath + nvmeofNodePluginRBAC,
			namespace: cephCSINamespace,
		},
		// the node-plugin itself
		&yamlResourceNamespaced{
			filename:  nvmeofDirPath + nvmeofNodePlugin,
			namespace: cephCSINamespace,
		},
	)

	for _, r := range resources {
		err := r.Do(action)
		Expect(err).ShouldNot(HaveOccurred())
	}
}

func deployNVMeoFPlugin(f *framework.Framework, deployTimeout int) {
	err := createConfigMap(nvmeofDirPath, f.ClientSet, f)
	Expect(err).ShouldNot(HaveOccurred())

	createORDeleteNVMeoFResources(kubectlCreate)

	err = waitForDeploymentComplete(
		f.ClientSet,
		nvmeofDeploymentName,
		cephCSINamespace,
		deployTimeout,
	)
	Expect(err).ShouldNot(HaveOccurred())

	err = waitForDaemonSets(
		nvmeofDaemonsetName,
		cephCSINamespace,
		f.ClientSet,
		deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())
}

func deleteNVMeoFPlugin() {
	createORDeleteNVMeoFResources(kubectlDelete)

	err := deleteConfigMap(nvmeofDirPath)
	Expect(err).ShouldNot(HaveOccurred())
}
