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

var _ = Describe("CephFS Upgrade Testing", func() {
	f := framework.NewDefaultFramework("upgrade-test-cephfs")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged
	var (
		c        clientset.Interface
		pvc      *v1.PersistentVolumeClaim
		pvcClone *v1.PersistentVolumeClaim
		app      *v1.Pod
		appClone *v1.Pod
		// cwd stores the initial working directory.
		cwd string
		err error
		// checkSum stores the md5sum of a file to verify uniqueness.
		checkSum string
		// newCheckSum stores the md5sum of a file in the cloned pvc.
		newCheckSum string
	)
	const (
		pvcSize  = "2Gi"
		appKey   = "app"
		appLabel = "cephfs-upgrade-testing"
	)
	// deploy cephFS CSI
	BeforeEach(func() {
		if !upgradeTesting || !testCephFS {
			Skip("Skipping CephFS Upgrade Test")
		}
		c = f.ClientSet
		if cephCSINamespace != defaultNs {
			err = createNamespace(c, cephCSINamespace)
			if err != nil {
				framework.Failf("failed to create namespace: %v", err)
			}
		}

		// fetch current working directory to switch back
		// when we are done upgrading.
		cwd, err = os.Getwd()
		if err != nil {
			framework.Failf("failed to getwd: %v", err)
		}
		deployVault(f.ClientSet, deployTimeout)
		err = upgradeAndDeployCSI(upgradeVersion, "cephfs")
		if err != nil {
			framework.Failf("failed to upgrade csi: %v", err)
		}
		err = createConfigMap(cephFSDirPath, f.ClientSet, f)
		if err != nil {
			framework.Failf("failed to create configmap: %v", err)
		}
		var key string
		// create cephFS provisioner secret
		key, err = createCephUser(f, keyringCephFSProvisionerUsername, cephFSProvisionerCaps())
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringCephFSProvisionerUsername, err)
		}
		err = createCephfsSecret(f, cephFSProvisionerSecretName, keyringCephFSProvisionerUsername, key)
		if err != nil {
			framework.Failf("failed to create provisioner secret: %v", err)
		}
		// create cephFS plugin secret
		key, err = createCephUser(f, keyringCephFSNodePluginUsername, cephFSNodePluginCaps())
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringCephFSNodePluginUsername, err)
		}
		err = createCephfsSecret(f, cephFSNodePluginSecretName, keyringCephFSNodePluginUsername, key)
		if err != nil {
			framework.Failf("failed to create node secret: %v", err)
		}

		err = createCephFSSnapshotClass(f)
		if err != nil {
			framework.Failf("failed to create snapshotclass: %v", err)
		}
		err = createCephfsStorageClass(f.ClientSet, f, true, nil)
		if err != nil {
			framework.Failf("failed to create storageclass: %v", err)
		}
	})
	AfterEach(func() {
		if !testCephFS || !upgradeTesting {
			Skip("Skipping CephFS Upgrade Test")
		}
		if CurrentSpecReport().Failed() {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-cephfs", c)
			// log provisoner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), c, cephCSINamespace)
		}
		err = deleteConfigMap(cephFSDirPath)
		if err != nil {
			framework.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(cephFSExamplePath + "storageclass.yaml")
		if err != nil {
			framework.Failf("failed to delete storageclass: %v", err)
		}
		err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
		if err != nil {
			framework.Failf("failed to delete storageclass: %v", err)
		}
		deleteVault()
		if deployCephFS {
			deleteCephfsPlugin()
			if cephCSINamespace != defaultNs {
				err = deleteNamespace(c, cephCSINamespace)
				if err != nil {
					if err != nil {
						framework.Failf("failed to delete namespace: %v", err)
					}
				}
			}
		}
	})

	Context("Cephfs Upgrade Test", func() {
		if !upgradeTesting || !testCephFS {
			return
		}

		It("Cephfs Upgrade Test", func() {
			By("checking provisioner deployment is running", func() {
				err = waitForDeploymentComplete(f.ClientSet, cephFSDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment %s: %v", cephFSDeploymentName, err)
				}
			})
			By("checking nodeplugin deamonset pods are running", func() {
				err = waitForDaemonSets(cephFSDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for daemonset %s: %v", cephFSDeamonSetName, err)
				}
			})

			By("upgrade to latest changes and verify app re-mount", func() {
				// TODO: fetch pvc size from spec.
				pvcPath := cephFSExamplePath + "pvc.yaml"
				appPath := cephFSExamplePath + "pod.yaml"
				data := "check data persists"
				label := make(map[string]string)

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
				pvc.Namespace = f.UniqueName
				pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc and application: %v", err)
				}
				var pv *v1.PersistentVolume
				_, pv, err = getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
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

				framework.Logf("Calculating checksum of %s", filePath)
				checkSum, err = calculateSHA512sum(f, app, filePath, &opt)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}
				// Create snapshot of the pvc
				snapshotPath := cephFSExamplePath + "snapshot.yaml"
				snap := getSnapshot(snapshotPath)
				snap.Name = "cephfs-pvc-snapshot"
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot %v", err)
				}
				validateCephFSSnapshotCount(f, 1, defaultSubvolumegroup, pv)

				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}
				deleteCephfsPlugin()

				// switch back to current changes.
				err = os.Chdir(cwd)
				if err != nil {
					framework.Failf("failed to d chdir: %v", err)
				}
				deployCephfsPlugin()

				err = waitForDeploymentComplete(f.ClientSet, cephFSDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for upgraded deployment %s: %v", cephFSDeploymentName, err)
				}

				err = waitForDaemonSets(cephFSDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for upgraded daemonset %s: %v", cephFSDeamonSetName, err)
				}

				app.Labels = label
				// validate if the app gets bound to a pvc created by
				// an earlier release.
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
			})

			By("Create clone from a snapshot", func() {
				pvcClonePath := cephFSExamplePath + "pvc-restore.yaml"
				appClonePath := cephFSExamplePath + "pod-restore.yaml"
				label := make(map[string]string)
				pvcClone, err = loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load pvc: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				appClone, err = loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				label[appKey] = "validate-snap-cephfs"
				appClone.Namespace = f.UniqueName
				appClone.Name = "snap-clone-cephfs"
				appClone.Labels = label
				err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc and application: %v", err)
				}
				var pv *v1.PersistentVolume
				_, pv, err = getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				mountPath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath
				testFilePath := filepath.Join(mountPath, "testClone")
				newCheckSum, err = calculateSHA512sum(f, appClone, testFilePath, &opt)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}
				if strings.Compare(newCheckSum, checkSum) != 0 {
					framework.Failf(
						"The checksum of files did not match, expected %s received %s  ",
						checkSum,
						newCheckSum)
				}
				framework.Logf("The checksum of files matched")

				// delete cloned pvc and pod
				err = deletePVCAndApp("", f, pvcClone, appClone)
				if err != nil {
					Fail(err.Error())
				}

				// Delete the snapshot of the parent pvc.
				snapshotPath := cephFSExamplePath + "snapshot.yaml"
				snap := getSnapshot(snapshotPath)
				snap.Name = "cephfs-pvc-snapshot"
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot %v", err)
				}
				validateCephFSSnapshotCount(f, 0, defaultSubvolumegroup, pv)
			})

			By("Create clone from existing PVC", func() {
				pvcSmartClonePath := cephFSExamplePath + "pvc-clone.yaml"
				appSmartClonePath := cephFSExamplePath + "pod-clone.yaml"
				label := make(map[string]string)

				pvcClone, err = loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load pvc: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				appClone, err = loadApp(appSmartClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				label[appKey] = "validate-snap-cephfs"
				appClone.Namespace = f.UniqueName
				appClone.Name = "appclone"
				appClone.Labels = label
				err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc and application: %v", err)
				}
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				mountPath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath
				testFilePath := filepath.Join(mountPath, "testClone")
				newCheckSum, err = calculateSHA512sum(f, appClone, testFilePath, &opt)
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

			By("delete pvc and app")
			err = deletePVCAndApp("", f, pvc, app)
			if err != nil {
				framework.Failf("failed to delete pvc and application: %v", err)
			}
			// delete cephFS provisioner secret
			err = deleteCephUser(f, keyringCephFSProvisionerUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringCephFSProvisionerUsername, err)
			}
			// delete cephFS plugin secret
			err = deleteCephUser(f, keyringCephFSNodePluginUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringCephFSNodePluginUsername, err)
			}
		})
	})
})
