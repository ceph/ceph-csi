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
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

	nvmeofKeyringProvisionerUsername = "csi-nvmeof-provisioner"
	nvmeofProvisionerSecretName      = "cephcsi-nvmeof-provisioner"
	nvmeofKeyringNodePluginUsername  = "csi-nvmeof-nodeplugin"
	nvmeofNodePluginSecretName       = "cephcsi-nvmeof-nodeplugin"
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

func createNVMeoFStorageClass(
	f *framework.Framework,
	name string,
	scOptions, parameters map[string]string,
	policy v1.PersistentVolumeReclaimPolicy,
) {
	sc, err := getStorageClass(nvmeofExamplePath + "/storageclass.yaml")
	Expect(err).ShouldNot(HaveOccurred())

	if name != "" {
		sc.Name = name
	}

	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = nvmeofProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-publish-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-publish-secret-name"] = nvmeofProvisionerSecretName
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = nvmeofProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = nvmeofNodePluginSecretName

	fsID, err := getClusterID(f)
	Expect(err).ShouldNot(HaveOccurred())
	sc.Parameters["clusterID"] = fsID

	for k, v := range parameters {
		sc.Parameters[k] = v
		// if any values are empty remove it from the map
		if v == "" {
			delete(sc.Parameters, k)
		}
	}

	sc.Parameters["subsystemNQN"] = "nqn.2025-08.io.ceph:" + f.UniqueName

	gwHost, gwIP := getNVMeofGateway(f.ClientSet)
	framework.Logf("configuring StorageClass for gateway %q at %s", gwHost, gwIP)

	sc.Parameters["nvmeofGatewayAddress"] = gwIP

	listeners := []struct {
		Hostname string `json:"hostname,omitempty"`
		Address  string `json:"address"`
		Port     int    `json:"port"`
	}{
		{
			Hostname: gwHost, // FIXME: is this required?
			Address:  gwIP,
			Port:     4420,
		},
		// only one gateway, so one listener
	}
	rawListeners, err := json.Marshal(listeners)
	Expect(err).ShouldNot(HaveOccurred())
	sc.Parameters["listeners"] = string(rawListeners)

	if scOptions["volumeBindingMode"] == "WaitForFirstConsumer" {
		value := storagev1.VolumeBindingWaitForFirstConsumer
		sc.VolumeBindingMode = &value
	}

	// comma separated mount options
	if opt, ok := scOptions[rbdMountOptions]; ok {
		mOpt := strings.Split(opt, ",")
		sc.MountOptions = append(sc.MountOptions, mOpt...)
	}
	sc.ReclaimPolicy = &policy

	timeout := time.Duration(deployTimeout) * time.Minute

	err = wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err = f.ClientSet.StorageV1().StorageClasses().Create(ctx, &sc, metav1.CreateOptions{})
		if err != nil {
			framework.Logf("error creating StorageClass %q: %v", sc.Name, err)
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to create StorageClass %q: %w", sc.Name, err)
		}

		return true, nil
	})
	Expect(err).ShouldNot(HaveOccurred())
}

func deleteNVMeofStorageClass(f *framework.Framework, scName string) {
	err := f.ClientSet.StorageV1().StorageClasses().Delete(context.TODO(), scName, metav1.DeleteOptions{})
	if err != nil && apierrs.IsNotFound(err) {
		err = nil
	}
	Expect(err).ShouldNot(HaveOccurred())
}

func createNVMeoFCredentials(f *framework.Framework) {
	key, err := createCephUser(f, nvmeofKeyringProvisionerUsername, rbdProvisionerCaps("", ""))
	Expect(err).ShouldNot(HaveOccurred())

	err = createRBDSecret(f, nvmeofProvisionerSecretName, nvmeofKeyringProvisionerUsername, key)
	Expect(err).ShouldNot(HaveOccurred())

	key, err = createCephUser(f, nvmeofKeyringNodePluginUsername, rbdNodePluginCaps("", ""))
	Expect(err).ShouldNot(HaveOccurred())

	err = createRBDSecret(f, nvmeofNodePluginSecretName, nvmeofKeyringNodePluginUsername, key)
	Expect(err).ShouldNot(HaveOccurred())
}
