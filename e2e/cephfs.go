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
	"strings"
	"sync"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2" //nolint:golint // e2e uses By() and other Ginkgo functions
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edebug "k8s.io/kubernetes/test/e2e/framework/debug"
	"k8s.io/pod-security-admission/api"
)

var (
	cephFSProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephFSProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephFSNodePlugin      = "csi-cephfsplugin.yaml"
	cephFSNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
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

	err := deleteResource(cephFSDirPath + cephFSProvisionerRBAC)
	if err != nil {
		framework.Failf("failed to delete provisioner rbac %s: %v", cephFSDirPath+cephFSProvisionerRBAC, err)
	}

	err = deleteResource(cephFSDirPath + cephFSNodePluginRBAC)
	if err != nil {
		framework.Failf("failed to delete nodeplugin rbac %s: %v", cephFSDirPath+cephFSNodePluginRBAC, err)
	}

	createORDeleteCephfsResources(kubectlCreate)
}

func deleteCephfsPlugin() {
	createORDeleteCephfsResources(kubectlDelete)
}

func createORDeleteCephfsResources(action kubectlAction) {
	cephConfigFile := getConfigFile(cephConfconfigMap, deployPath, examplePath)
	resources := []ResourceDeployer{
		// shared resources
		&yamlResource{
			filename:     cephFSDirPath + csiDriverObject,
			allowMissing: true,
		},
		&yamlResource{
			filename:     cephConfigFile,
			allowMissing: true,
		},
		// dependencies for provisioner
		&yamlResourceNamespaced{
			filename:  cephFSDirPath + cephFSProvisionerRBAC,
			namespace: cephCSINamespace,
		},
		// the provisioner itself
		&yamlResourceNamespaced{
			filename:   cephFSDirPath + cephFSProvisioner,
			namespace:  cephCSINamespace,
			oneReplica: true,
		},
		// dependencies for the node-plugin
		&yamlResourceNamespaced{
			filename:  cephFSDirPath + cephFSNodePluginRBAC,
			namespace: cephCSINamespace,
		},
		// the node-plugin itself
		&yamlResourceNamespaced{
			filename:  cephFSDirPath + cephFSNodePlugin,
			namespace: cephCSINamespace,
		},
	}

	for _, r := range resources {
		err := r.Do(action)
		if err != nil {
			framework.Failf("failed to %s resource: %v", action, err)
		}
	}
}

func validateSubvolumeCount(f *framework.Framework, count int, fileSystemName, subvolumegroup string) {
	subVol, err := listCephFSSubVolumes(f, fileSystemName, subvolumegroup)
	if err != nil {
		framework.Failf("failed to list CephFS subvolumes: %v", err)
	}
	if len(subVol) != count {
		framework.Failf("subvolumes [%v]. subvolume count %d not matching expected count %d", subVol, len(subVol), count)
	}
}

