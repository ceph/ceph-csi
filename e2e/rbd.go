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
	rbdProvisioner     = "csi-rbdplugin-provisioner.yaml"
	rbdProvisionerRBAC = "csi-provisioner-rbac.yaml"
	rbdNodePlugin      = "csi-rbdplugin.yaml"
	rbdNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	configMap          = "csi-config-map.yaml"
	rbdDirPath         = "../deploy/rbd/kubernetes"
	rbdExamplePath     = "../examples/rbd/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	namespace          = "default"
)

func updaterbdDirPath(c clientset.Interface) {
	version := getKubeVersionToDeploy(c)
	rbdDirPath = fmt.Sprintf("%s/%s/", rbdDirPath, version)
}

func deployRBDPlugin() {
	// deploy provisioner
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdProvisioner)
	framework.RunKubectlOrDie("apply", "-f", rbdDirPath+rbdProvisionerRBAC)
	// deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdNodePlugin)
	framework.RunKubectlOrDie("apply", "-f", rbdDirPath+rbdNodePluginRBAC)
}

func deleteRBDPlugin() {
	_, err := framework.RunKubectl("delete", "-f", rbdDirPath+rbdProvisioner)
	if err != nil {
		e2elog.Logf("failed to delete rbd provisioner %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to delete provisioner rbac %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdNodePlugin)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin rbac %v", err)
	}
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("rbd")
	// deploy RBD CSI
	BeforeEach(func() {
		updaterbdDirPath(f.ClientSet)
		createRBDPool()
		createConfigMap(rbdDirPath, f.ClientSet, f)
		deployRBDPlugin()
		createRBDStorageClass(f.ClientSet, f, make(map[string]string))
		createRBDSecret(f.ClientSet, f)

	})

	AfterEach(func() {
		deleteRBDPlugin()
		deleteConfiMap(rbdDirPath)
		deleteRBDPool()
		deleteResource(rbdExamplePath + "secret.yaml")
		deleteResource(rbdExamplePath + "storageclass.yaml")
		deleteResource(rbdExamplePath + "snapshotclass.yaml")
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"
			// rawPvcPath := rbdExamplePath + "raw-block-pvc.yaml"
			// rawAppPath := rbdExamplePath + "raw-block-pod.yaml"
			pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
			appClonePath := rbdExamplePath + "pod-restore.yaml"
			snapshotPath := rbdExamplePath + "snapshot.yaml"

			By("checking provisioner statefulset/deployment is running")
			timeout := time.Duration(deployTimeout) * time.Minute
			var err error
			sts := deployProvAsSTS(f.ClientSet)
			if sts {
				err = framework.WaitForStatefulSetReplicasReady(rbdDeploymentName, namespace, f.ClientSet, 1*time.Second, timeout)
			} else {
				err = waitForDeploymentComplete(rbdDeploymentName, namespace, f.ClientSet, deployTimeout)
			}
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets(rbdDaemonsetName, namespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("create a PVC and Bind it to an app", func() {
				validatePVCAndAppBinding(pvcPath, appPath, 10, true, false, f)
			})

			By("create a PVC and Bind it to an app with normal user", func() {
				validateNormalUserPVCAccess(pvcPath, f)
			})
			// Skipping ext4 FS testing
			By("create a PVC and Bind it to an app with ext4 as the FS ", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, map[string]string{"csi.storage.k8s.io/fstype": "ext4"})
				validatePVCAndAppBinding(pvcPath, appPath, 1, true, false, f)
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, make(map[string]string))
			})

			By("create a PVC clone and Bind it to an app", func() {
				createRBDSnapshotClass(f)
				validateCloneFromSnapshot(pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath, 1, true, false, f)
			})

			// skipped raw pvc test in travis
			// By("create a block type PVC and Bind it to an app", func() {
			// 	validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
			// })

			By("check data persist after recreating pod with same pvc", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("Test unmount after nodeplugin restart", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					Fail(err.Error())
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					Fail(err.Error())
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}

				// delete rbd nodeplugin pods
				err = deletePodWithLabel("app=csi-rbdplugin")
				if err != nil {
					Fail(err.Error())
				}
				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, namespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}
			})
		})
	})

})
