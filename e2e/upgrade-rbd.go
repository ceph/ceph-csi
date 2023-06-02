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
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:golint // e2e uses By() and other Ginkgo functions
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edebug "k8s.io/kubernetes/test/e2e/framework/debug"
	"k8s.io/pod-security-admission/api"
)

var _ = Describe("RBD Upgrade Testing", func() {
	f := framework.NewDefaultFramework("upgrade-test-rbd")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged
	var (
		// cwd stores the initial working directory.
		cwd string
		c   clientset.Interface
		pvc *v1.PersistentVolumeClaim
		app *v1.Pod
		// checkSum stores the md5sum of a file to verify uniqueness.
		checkSum string
	)
	const (
		pvcSize  = "2Gi"
		appKey   = "app"
		appLabel = "rbd-upgrade-testing"
	)

	// deploy rbd CSI
	BeforeEach(func() {
		if !upgradeTesting || !testRBD {
			Skip("Skipping RBD Upgrade Testing")
		}
		c = f.ClientSet
		if cephCSINamespace != defaultNs {
			err := createNamespace(c, cephCSINamespace)
			if err != nil {
				framework.Failf("failed to create namespace: %v", err)
			}
		}

		// fetch current working directory to switch back
		// when we are done upgrading.
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			framework.Failf("failed to do  getwd: %v", err)
		}

		deployVault(f.ClientSet, deployTimeout)
		err = upgradeAndDeployCSI(upgradeVersion, "rbd")
		if err != nil {
			framework.Failf("failed to upgrade and deploy CSI: %v", err)
		}
		err = createConfigMap(rbdDirPath, f.ClientSet, f)
		if err != nil {
			framework.Failf("failed to create configmap: %v", err)
		}
		err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
		if err != nil {
			framework.Failf("failed to create storageclass: %v", err)
		}
		// create rbd provisioner secret
		key, err := createCephUser(f, keyringRBDProvisionerUsername, rbdProvisionerCaps("", ""))
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringRBDProvisionerUsername, err)
		}
		err = createRBDSecret(f, rbdProvisionerSecretName, keyringRBDProvisionerUsername, key)
		if err != nil {
			framework.Failf("failed to create provisioner secret: %v", err)
		}
		// create rbd plugin secret
		key, err = createCephUser(f, keyringRBDNodePluginUsername, rbdNodePluginCaps("", ""))
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringRBDNodePluginUsername, err)
		}
		err = createRBDSecret(f, rbdNodePluginSecretName, keyringRBDNodePluginUsername, key)
		if err != nil {
			framework.Failf("failed to create node secret: %v", err)
		}
		err = createRBDSnapshotClass(f)
		if err != nil {
			framework.Failf("failed to create snapshotclass: %v", err)
		}

		err = createNodeLabel(f, nodeRegionLabel, regionValue)
		if err != nil {
			framework.Failf("failed to create node label: %v", err)
		}
		err = createNodeLabel(f, nodeZoneLabel, zoneValue)
		if err != nil {
			framework.Failf("failed to create node label: %v", err)
		}
	})
	AfterEach(func() {
		if !testRBD || !upgradeTesting {
			Skip("Skipping RBD Upgrade Testing")
		}
		if CurrentSpecReport().Failed() {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-rbd", c)
			// log provisoner
			logsCSIPods("app=csi-rbdplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-rbdplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), c, cephCSINamespace)
		}

		err := deleteConfigMap(rbdDirPath)
		if err != nil {
			framework.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(rbdExamplePath + "storageclass.yaml")
		if err != nil {
			framework.Failf("failed to delete storageclass: %v", err)
		}
		err = deleteResource(rbdExamplePath + "snapshotclass.yaml")
		if err != nil {
			framework.Failf("failed to delete snapshotclass: %v", err)
		}
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err = deleteNamespace(c, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to delete namespace: %v", err)
				}
			}
		}
		err = deleteNodeLabel(c, nodeRegionLabel)
		if err != nil {
			framework.Failf("failed to delete node label: %v", err)
		}
		err = deleteNodeLabel(c, nodeZoneLabel)
		if err != nil {
			framework.Failf("failed to delete node label: %v", err)
		}
	})

	Context("Test RBD CSI", func() {
		if !testRBD || !upgradeTesting {
			return
		}

		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(f.ClientSet, rbdDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment %s: %v", rbdDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for daemonset %s: %v", rbdDaemonsetName, err)
				}
			})

			By("upgrade to latest changes and verify app re-mount", func() {
				// TODO: fetch pvc size from spec.
				var err error
				label := make(map[string]string)
				data := "check data persists"

				pvc, err = loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load pvc: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err = loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				label[appKey] = appLabel
				app.Namespace = f.UniqueName
				app.Labels = label
				pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				// fetch the path where volume is mounted.
				mountPath := app.Spec.Containers[0].VolumeMounts[0].MountPath
				filePath := filepath.Join(mountPath, "testClone")

				// create a test file at the mountPath.
				_, stdErr := execCommandInPodAndAllowFail(
					f,
					fmt.Sprintf("echo %s > %s", data, filePath),
					app.Namespace,
					&opt)
				if stdErr != "" {
					framework.Failf("failed to write data to a file %s", stdErr)
				}

				// force an immediate write of all cached data to disk.
				_, stdErr = execCommandInPodAndAllowFail(f, fmt.Sprintf("sync %s", filePath), app.Namespace, &opt)
				if stdErr != "" {
					framework.Failf("failed to sync data to a disk %s", stdErr)
				}

				opt = metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", appLabel),
				}
				framework.Logf("Calculating checksum of %s", filePath)
				checkSum, err = calculateSHA512sum(f, app, filePath, &opt)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}

				// Create snapshot of the pvc
				snapshotPath := rbdExamplePath + "snapshot.yaml"
				snap := getSnapshot(snapshotPath)
				snap.Name = "rbd-pvc-snapshot"
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot %v", err)
				}

				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}
				deleteRBDPlugin()

				err = os.Chdir(cwd)
				if err != nil {
					framework.Failf("failed to change directory: %v", err)
				}

				deployRBDPlugin()

				err = waitForDeploymentComplete(f.ClientSet, rbdDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for upgraded deployment %s: %v", rbdDeploymentName, err)
				}

				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for upgraded daemonset %s: %v", rbdDaemonsetName, err)
				}

				// validate if the app gets bound to a pvc created by
				// an earlier release.
				app.Labels = label
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
			})

			By("Create clone from a snapshot", func() {
				pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
				appClonePath := rbdExamplePath + "pod-restore.yaml"
				label := make(map[string]string)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load pvc: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				pvcClone.Spec.DataSource.Name = "rbd-pvc-snapshot"
				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				label[appKey] = "validate-snap-clone"
				appClone.Namespace = f.UniqueName
				appClone.Name = "app-clone-from-snap"
				appClone.Labels = label
				err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				mountPath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath
				testFilePath := filepath.Join(mountPath, "testClone")
				newCheckSum, err := calculateSHA512sum(f, appClone, testFilePath, &opt)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}
				if strings.Compare(newCheckSum, checkSum) != 0 {
					framework.Failf(
						"The checksum of files did not match, expected %s received %s",
						checkSum,
						newCheckSum)
				}
				framework.Logf("The checksum of files matched")

				// delete cloned pvc and pod
				err = deletePVCAndApp("", f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete pvc and application: %v", err)
				}
			})

			By("Create clone from existing PVC", func() {
				pvcSmartClonePath := rbdExamplePath + "pvc-clone.yaml"
				appSmartClonePath := rbdExamplePath + "pod-clone.yaml"
				label := make(map[string]string)

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load pvc: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				appClone, err := loadApp(appSmartClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				label[appKey] = "validate-clone"
				appClone.Namespace = f.UniqueName
				appClone.Name = "appclone"
				appClone.Labels = label
				err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				mountPath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath
				testFilePath := filepath.Join(mountPath, "testClone")
				newCheckSum, err := calculateSHA512sum(f, appClone, testFilePath, &opt)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}
				if strings.Compare(newCheckSum, checkSum) != 0 {
					framework.Failf(
						"The checksum of files did not match, expected %s received %s",
						checkSum,
						newCheckSum)
				}
				framework.Logf("The checksum of files matched")

				// delete cloned pvc and pod
				err = deletePVCAndApp("", f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete pvc and application: %v", err)
				}
			})

			By("Resize pvc and verify expansion", func() {
				pvcExpandSize := "5Gi"
				label := make(map[string]string)

				label[appKey] = appLabel
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				var err error
				pvc, err = getPersistentVolumeClaim(f.ClientSet, pvc.Namespace, pvc.Name)
				if err != nil {
					framework.Failf("failed to get pvc: %v", err)
				}

				// resize PVC
				err = expandPVCSize(f.ClientSet, pvc, pvcExpandSize, deployTimeout)
				if err != nil {
					framework.Failf("failed to expand pvc: %v", err)
				}
				// wait for application pod to come up after resize
				err = waitForPodInRunningState(app.Name, app.Namespace, f.ClientSet, deployTimeout, noError)
				if err != nil {
					framework.Failf("timeout waiting for pod to be in running state: %v", err)
				}
				// validate if resize is successful.
				err = checkDirSize(app, f, &opt, pvcExpandSize)
				if err != nil {
					framework.Failf("failed to check directory size: %v", err)
				}
			})

			By("delete pvc and app", func() {
				err := deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete pvc and application: %v", err)
				}
			})
			// delete RBD provisioner secret
			err := deleteCephUser(f, keyringRBDProvisionerUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringRBDProvisionerUsername, err)
			}
			// delete RBD plugin secret
			err = deleteCephUser(f, keyringRBDNodePluginUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringRBDNodePluginUsername, err)
			}
		})
	})
})