func validateCephFSSnapshotCount(
	f *framework.Framework,
	count int,
	subvolumegroup string,
	pv *v1.PersistentVolume,
) {
	subVolumeName := pv.Spec.CSI.VolumeAttributes["subvolumeName"]
	fsName := pv.Spec.CSI.VolumeAttributes["fsName"]
	snaps, err := listCephFSSnapshots(f, fsName, subVolumeName, subvolumegroup)
	if err != nil {
		framework.Failf("failed to list subvolume snapshots: %v", err)
	}
	if len(snaps) != count {
		framework.Failf("snapshots [%v]. snapshots count %d not matching expected count %d", snaps, len(snaps), count)
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

var _ = Describe(cephfsType, func() {
	f := framework.NewDefaultFramework(cephfsType)
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged
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
					framework.Failf("failed to create namespace %s: %v", cephCSINamespace, err)
				}
			}
			deployCephfsPlugin()
		}
		err := createConfigMap(cephFSDirPath, f.ClientSet, f)
		if err != nil {
			framework.Failf("failed to create configmap: %v", err)
		}
		// create cephFS provisioner secret
		key, err := createCephUser(f, keyringCephFSProvisionerUsername, cephFSProvisionerCaps())
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
		deployVault(f.ClientSet, deployTimeout)

		// wait for cluster name update in deployment
		containers := []string{cephFSContainerName}
		err = waitForContainersArgsUpdate(c, cephCSINamespace, cephFSDeploymentName,
			"clustername", defaultClusterName, containers, deployTimeout)
		if err != nil {
			framework.Failf("timeout waiting for deployment update %s/%s: %v", cephCSINamespace, cephFSDeploymentName, err)
		}
	})

	AfterEach(func() {
		if !testCephFS || upgradeTesting {
			Skip("Skipping CephFS E2E")
		}
		if CurrentSpecReport().Failed() {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-cephfs", c)
			// log provisioner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), c, cephCSINamespace)
		}
		err := deleteConfigMap(cephFSDirPath)
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
		deleteVault()

		if deployCephFS {
			deleteCephfsPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to delete namespace %s: %v", cephCSINamespace, err)
				}
			}
		}
	})

	Context("Test CephFS CSI", func() {
		if !testCephFS || upgradeTesting {
			return
		}

		It("Test CephFS CSI", func() {
			pvcPath := cephFSExamplePath + "pvc.yaml"
			appPath := cephFSExamplePath + "pod.yaml"
			deplPath := cephFSExamplePath + "deployment.yaml"
			appRWOPPath := cephFSExamplePath + "pod-rwop.yaml"
			pvcClonePath := cephFSExamplePath + "pvc-restore.yaml"
			pvcSmartClonePath := cephFSExamplePath + "pvc-clone.yaml"
			appClonePath := cephFSExamplePath + "pod-restore.yaml"
			appSmartClonePath := cephFSExamplePath + "pod-clone.yaml"
			snapshotPath := cephFSExamplePath + "snapshot.yaml"
			appEphemeralPath := cephFSExamplePath + "pod-ephemeral.yaml"
			pvcRWOPPath := cephFSExamplePath + "pvc-rwop.yaml"

			metadataPool, getErr := getCephFSMetadataPoolName(f, fileSystemName)
			if getErr != nil {
				framework.Failf("failed getting cephFS metadata pool name: %v", getErr)
			}

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(f.ClientSet, cephFSDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment %s: %v", cephFSDeploymentName, err)
				}
			})

			By("checking nodeplugin daemonset pods are running", func() {
				err := waitForDaemonSets(cephFSDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for daemonset %s: %v", cephFSDeamonSetName, err)
				}
			})

			// test only if ceph-csi is deployed via helm
			if helmTest {
				By("verify PVC and app binding on helm installation", func() {
					err := validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						framework.Failf("failed to validate CephFS pvc and application binding: %v", err)
					}
					//  Deleting the storageclass and secret created by helm
					err = deleteResource(cephFSExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete CephFS storageclass: %v", err)
					}
					err = deleteResource(cephFSExamplePath + "secret.yaml")
					if err != nil {
						framework.Failf("failed to delete CephFS storageclass: %v", err)
					}
				})
			}

			By("verify mountOptions support", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}

				err = verifySeLinuxMountOption(f, pvcPath, appPath,
					cephFSDeamonSetName, cephFSContainerName, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to verify mount options: %v", err)
				}

				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("verify generic ephemeral volume support", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				// create application
				app, err := loadApp(appEphemeralPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
				validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)
				validateOmapCount(f, 1, cephfsType, metadataPool, volumesType)
				// delete pod
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("verify RWOP volume support", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcRWOPPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				// create application
				app, err := loadApp(appRWOPPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				baseAppName := app.Name

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					if rwopMayFail(err) {
						framework.Logf("RWOP is not supported: %v", err)

						return
					}
					framework.Failf("failed to create PVC: %v", err)
				}
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
				validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)
				validateOmapCount(f, 1, cephfsType, metadataPool, volumesType)

				err = validateRWOPPodCreation(f, pvc, app, baseAppName)
				if err != nil {
					framework.Failf("failed to validate RWOP pod creation: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("check static PVC", func() {
				scPath := cephFSExamplePath + "secret.yaml"
				err := validateCephFsStaticPV(f, appPath, scPath)
				if err != nil {
					framework.Failf("failed to validate CephFS static pv: %v", err)
				}
			})

			By("create a storageclass with pool and a PVC then bind it to an app", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			if testCephFSFscrypt {
				kmsToTest := map[string]kmsConfig{
					"secrets-metadata-test": secretsMetadataKMS,
					"vault-test":            vaultKMS,
					"vault-tokens-test":     vaultTokensKMS,
					"vault-tenant-sa-test":  vaultTenantSAKMS,
				}

				for kmsID, kmsConf := range kmsToTest {
					kmsID := kmsID
					kmsConf := kmsConf
					By("create a storageclass with pool and an encrypted PVC then bind it to an app with "+kmsID, func() {
						scOpts := map[string]string{
							"encrypted":       "true",
							"encryptionKMSID": kmsID,
						}
						err := createCephfsStorageClass(f.ClientSet, f, true, scOpts)
						if err != nil {
							framework.Failf("failed to create CephFS storageclass: %v", err)
						}

						if kmsID == "vault-tokens-test" {
							var token v1.Secret
							tenant := f.UniqueName
							token, err = getSecret(vaultExamplePath + "tenant-token.yaml")
							if err != nil {
								framework.Failf("failed to load tenant token from secret: %v", err)
							}
							_, err = c.CoreV1().Secrets(tenant).Create(context.TODO(), &token, metav1.CreateOptions{})
							if err != nil {
								framework.Failf("failed to create Secret with tenant token: %v", err)
							}
							defer func() {
								err = c.CoreV1().Secrets(tenant).Delete(context.TODO(), token.Name, metav1.DeleteOptions{})
								if err != nil {
									framework.Failf("failed to delete Secret with tenant token: %v", err)
								}
							}()

						}
						if kmsID == "vault-tenant-sa-test" {
							err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
							if err != nil {
								framework.Failf("failed to create ServiceAccount: %v", err)
							}
							defer deleteTenantServiceAccount(f.UniqueName)
						}

						err = validateFscryptAndAppBinding(pvcPath, appPath, kmsConf, f)
						if err != nil {
							framework.Failf("failed to validate CephFS pvc and application binding: %v", err)
						}

						err = deleteResource(cephFSExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete CephFS storageclass: %v", err)
						}
					})
				}
			}

			By("create a PVC and check PVC/PV metadata on CephFS subvolume", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)
				validateOmapCount(f, 1, cephfsType, metadataPool, volumesType)

				pvcObj, err := getPersistentVolumeClaim(c, pvc.Namespace, pvc.Name)
				if err != nil {
					framework.Logf("error getting pvc %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				if pvcObj.Spec.VolumeName == "" {
					framework.Logf("pv name is empty %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				subvol, err := listCephFSSubVolumes(f, fileSystemName, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list CephFS subvolumes: %v", err)
				}
				if len(subvol) == 0 {
					framework.Failf("cephFS subvolumes list is empty %s", fileSystemName)
				}
				metadata, err := listCephFSSubvolumeMetadata(f, fileSystemName, subvol[0].Name, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list subvolume metadata: %v", err)
				}

				if metadata.PVCNameKey != pvc.Name {
					framework.Failf("expected pvcName %q got %q", pvc.Name, metadata.PVCNameKey)
				} else if metadata.PVCNamespaceKey != pvc.Namespace {
					framework.Failf("expected pvcNamespace %q got %q", pvc.Namespace, metadata.PVCNamespaceKey)
				} else if metadata.PVNameKey != pvcObj.Spec.VolumeName {
					framework.Failf("expected pvName %q got %q", pvcObj.Spec.VolumeName, metadata.PVNameKey)
				} else if metadata.ClusterNameKey != defaultClusterName {
					framework.Failf("expected clusterName %q got %q", defaultClusterName, metadata.ClusterNameKey)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
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
					framework.Failf("failed to create storageclass: %v", err)
				}
				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)
				validateOmapCount(f, 1, cephfsType, metadataPool, volumesType)
				// list subvolumes and check if one of them has the same prefix
				foundIt := false
				subvolumes, err := listCephFSSubVolumes(f, fileSystemName, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list subvolumes: %v", err)
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
					framework.Failf("failed to delete PVC: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				if !foundIt {
					framework.Failf("could not find subvolume with prefix %s", volumeNamePrefix)
				}
			})

			By("create a storageclass with ceph-fuse and a PVC then bind it to an app", func() {
				params := map[string]string{
					"mounter": "fuse",
				}
				err := createCephfsStorageClass(f.ClientSet, f, true, params)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("create a storageclass with ceph-fuse, mount-options and a PVC then bind it to an app", func() {
				params := map[string]string{
					"mounter":          "fuse",
					"fuseMountOptions": "debug",
				}
				err := createCephfsStorageClass(f.ClientSet, f, true, params)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("verifying that ceph-fuse recovery works for new pods", func() {
				err := deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
				err = createCephfsStorageClass(f.ClientSet, f, true, map[string]string{
					"mounter": "fuse",
				})
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				replicas := int32(2)
				pvc, depl, err := validatePVCAndDeploymentAppBinding(
					f, pvcPath, deplPath, f.UniqueName, &replicas, deployTimeout,
				)
				if err != nil {
					framework.Failf("failed to create PVC and Deployment: %v", err)
				}
				deplPods, err := listPods(f, depl.Namespace, &metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", depl.Labels["app"]),
				})
				if err != nil {
					framework.Failf("failed to list pods for Deployment: %v", err)
				}

				doStat := func(podName string) (string, error) {
					_, stdErr, execErr := execCommandInContainerByPodName(
						f,
						fmt.Sprintf("stat %s", depl.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath),
						depl.Namespace,
						podName,
						depl.Spec.Template.Spec.Containers[0].Name,
					)

					return stdErr, execErr
				}
				ensureStatSucceeds := func(podName string) error {
					stdErr, statErr := doStat(podName)
					if statErr != nil || stdErr != "" {
						return fmt.Errorf(
							"expected stat to succeed without error output ; got err %w, stderr %s",
							statErr, stdErr,
						)
					}

					return nil
				}

				pod1Name, pod2Name := deplPods[0].Name, deplPods[1].Name

				// stat() ceph-fuse mountpoints to make sure they are working.
				for i := range deplPods {
					err = ensureStatSucceeds(deplPods[i].Name)
					if err != nil {
						framework.Failf(err.Error())
					}
				}
				// Kill ceph-fuse in cephfs-csi node plugin Pods.
				nodePluginSelector, err := getDaemonSetLabelSelector(f, cephCSINamespace, cephFSDeamonSetName)
				if err != nil {
					framework.Failf("failed to get node plugin DaemonSet label selector: %v", err)
				}
				_, stdErr, err := execCommandInContainer(
					f, "killall -9 ceph-fuse", cephCSINamespace, "csi-cephfsplugin", &metav1.ListOptions{
						LabelSelector: nodePluginSelector,
					},
				)
				if err != nil {
					framework.Failf("killall command failed: err %v, stderr %s", err, stdErr)
				}
				// Verify Pod pod2Name that stat()-ing the mountpoint results in ENOTCONN.
				stdErr, err = doStat(pod2Name)
				if err == nil || !strings.Contains(stdErr, "not connected") {
					framework.Failf(
						"expected stat to fail with 'Transport endpoint not connected' or 'Socket not connected'; got err %v, stderr %s",
						err, stdErr,
					)
				}
				// Delete pod2Name Pod. This serves two purposes: it verifies that deleting pods with
				// corrupted ceph-fuse mountpoints works, and it lets the replicaset controller recreate
				// the pod with hopefully mounts working again.
				err = deletePod(pod2Name, depl.Namespace, c, deployTimeout)
				if err != nil {
					framework.Failf(err.Error())
				}
				// Wait for the second Pod to be recreated.
				err = waitForDeploymentComplete(c, depl.Name, depl.Namespace, deployTimeout)
				if err != nil {
					framework.Failf(err.Error())
				}
				// List Deployment's pods again to get name of the new pod.
				deplPods, err = listPods(f, depl.Namespace, &metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", depl.Labels["app"]),
				})
				if err != nil {
					framework.Failf("failed to list pods for Deployment: %v", err)
				}
				for i := range deplPods {
					if deplPods[i].Name != pod1Name {
						pod2Name = deplPods[i].Name

						break
					}
				}
				if pod2Name == "" {
					podNames := make([]string, len(deplPods))
					for i := range deplPods {
						podNames[i] = deplPods[i].Name
					}
					framework.Failf("no new replica found ; found replicas %v", podNames)
				}
				// Verify Pod pod2Name has its ceph-fuse mount working again.
				err = ensureStatSucceeds(pod2Name)
				if err != nil {
					framework.Failf(err.Error())
				}

				// Delete created resources.
				err = deletePVCAndDeploymentApp(f, pvc, depl)
				if err != nil {
					framework.Failf("failed to delete PVC and Deployment: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app", func() {
				err := createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate CephFS pvc and application  binding: %v", err)
				}
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					framework.Failf("failed to validate normal user CephFS pvc and application binding: %v", err)
				}
			})

			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC or application: %v", err)
					}
					err = validateSubvolumePath(f, pvc.Name, pvc.Namespace, fileSystemName, subvolumegroup)
					if err != nil {
						framework.Failf("failed to validate subvolumePath: %v", err)
					}
				}

				validateSubvolumeCount(f, totalCount, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, volumesType)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						framework.Failf("failed to delete PVC or application: %v", err)
					}

				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to check data persist in pvc: %v", err)
				}
			})

			By("Create PVC, bind it to an app, unmount volume and check app deletion", func() {
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC or application: %v", err)
				}

				err = unmountCephFSVolume(f, app.Name, pvc.Name)
				if err != nil {
					framework.Failf("failed to unmount volume: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}
			})

			By("create PVC, delete backing subvolume and check pv deletion", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				err = deleteBackingCephFSVolume(f, pvc)
				if err != nil {
					framework.Failf("failed to delete CephFS subvolume: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
			})

			By("validate multiple subvolumegroup creation", func() {
				err := deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
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
					framework.Failf("failed to create configmap: %v", err)
				}
				params := map[string]string{
					"clusterID": "clusterID-1",
				}
				err = createCephfsStorageClass(f.ClientSet, f, false, params)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				// verify subvolume group creation.
				err = validateSubvolumegroup(f, "subvolgrp1")
				if err != nil {
					framework.Failf("failed to validate subvolume group: %v", err)
				}

				// create resources and verify subvolume group creation
				// for the second cluster.
				params["clusterID"] = "clusterID-2"
				err = createCephfsStorageClass(f.ClientSet, f, false, params)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application: %v", err)
				}
				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = validateSubvolumegroup(f, "subvolgrp2")
				if err != nil {
					framework.Failf("failed to validate subvolume group: %v", err)
				}
				err = deleteConfigMap(cephFSDirPath)
				if err != nil {
					framework.Failf("failed to delete configmap: %v", err)
				}
				err = createConfigMap(cephFSDirPath, f.ClientSet, f)
				if err != nil {
					framework.Failf("failed to create configmap: %v", err)
				}
				err = createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Resize PVC and check application directory size", func() {
				err := resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize PVC: %v", err)
				}
			})

			By("Mount pvc as readonly in pod", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
					framework.Failf("failed to create PVC or application: %v", err)
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
					framework.Failf(stdErr)
				}

				// delete PVC and app
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}
			})

			By("Test subvolume snapshot and restored PVC metadata", func() {
				err := createCephFSSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create CephFS snapshotclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				snap.Name = f.UniqueName
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot (%s): %v", snap.Name, err)
				}
				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}
				subVolumeName := pv.Spec.CSI.VolumeAttributes["subvolumeName"]
				validateCephFSSnapshotCount(f, 1, subvolumegroup, pv)
				snaps, err := listCephFSSnapshots(f, fileSystemName, subVolumeName, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list subvolume snapshots: %v", err)
				}
				content, err := getVolumeSnapshotContent(snap.Namespace, snap.Name)
				if err != nil {
					framework.Failf("failed to get snapshotcontent for %s in namespace %s: %v",
						snap.Name, snap.Namespace, err)
				}
				metadata, err := listCephFSSnapshotMetadata(f,
					fileSystemName, subVolumeName, snaps[0].Name, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list subvolume snapshots metadata: %v", err)
				}
				if metadata.VolSnapNameKey != snap.Name {
					framework.Failf("failed, snapname expected:%s got:%s",
						snap.Name, metadata.VolSnapNameKey)
				} else if metadata.VolSnapNamespaceKey != snap.Namespace {
					framework.Failf("failed, snapnamespace expected:%s got:%s",
						snap.Namespace, metadata.VolSnapNamespaceKey)
				} else if metadata.VolSnapContentNameKey != content.Name {
					framework.Failf("failed, contentname expected:%s got:%s",
						content.Name, metadata.VolSnapContentNameKey)
				} else if metadata.ClusterNameKey != defaultClusterName {
					framework.Failf("expected clusterName %q got %q", defaultClusterName, metadata.ClusterNameKey)
				}

				// Delete the parent pvc before restoring
				// another one from snapshot.
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}

				// Test Restore snapshot
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = snap.Name
				// create PVC from the snapshot
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc clone: %v", err)
				}
				pvcCloneObj, pvCloneObj, err := getPVCAndPV(f.ClientSet, pvcClone.Name, pvcClone.Namespace)
				if err != nil {
					framework.Logf("error getting pvc %q in namespace %q: %v", pvcClone.Name, pvcClone.Namespace, err)
				}
				subVolumeCloneName := pvCloneObj.Spec.CSI.VolumeAttributes["subvolumeName"]
				cloneMetadata, err := listCephFSSubvolumeMetadata(f, fileSystemName, subVolumeCloneName, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list subvolume clone metadata: %v", err)
				}
				if cloneMetadata.PVCNameKey != pvcClone.Name {
					framework.Failf("expected pvcName %q got %q", pvcClone.Name, cloneMetadata.PVCNameKey)
				} else if cloneMetadata.PVCNamespaceKey != pvcClone.Namespace {
					framework.Failf("expected pvcNamespace %q got %q", pvcClone.Namespace, cloneMetadata.PVCNamespaceKey)
				} else if cloneMetadata.PVNameKey != pvcCloneObj.Spec.VolumeName {
					framework.Failf("expected pvName %q got %q", pvcCloneObj.Spec.VolumeName, cloneMetadata.PVNameKey)
				} else if cloneMetadata.ClusterNameKey != defaultClusterName {
					framework.Failf("expected clusterName %q got %q", defaultClusterName, cloneMetadata.ClusterNameKey)
				}

				// delete clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc clone: %v", err)
				}
				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot (%s): %v", f.UniqueName, err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)

				err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS snapshotclass: %v", err)
				}
			})

			By("Test Clone metadata", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = pvc.Name
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc clone: %v", err)
				}
				// delete parent PVC
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}

				pvcCloneObj, pvCloneObj, err := getPVCAndPV(f.ClientSet, pvcClone.Name, pvcClone.Namespace)
				if err != nil {
					framework.Logf("error getting pvc %q in namespace %q: %v", pvcClone.Name, pvcClone.Namespace, err)
				}
				subVolumeCloneName := pvCloneObj.Spec.CSI.VolumeAttributes["subvolumeName"]
				cloneMetadata, err := listCephFSSubvolumeMetadata(f, fileSystemName, subVolumeCloneName, subvolumegroup)
				if err != nil {
					framework.Failf("failed to list subvolume clone metadata: %v", err)
				}
				if cloneMetadata.PVCNameKey != pvcClone.Name {
					framework.Failf("expected pvcName %q got %q", pvc.Name, cloneMetadata.PVCNameKey)
				} else if cloneMetadata.PVCNamespaceKey != pvcClone.Namespace {
					framework.Failf("expected pvcNamespace %q got %q", pvc.Namespace, cloneMetadata.PVCNamespaceKey)
				} else if cloneMetadata.PVNameKey != pvcCloneObj.Spec.VolumeName {
					framework.Failf("expected pvName %q got %q", pvcCloneObj.Spec.VolumeName, cloneMetadata.PVNameKey)
				} else if cloneMetadata.ClusterNameKey != defaultClusterName {
					framework.Failf("expected clusterName %q got %q", defaultClusterName, cloneMetadata.ClusterNameKey)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc clone: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
			})

			By("Delete snapshot after deleting subvolume and snapshot from backend", func() {
				err := createCephFSSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create CephFS snapshotclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				snap.Name = f.UniqueName
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot (%s): %v", snap.Name, err)
				}
				validateCephFSSnapshotCount(f, 1, subvolumegroup, pv)
				err = deleteBackingCephFSSubvolumeSnapshot(f, pvc, &snap)
				if err != nil {
					framework.Failf("failed to delete backing snapshot for snapname:=%s", err)
				}

				err = deleteBackingCephFSVolume(f, pvc)
				if err != nil {
					framework.Failf("failed to delete backing subvolume error=%s", err)
				}

				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot:=%s", err)
				} else {
					framework.Logf("successfully deleted snapshot")
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}

				err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS snapshotclass: %v", err)
				}
			})

			By("Test snapshot retention feature", func() {
				// Delete the PVC after creating a snapshot,
				// this should work because of the snapshot
				// retention feature. Restore a PVC from that
				// snapshot.

				err := createCephFSSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create CephFS snapshotclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				snap.Name = f.UniqueName
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot (%s): %v", snap.Name, err)
				}

				validateCephFSSnapshotCount(f, 1, subvolumegroup, pv)
				// Delete the parent pvc before restoring
				// another one from snapshot.
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}

				pvcClone.Namespace = f.UniqueName
				appClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = snap.Name

				// create PVC from the snapshot
				name := f.UniqueName
				err = createPVCAndApp(name, f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Logf("failed to create PVC and app (%s): %v", f.UniqueName, err)
				}

				// delete clone and app
				err = deletePVCAndApp(name, f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete PVC and app (%s): %v", f.UniqueName, err)
				}

				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot (%s): %v", f.UniqueName, err)
				}

				err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS snapshotclass: %v", err)
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
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
					framework.Failf("failed to  write data : %v", wErr)
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
						framework.Logf("failed to create snapshot (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("creating snapshots failed, %d errors were logged", failed)
				}
				validateCephFSSnapshotCount(f, totalCount, subvolumegroup, pv)

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
						framework.Logf("failed to create PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("creating PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalSubvolumes, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, snapsType)

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
						framework.Logf("failed to delete PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				parentPVCCount := totalSubvolumes - totalCount
				validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
				validateOmapCount(f, parentPVCCount, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, snapsType)
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
						framework.Logf("failed to create PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("creating PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalSubvolumes, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, snapsType)

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
						framework.Logf("failed to delete snapshot (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting snapshots failed, %d errors were logged", failed)
				}

				validateCephFSSnapshotCount(f, 0, subvolumegroup, pv)

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
						framework.Logf("failed to delete PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
				validateOmapCount(f, parentPVCCount, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, 0, cephfsType, metadataPool, snapsType)
				// delete parent pvc
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, 0, cephfsType, metadataPool, snapsType)

				err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS snapshotclass: %v", err)
				}
			})

			if testCephFSFscrypt {
				for _, kmsID := range []string{"secrets-metadata-test", "vault-test"} {
					kmsID := kmsID
					By("checking encrypted snapshot-backed volume with KMS "+kmsID, func() {
						err := deleteResource(cephFSExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}

						scOpts := map[string]string{
							"encrypted":       "true",
							"encryptionKMSID": kmsID,
						}

						err = createCephfsStorageClass(f.ClientSet, f, true, scOpts)
						if err != nil {
							framework.Failf("failed to create CephFS storageclass: %v", err)
						}

						err = createCephFSSnapshotClass(f)
						if err != nil {
							framework.Failf("failed to delete CephFS storageclass: %v", err)
						}

						pvc, err := loadPVC(pvcPath)
						if err != nil {
							framework.Failf("failed to load PVC: %v", err)
						}
						pvc.Namespace = f.UniqueName
						err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
						if err != nil {
							framework.Failf("failed to create PVC: %v", err)
						}

						app, err := loadApp(appPath)
						if err != nil {
							framework.Failf("failed to load application: %v", err)
						}
						app.Namespace = f.UniqueName
						app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
						appLabels := map[string]string{
							appKey: appLabel,
						}
						app.Labels = appLabels
						optApp := metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s", appKey, appLabels[appKey]),
						}
						err = writeDataInPod(app, &optApp, f)
						if err != nil {
							framework.Failf("failed to write data: %v", err)
						}

						appTestFilePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

						snap := getSnapshot(snapshotPath)
						snap.Namespace = f.UniqueName
						snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
						err = createSnapshot(&snap, deployTimeout)
						if err != nil {
							framework.Failf("failed to create snapshot: %v", err)
						}

						err = appendToFileInContainer(f, app, appTestFilePath, "hello", &optApp)
						if err != nil {
							framework.Failf("failed to append data: %v", err)
						}

						parentFileSum, err := calculateSHA512sum(f, app, appTestFilePath, &optApp)
						if err != nil {
							framework.Failf("failed to get SHA512 sum for file: %v", err)
						}

						err = deleteResource(cephFSExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete CephFS storageclass: %v", err)
						}

						err = createCephfsStorageClass(f.ClientSet, f, false, map[string]string{
							"encrypted":       "true",
							"encryptionKMSID": kmsID,
						})
						if err != nil {
							framework.Failf("failed to create CephFS storageclass: %v", err)
						}

						pvcClone, err := loadPVC(pvcClonePath)
						if err != nil {
							framework.Failf("failed to load PVC: %v", err)
						}
						// Snapshot-backed volumes support read-only access modes only.
						pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
						appClone, err := loadApp(appClonePath)
						if err != nil {
							framework.Failf("failed to load application: %v", err)
						}
						appCloneLabels := map[string]string{
							appKey: appCloneLabel,
						}
						appClone.Labels = appCloneLabels
						optAppClone := metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s", appKey, appCloneLabels[appKey]),
						}
						pvcClone.Namespace = f.UniqueName
						appClone.Namespace = f.UniqueName
						err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
						if err != nil {
							framework.Failf("failed to create PVC and app: %v", err)
						}

						// Snapshot-backed volume shouldn't contribute to total subvolume count.
						validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)

						// Deleting snapshot before deleting pvcClone should succeed. It will be
						// deleted once all volumes that are backed by this snapshot are gone.
						err = deleteSnapshot(&snap, deployTimeout)
						if err != nil {
							framework.Failf("failed to delete snapshot: %v", err)
						}

						appCloneTestFilePath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

						snapFileSum, err := calculateSHA512sum(f, appClone, appCloneTestFilePath, &optAppClone)
						if err != nil {
							framework.Failf("failed to get SHA512 sum for file: %v", err)
						}

						if parentFileSum == snapFileSum {
							framework.Failf("SHA512 sums of files in parent subvol and snapshot should differ")
						}

						err = deletePVCAndApp("", f, pvcClone, appClone)
						if err != nil {
							framework.Failf("failed to delete PVC or application: %v", err)
						}

						err = deletePVCAndApp("", f, pvc, app)
						if err != nil {
							framework.Failf("failed to delete PVC or application: %v", err)
						}

						err = deleteResource(cephFSExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete CephFS storageclass: %v", err)
						}

						err = deleteResource(cephFSExamplePath + "snapshotclass.yaml")
						if err != nil {
							framework.Failf("failed to delete CephFS snapshotclass: %v", err)
						}

						err = createCephfsStorageClass(f.ClientSet, f, false, nil)
						if err != nil {
							framework.Failf("failed to create CephFS storageclass: %v", err)
						}
					})
				}
			}

			By("checking snapshot-backed volume", func() {
				err := createCephFSSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create CephFS snapshotclass: %v", err)
				}

				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				appLabels := map[string]string{
					appKey: appLabel,
				}
				app.Labels = appLabels
				optApp := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, appLabels[appKey]),
				}
				err = writeDataInPod(app, &optApp, f)
				if err != nil {
					framework.Failf("failed to write data: %v", err)
				}

				appTestFilePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				validateCephFSSnapshotCount(f, 1, subvolumegroup, pv)

				err = appendToFileInContainer(f, app, appTestFilePath, "hello", &optApp)
				if err != nil {
					framework.Failf("failed to append data: %v", err)
				}

				parentFileSum, err := calculateSHA512sum(f, app, appTestFilePath, &optApp)
				if err != nil {
					framework.Failf("failed to get SHA512 sum for file: %v", err)
				}

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				// Snapshot-backed volumes support read-only access modes only.
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				appCloneLabels := map[string]string{
					appKey: appCloneLabel,
				}
				appClone.Labels = appCloneLabels
				optAppClone := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, appCloneLabels[appKey]),
				}
				pvcClone.Namespace = f.UniqueName
				appClone.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and app: %v", err)
				}

				// Snapshot-backed volume shouldn't contribute to total subvolume count.
				validateSubvolumeCount(f, 1, fileSystemName, subvolumegroup)

				// Deleting snapshot before deleting pvcClone should succeed. It will be
				// deleted once all volumes that are backed by this snapshot are gone.
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}

				appCloneTestFilePath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

				snapFileSum, err := calculateSHA512sum(f, appClone, appCloneTestFilePath, &optAppClone)
				if err != nil {
					framework.Failf("failed to get SHA512 sum for file: %v", err)
				}

				if parentFileSum == snapFileSum {
					framework.Failf("SHA512 sums of files in parent subvol and snapshot should differ")
				}

				err = deletePVCAndApp("", f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				validateCephFSSnapshotCount(f, 0, subvolumegroup, pv)

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}

				err = createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
			})

			By("checking snapshot-backed volume by backing snapshot as false", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				appLabels := map[string]string{
					appKey: appLabel,
				}
				app.Labels = appLabels
				optApp := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, appLabels[appKey]),
				}
				err = writeDataInPod(app, &optApp, f)
				if err != nil {
					framework.Failf("failed to write data: %v", err)
				}

				appTestFilePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				validateCephFSSnapshotCount(f, 1, subvolumegroup, pv)

				err = appendToFileInContainer(f, app, appTestFilePath, "hello", &optApp)
				if err != nil {
					framework.Failf("failed to append data: %v", err)
				}

				parentFileSum, err := calculateSHA512sum(f, app, appTestFilePath, &optApp)
				if err != nil {
					framework.Failf("failed to get SHA512 sum for file: %v", err)
				}

				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}

				err = createCephfsStorageClass(f.ClientSet, f, false, map[string]string{
					"backingSnapshot": "false",
				})
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				// Snapshot-backed volumes support read-only access modes only.
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				appCloneLabels := map[string]string{
					appKey: appCloneLabel,
				}
				appClone.Labels = appCloneLabels
				optAppClone := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, appCloneLabels[appKey]),
				}
				pvcClone.Namespace = f.UniqueName
				appClone.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and app: %v", err)
				}

				validateSubvolumeCount(f, 2, fileSystemName, subvolumegroup)

				// Deleting snapshot before deleting pvcClone should succeed. It will be
				// deleted once all volumes that are backed by this snapshot are gone.
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}
				validateCephFSSnapshotCount(f, 0, subvolumegroup, pv)

				appCloneTestFilePath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

				snapFileSum, err := calculateSHA512sum(f, appClone, appCloneTestFilePath, &optAppClone)
				if err != nil {
					framework.Failf("failed to get SHA512 sum for file: %v", err)
				}

				if parentFileSum == snapFileSum {
					framework.Failf("SHA512 sums of files in parent subvol and snapshot should differ")
				}

				err = deletePVCAndApp("", f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				err = deleteResource(cephFSExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete CephFS storageclass: %v", err)
				}

				err = createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create CephFS storageclass: %v", err)
				}
			})

			if testCephFSFscrypt {
				kmsToTest := map[string]kmsConfig{
					"secrets-metadata-test": secretsMetadataKMS,
					"vault-test":            vaultKMS,
				}
				for kmsID, kmsConf := range kmsToTest {
					kmsID := kmsID
					kmsConf := kmsConf
					By("create an encrypted PVC-PVC clone and bind it to an app with "+kmsID, func() {
						err := deleteResource(cephFSExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}

						scOpts := map[string]string{
							"encrypted":       "true",
							"encryptionKMSID": kmsID,
						}

						err = createCephfsStorageClass(f.ClientSet, f, true, scOpts)
						if err != nil {
							framework.Failf("failed to create CephFS storageclass: %v", err)
						}

						validateFscryptClone(pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, kmsConf, f)

						err = deleteResource(cephFSExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
						err = createCephfsStorageClass(f.ClientSet, f, false, nil)
						if err != nil {
							framework.Failf("failed to create CephFS storageclass: %v", err)
						}
					})
				}
			}

			By("create a PVC-PVC clone and bind it to an app", func() {
				var wg sync.WaitGroup
				totalCount := 3
				wgErrs := make([]error, totalCount)
				// totalSubvolumes represents the subvolumes in backend
				// always totalCount+parentPVC
				totalSubvolumes := totalCount + 1
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
					framework.Failf("failed to write data from application %v", wErr)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				appClone, err := loadApp(appSmartClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
						framework.Logf("failed to create PVC or application (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalSubvolumes, cephfsType, metadataPool, volumesType)

				// delete parent pvc
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
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
						framework.Logf("failed to delete PVC or application (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
			})

			By("Create ROX PVC and bind it to an app", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}

				app.Namespace = f.UniqueName
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				err = createPVCAndApp("", f, pvcClone, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC or application: %v", err)
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
					framework.Failf(stdErr)
				}

				// delete cloned ROX pvc and app
				err = deletePVCAndApp("", f, pvcClone, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
			})

			By("restore snapshot to a bigger size PVC", func() {
				err := validateBiggerPVCFromSnapshot(f,
					pvcPath,
					appPath,
					snapshotPath,
					pvcClonePath,
					appClonePath)
				if err != nil {
					framework.Failf("failed to validate restore bigger size clone: %v", err)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
			})

			By("clone PVC to a bigger size PVC", func() {
				err := validateBiggerCloneFromPVC(f,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath)
				if err != nil {
					framework.Failf("failed to validate bigger size clone: %v", err)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
			})

			// FIXME: in case NFS testing is done, prevent deletion
			// of the CephFS filesystem and related pool. This can
			// probably be addressed in a nicer way, making sure
			// everything is tested, always.
			if testNFS {
				framework.Logf("skipping CephFS destructive tests, allow NFS to run")

				return
			}

			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, true, f)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
			})
			// delete cephFS provisioner secret
			err := deleteCephUser(f, keyringCephFSProvisionerUsername)
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
