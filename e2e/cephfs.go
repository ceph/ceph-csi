/*
Copyright 2018 The Ceph-CSI Authors.

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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo" // nolint

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	cephfsProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephfsNodePlugin      = "csi-cephfsplugin.yaml"
	cephfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	cephfsDeploymentName  = "csi-cephfsplugin-provisioner"
	cephfsDeamonSetName   = "csi-cephfsplugin"
	cephfsDirPath         = "../deploy/cephfs/kubernetes"
	cephfsExamplePath     = "../examples/cephfs/"
)

func updateCephfsDirPath(c clientset.Interface) {
	version := getKubeVersionToDeploy(c)
	cephfsDirPath = fmt.Sprintf("%s/%s/", cephfsDirPath, version)
}

func deployCephfsPlugin() {
	// deploy provisioner
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsProvisioner)
	framework.RunKubectlOrDie("apply", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	// deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsNodePlugin)
	framework.RunKubectlOrDie("apply", "-f", cephfsDirPath+cephfsNodePluginRBAC)
}

func deleteCephfsPlugin() {
	_, err := framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsProvisioner)
	if err != nil {
		e2elog.Logf("failed to delete cephfs provisioner %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to delete cephfs provisioner rbac %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsNodePlugin)
	if err != nil {
		e2elog.Logf("failed to delete cephfs nodeplugin %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to delete cephfs nodeplugin rbac %v", err)
	}
}

var _ = Describe("cephfs", func() {
	f := framework.NewDefaultFramework("cephfs")
	var c clientset.Interface
	// deploy cephfs CSI
	BeforeEach(func() {
		c = f.ClientSet
		updateCephfsDirPath(f.ClientSet)
		createFileSystem(f.ClientSet)
		createConfigMap(cephfsDirPath, f.ClientSet, f)
		deployCephfsPlugin()
		createCephfsSecret(f.ClientSet, f)
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			// log provisoner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)
		}
		deleteCephfsPlugin()
		deleteConfiMap(cephfsDirPath)
		deleteResource(cephfsExamplePath + "secret.yaml")
		deleteResource(cephfsExamplePath + "storageclass.yaml")
		deleteFileSystem()
	})

	Context("Test cephfs CSI", func() {
		It("Test cephfs CSI", func() {
			pvcPath := cephfsExamplePath + "pvc.yaml"
			appPath := cephfsExamplePath + "pod.yaml"

			By("checking provisioner statefulset/deployment is running")
			timeout := time.Duration(deployTimeout) * time.Minute
			var err error
			sts := deployProvAsSTS(f.ClientSet)
			if sts {
				err = framework.WaitForStatefulSetReplicasReady(cephfsDeploymentName, namespace, f.ClientSet, 1*time.Second, timeout)
			} else {
				err = waitForDeploymentComplete(cephfsDeploymentName, namespace, f.ClientSet, deployTimeout)
			}
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets(cephfsDeamonSetName, namespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("create a storage class with pool and a PVC then Bind it to an app", func() {
				createCephfsStorageClass(f.ClientSet, f, true)
				validatePVCAndAppBinding(pvcPath, appPath, 1, false, false, f)
				deleteResource(cephfsExamplePath + "storageclass.yaml")
			})

			createCephfsStorageClass(f.ClientSet, f, false)

			By("create/delete multiple PVCs and Apps", func() {
				validatePVCAndAppBinding(pvcPath, appPath, 10, false, false, f)

			})

			By("create a PVC and Bind it to an app with normal user", func() {

				validateNormalUserPVCAccess(pvcPath, f)
			})

			By("check data persist after recreating pod with same pvc", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					Fail(err.Error())
				}
			})

		})
	})

})
