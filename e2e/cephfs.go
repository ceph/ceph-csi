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
	"io/ioutil"
	"os"
	"strings"
	"sync"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	cephFSProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephFSProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephFSProvisionerPSP  = "csi-provisioner-psp.yaml"
	cephFSNodePlugin      = "csi-cephfsplugin.yaml"
	cephFSNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	cephFSNodePluginPSP   = "csi-nodeplugin-psp.yaml"
	cephFSDeploymentName  = "csi-cephfsplugin-provisioner"
	cephFSDeamonSetName   = "csi-cephfsplugin"
	cephFSContainerName   = "csi-cephfsplugin"
	cephFSDirPath         = "../deploy/cephfs/kubernetes/"
	cephFSExamplePath     = examplePath + "cephfs/"
	subvolumegroup        = "e2e"
	fileSystemName        = "myfs"
)

func deployCephfsPlugin() {
	// delete objects deployed by rook

	data, err := replaceNamespaceInTemplate(cephFSDirPath + cephFSProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "--ignore-not-found=true", ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Failf("failed to delete provisioner rbac %s: %v", cephFSDirPath+cephFSProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(cephFSDirPath + cephFSNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "delete", "--ignore-not-found=true", ns, "-f", "-")

	if err != nil {
		e2elog.Failf("failed to delete nodeplugin rbac %s: %v", cephFSDirPath+cephFSNodePluginRBAC, err)
	}

	createORDeleteCephfsResources(kubectlCreate)
}

func deleteCephfsPlugin() {
	createORDeleteCephfsResources(kubectlDelete)
}

func createORDeleteCephfsResources(action kubectlAction) {
	csiDriver, err := ioutil.ReadFile(cephFSDirPath + csiDriverObject)
	if err != nil {
		// createORDeleteRbdResources is used for upgrade testing as csidriverObject is
		// newly added, discarding file not found error.
		if !os.IsNotExist(err) {
			e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+csiDriverObject, err)
		}
	} else {
		err = retryKubectlInput(cephCSINamespace, action, string(csiDriver), deployTimeout)
		if err != nil {
			e2elog.Failf("failed to %s CSIDriver object: %v", action, err)
		}
	}
	cephConf, err := ioutil.ReadFile(examplePath + cephConfconfigMap)
	if err != nil {
		// createORDeleteCephfsResources is used for upgrade testing as cephConfConfigmap is
		// newly added, discarding file not found error.
		if !os.IsNotExist(err) {
			e2elog.Failf("failed to read content from %s: %v", examplePath+cephConfconfigMap, err)
		}
	} else {
		err = retryKubectlInput(cephCSINamespace, action, string(cephConf), deployTimeout)
		if err != nil {
			e2elog.Failf("failed to %s ceph-conf configmap object: %v", action, err)
		}
	}
	data, err := replaceNamespaceInTemplate(cephFSDirPath + cephFSProvisioner)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSProvisioner, err)
	}
	data = oneReplicaDeployYaml(data)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s CephFS provisioner: %v", action, err)
	}
	data, err = replaceNamespaceInTemplate(cephFSDirPath + cephFSProvisionerRBAC)

	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSProvisionerRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s CephFS provisioner rbac: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephFSDirPath + cephFSProvisionerPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSProvisionerPSP, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s CephFS provisioner psp: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephFSDirPath + cephFSNodePlugin)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSNodePlugin, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s CephFS nodeplugin: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephFSDirPath + cephFSNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSNodePluginRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s CephFS nodeplugin rbac: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephFSDirPath + cephFSNodePluginPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", cephFSDirPath+cephFSNodePluginPSP, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s CephFS nodeplugin psp: %v", action, err)
	}
}

func validateSubvolumeCount(f *framework.Framework, count int, fileSystemName, subvolumegroup string) {
	subVol, err := listCephFSSubVolumes(f, fileSystemName, subvolumegroup)
	if err != nil {
		e2elog.Failf("failed to list CephFS subvolumes: %v", err)
	}
	if len(subVol) != count {
		e2elog.Failf("subvolumes [%v]. subvolume count %d not matching expected count %v", subVol, len(subVol), count)
	}
}

func validateSubvolumePath(f *framework.Framework, pvcName, pvcNamespace, fileSystemName, subvolumegroup string) error {
	_, pv, err := getPVCAndPV(f.ClientSet, pvcName, pvcNamespace)
	if err != nil {
		return fmt.Errorf("failed to get PVC %s in namespace %s: %w", pvcName, pvcNamespace, err)
	}
	subVolumePathInPV := pv.Spec.CSI.VolumeAttributes["subvolumePath"]
	subVolume := pv.Spec.CSI.VolumeAttributes["subvolumeName"]
	if subVolumePathInPV == "" {
		return fmt.Errorf("subvolumePath is not set in %s PVC", pvcName)
	}
	if subVolume == "" {
		return fmt.Errorf("subvolumeName is not set in %s PVC", pvcName)
	}
	subVolumePath, err := getSubvolumePath(f, fileSystemName, subvolumegroup, subVolume)
	if err != nil {
		return err
	}
	if subVolumePath != subVolumePathInPV {
		return fmt.Errorf(
			"subvolumePath %s is not matching the subvolumePath %s in PV",
			subVolumePath,
			subVolumePathInPV)
	}

	return nil
}

var _ = Describe("cephfs", func() {
	f := framework.NewDefaultFramework("cephfs")
	var c clientset.Interface
	// deploy CephFS CSI
	BeforeEach(func() {
		if !testCephFS || upgradeTesting {
			Skip("Skipping CephFS E2E")
		}
		c = f.ClientSet
		if deployCephFS {
			if cephCSINamespace != defaultNs {
				err := createNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to create namespace %s: %v", cephCSINamespace, err)
				}
			}
			deployCephfsPlugin()
		}
		err := createConfigMap(cephFSDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap: %v", err)
		}
		// create cephFS provisioner secret
		key, err := createCephUser(f, keyringCephFSProvisionerUsername, cephFSProvisionerCaps())
		if err != nil {
			e2elog.Failf("failed to create user %s: %v", keyringCephFSProvisionerUsername, err)
		}
		err = createCephfsSecret(f, cephFSProvisionerSecretName, keyringCephFSProvisionerUsername, key)
		if err != nil {
			e2elog.Failf("failed to create provisioner secret: %v", err)
		}
		// create cephFS plugin secret
		key, err = createCephUser(f, keyringCephFSNodePluginUsername, cephFSNodePluginCaps())
		if err != nil {
			e2elog.Failf("failed to create user %s: %v", keyringCephFSNodePluginUsername, err)
		}
		err = createCephfsSecret(f, cephFSNodePluginSecretName, keyringCephFSNodePluginUsername, key)
		if err != nil {
			e2elog.Failf("failed to create node secret: %v", err)
		}
	})

	AfterEach(func() {
		if !testCephFS || upgradeTesting {
			Skip("Skipping CephFS E2E")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-cephfs", c)
			// log provisioner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			framework.DumpAllNamespaceInfo(c, cephCSINamespace)
		}
		err := deleteConfigMap(cephFSDirPath)
		if err != nil {
			e2elog.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(cephFSExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass: %v", err)
		}
		if deployCephFS {
			deleteCephfsPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to delete namespace %s: %v", cephCSINamespace, err)
				}
			}
		}
	})

	Context("Test CephFS CSI", func() {
		It("Test CephFS CSI", func() {
			pvcPath := cephFSExamplePath + "pvc.yaml"
			appPath := cephFSExamplePath + "pod.yaml"
			pvcClonePath := cephFSExamplePath + "pvc-restore.yaml"
			pvcSmartClonePath := cephFSExamplePath + "pvc-clone.yaml"
			appClonePath := cephFSExamplePath + "pod-restore.yaml"
			appSmartClonePath := cephFSExamplePath + "pod-clone.yaml"
			snapshotPath := cephFSExamplePath + "snapshot.yaml"
			appEphemeralPath := cephFSExamplePath + "pod-ephemeral.yaml"

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(f.ClientSet, cephFSDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment %s: %v", cephFSDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(cephFSDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset %s: %v", cephFSDeamonSetName, err)
				}
			})

			// test only if ceph-csi is deployed via helm
			if helmTest {
				By("verify PVC and app binding on helm installation", func() {
					err := validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
					}
					//  Deleting the storageclass and secret created by helm
					err = deleteResource(cephFSExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete CephFS storageclass: %v", err)
					}
					err = deleteResource(cephFSExamplePath + "secret.yaml")
					if err != nil {
						e2elog.Failf("failed to delete CephFS storageclass: %v", err)
					}
				})
			}
			By("verify generic ephemeral volume support", func() {
				// generic ephemeral volume support is beta since v1.21.
				if k8sVersionGreaterEquals(f.ClientSet, 1, 21) {
					err := createCephfsStorageClass(f.ClientSet, f, true, nil)
					if err != nil {
						e2elog.Failf("failed to create CephFS storageclass: %v", err)
					}
					// create application
					app, err := loadApp(appEphemeralPath)
					if err != nil {
						e2elog.Failf("failed to load application: %v", err)
					}
					app.Namespace = f.UniqueName
					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application: %v", err)
					}
					validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)
					// delete pod
					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application: %v", err)
					}
					validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
					err = deleteResource(cephFSExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete CephFS storageclass: %v", err)
					}
				}
			})

			By("check static PVC", func() {
				scPath := cephFSExamplePath + "secret.yaml"
				err := validateCephFsStaticPV(f, appPath, scPath)
				if err != nil {
					e2elog.Failf("failed to validate CephFS static pv: %v", err)
				}
			})

			By("create a storageclass with pool and a PVC then bind it to an app", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("create PVC in storageClass with volumeNamePrefix", func() {
				volumeNamePrefix := "foo-bar-"
				err := createCephfsStorageClass(
					f.ClientSet,
					f,
					false,
					map[string]string{"volumeNamePrefix": volumeNamePrefix})
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}

				validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)
				// list subvolumes and check if one of them has the same prefix
				foundIt := false
				subvolumes, err := listCephFSSubVolumes(f, fileSystemName, subvolumegroup)
				if err != nil {
					e2elog.Failf("failed to list subvolumes: %v", err)
				}
				for _, subVol := range subvolumes {
					fmt.Printf("Checking prefix on %s\n", subVol)
					if strings.HasPrefix(subVol.Name, volumeNamePrefix) {
						foundIt = true

						break
					}
				}
				// clean up after ourselves
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to  delete PVC: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				if !foundIt {
					e2elog.Failf("could not find subvolume with prefix %s", volumeNamePrefix)
				}
			})

			By("create a storageclass with ceph-fuse and a PVC then bind it to an app", func() {
				params := map[string]string{
					"mounter": "fuse",
				}
				err := createCephfsStorageClass(f.ClientSet, f, true, params)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("create a storageclass with ceph-fuse, mount-options and a PVC then bind it to an app", func() {
				params := map[string]string{
					"mounter":          "fuse",
					"fuseMountOptions": "debug",
				}
				err := createCephfsStorageClass(f.ClientSet, f, true, params)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app", func() {
				err := createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application  binding: %v", err)
				}
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate normal user CephFS pvc and application binding: %v", err)
				}
			})

			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC or application: %v", err)
					}
					err = validateSubvolumePath(f, pvc.Name, pvc.Namespace, fileSystemName, subvolumegroup)
					if err != nil {
						e2elog.Failf("failed to validate subvolumePath: %v", err)
					}
				}

				validateSubvolumeCount(f, totalCount, fileSystemName, subvolumegroup)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC or application: %v", err)
					}

				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to check data persist in pvc: %v", err)
				}
			})

			By("Create PVC, bind it to an app, unmount volume and check app deletion", func() {
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC or application: %v", err)
				}

				err = unmountCephFSVolume(f, app.Name, pvc.Name)
				if err != nil {
					e2elog.Failf("failed to unmount volume: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}
			})

			By("create PVC, delete backing subvolume and check pv deletion", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}

				err = deleteBackingCephFSVolume(f, pvc)
				if err != nil {
					e2elog.Failf("failed to delete CephFS subvolume: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
			})

			By("validate multiple subvolumegroup creation", func() {
				err := deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}

				// re-define configmap with information of multiple clusters.
				clusterInfo := map[string]map[string]string{}
				clusterID1 := "clusterID-1"
				clusterID2 := "clusterID-2"
				clusterInfo[clusterID1] = map[string]string{}
				clusterInfo[clusterID1]["subvolumeGroup"] = "subvolgrp1"
				clusterInfo[clusterID2] = map[string]string{}
				clusterInfo[clusterID2]["subvolumeGroup"] = "subvolgrp2"

				err = createCustomConfigMap(f.ClientSet, cephFSDirPath, clusterInfo)
				if err != nil {
					e2elog.Failf("failed to create configmap: %v", err)
				}
				params := map[string]string{
					"clusterID": "clusterID-1",
				}
				err = createCephfsStorageClass(f.ClientSet, f, false, params)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				// verify subvolume group creation.
				err = validateSubvolumegroup(f, "subvolgrp1")
				if err != nil {
					e2elog.Failf("failed to validate subvolume group: %v", err)
				}

				// create resources and verify subvolume group creation
				// for the second cluster.
				params["clusterID"] = "clusterID-2"
				err = createCephfsStorageClass(f.ClientSet, f, false, params)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = validateSubvolumegroup(f, "subvolgrp2")
				if err != nil {
					e2elog.Failf("failed to validate subvolume group: %v", err)
				}
				err = deleteConfigMap(cephFSDirPath)
				if err != nil {
					e2elog.Failf("failed to delete configmap: %v", err)
				}
				err = createConfigMap(cephFSDirPath, f.ClientSet, f)
				if err != nil {
					e2elog.Failf("failed to create configmap: %v", err)
				}
				err = createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Resize PVC and check application directory size", func() {
				err := resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize PVC: %v", err)
				}
			})

			By("Mount pvc as readonly in pod", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}

				app.Namespace = f.UniqueName
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				app.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC or application: %v", err)
				}

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr := execCommandInPodAndAllowFail(
					f,
					fmt.Sprintf("echo 'Hello World' > %s", filePath),
					app.Namespace,
					&opt)
				readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
				if !strings.Contains(stdErr, readOnlyErr) {
					e2elog.Failf(stdErr)
				}

				// delete PVC and app
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}
			})

			By("Delete snapshot after deleting subvolume and snapshot from backend", func() {
				err := createCephFSSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create CephFS snapshotclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				snap.Name = f.UniqueName
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot (%s): %v", snap.Name, err)
				}

				err = deleteBackingCephFSSubvolumeSnapshot(f, pvc, &snap)
				if err != nil {
					e2elog.Failf("failed to delete backing snapshot for snapname:=%s", err)
				}

				err = deleteBackingCephFSVolume(f, pvc)
				if err != nil {
					e2elog.Failf("failed to delete backing subvolume error=%s", err)
				}

				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot:=%s", err)
				} else {
					e2elog.Logf("successfully deleted snapshot")
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}

				err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS snapshotclass: %v", err)
				}
			})

			By("Test snapshot retention feature", func() {
				// Delete the PVC after creating a snapshot,
				// this should work because of the snapshot
				// retention feature. Restore a PVC from that
				// snapshot.

				err := createCephFSSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create CephFS snapshotclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				snap.Name = f.UniqueName
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot (%s): %v", snap.Name, err)
				}

				// Delete the parent pvc before restoring
				// another one from snapshot.
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				appClone, err := loadApp(appClonePath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}

				pvcClone.Namespace = f.UniqueName
				appClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = snap.Name

				// create PVC from the snapshot
				name := f.UniqueName
				err = createPVCAndApp(name, f, pvcClone, appClone, deployTimeout)
				if err != nil {
					e2elog.Logf("failed to create PVC and app (%s): %v", f.UniqueName, err)
				}

				// delete clone and app
				err = deletePVCAndApp(name, f, pvcClone, appClone)
				if err != nil {
					e2elog.Failf("failed to delete PVC and app (%s): %v", f.UniqueName, err)
				}

				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot (%s): %v", f.UniqueName, err)
				}

				err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS snapshotclass: %v", err)
				}
			})

			By("create a PVC clone and bind it to an app", func() {
				var wg sync.WaitGroup
				totalCount := 3
				wgErrs := make([]error, totalCount)
				// totalSubvolumes represents the subvolumes in backend
				// always totalCount+parentPVC
				totalSubvolumes := totalCount + 1
				wg.Add(totalCount)
				err := createCephFSSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}

				app.Namespace = f.UniqueName
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				label := make(map[string]string)
				label[appKey] = appLabel
				app.Labels = label
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				wErr := writeDataInPod(app, &opt, f)
				if wErr != nil {
					e2elog.Failf("failed to  write data : %v", wErr)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				for i := 0; i < totalCount; i++ {
					go func(n int, s snapapi.VolumeSnapshot) {
						s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = createSnapshot(&s, deployTimeout)
						wg.Done()
					}(i, snap)
				}
				wg.Wait()

				failed := 0
				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to create snapshot (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("creating snapshots failed, %d errors were logged", failed)
				}

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				appClone, err := loadApp(appClonePath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				appClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)

				// create multiple PVC from same snapshot
				wg.Add(totalCount)
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
						if wgErrs[n] == nil {
							err = validateSubvolumePath(f, p.Name, p.Namespace, fileSystemName, subvolumegroup)
							if err != nil {
								wgErrs[n] = err
							}
						}
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to create PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("creating PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)

				wg.Add(totalCount)
				// delete clone and app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to delete PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				parentPVCCount := totalSubvolumes - totalCount
				validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
				// create clones from different snapshots and bind it to an
				// app
				wg.Add(totalCount)
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
						if wgErrs[n] == nil {
							err = validateSubvolumePath(f, p.Name, p.Namespace, fileSystemName, subvolumegroup)
							if err != nil {
								wgErrs[n] = err
							}
						}
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to create PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("creating PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)

				wg.Add(totalCount)
				// delete snapshot
				for i := 0; i < totalCount; i++ {
					go func(n int, s snapapi.VolumeSnapshot) {
						s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = deleteSnapshot(&s, deployTimeout)
						wg.Done()
					}(i, snap)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to delete snapshot (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("deleting snapshots failed, %d errors were logged", failed)
				}

				wg.Add(totalCount)
				// delete clone and app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to delete PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
				// delete parent pvc
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				var wg sync.WaitGroup
				totalCount := 3
				wgErrs := make([]error, totalCount)
				// totalSubvolumes represents the subvolumes in backend
				// always totalCount+parentPVC
				totalSubvolumes := totalCount + 1
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}
				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				label := make(map[string]string)
				label[appKey] = appLabel
				app.Labels = label
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				wErr := writeDataInPod(app, &opt, f)
				if wErr != nil {
					e2elog.Failf("failed to write data from application %v", wErr)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				appClone, err := loadApp(appSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				appClone.Namespace = f.UniqueName
				wg.Add(totalCount)
				// create clone and bind it to an app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				failed := 0
				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to create PVC or application (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)

				// delete parent pvc
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}

				wg.Add(totalCount)
				// delete clone and app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						e2elog.Logf("failed to delete PVC or application (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					e2elog.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
			})

			By("Create ROX PVC and bind it to an app", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}

				app.Namespace = f.UniqueName
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				err = createPVCAndApp("", f, pvcClone, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC or application: %v", err)
				}

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr := execCommandInPodAndAllowFail(
					f,
					fmt.Sprintf("echo 'Hello World' > %s", filePath),
					app.Namespace,
					&opt)
				readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
				if !strings.Contains(stdErr, readOnlyErr) {
					e2elog.Failf(stdErr)
				}

				// delete cloned ROX pvc and app
				err = deletePVCAndApp("", f, pvcClone, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}

				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
			})
			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, true, f)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
			})
			// delete cephFS provisioner secret
			err := deleteCephUser(f, keyringCephFSProvisionerUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s: %v", keyringCephFSProvisionerUsername, err)
			}
			// delete cephFS plugin secret
			err = deleteCephUser(f, keyringCephFSNodePluginUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s: %v", keyringCephFSNodePluginUsername, err)
			}
		})
	})
})
