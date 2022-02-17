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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	rbdProvisioner     = "csi-rbdplugin-provisioner.yaml"
	rbdProvisionerRBAC = "csi-provisioner-rbac.yaml"
	rbdProvisionerPSP  = "csi-provisioner-psp.yaml"
	rbdNodePlugin      = "csi-rbdplugin.yaml"
	rbdNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	rbdNodePluginPSP   = "csi-nodeplugin-psp.yaml"
	configMap          = "csi-config-map.yaml"
	cephConfconfigMap  = "ceph-conf.yaml"
	csiDriverObject    = "csidriver.yaml"
	rbdDirPath         = "../deploy/rbd/kubernetes/"
	examplePath        = "../examples/"
	rbdExamplePath     = examplePath + "/rbd/"
	e2eTemplatesPath   = "../e2e/templates/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	defaultRBDPool     = "replicapool"
	erasureCodedPool   = "ec-pool"
	noDataPool         = ""
	// Topology related variables.
	nodeRegionLabel     = "test.failure-domain/region"
	regionValue         = "testregion"
	nodeZoneLabel       = "test.failure-domain/zone"
	zoneValue           = "testzone"
	nodeCSIRegionLabel  = "topology.rbd.csi.ceph.com/region"
	nodeCSIZoneLabel    = "topology.rbd.csi.ceph.com/zone"
	rbdTopologyPool     = "newrbdpool"
	rbdTopologyDataPool = "replicapool" // NOTE: should be different than rbdTopologyPool for test to be effective

	// yaml files required for deployment.
	pvcPath                = rbdExamplePath + "pvc.yaml"
	appPath                = rbdExamplePath + "pod.yaml"
	rawPvcPath             = rbdExamplePath + "raw-block-pvc.yaml"
	rawAppPath             = rbdExamplePath + "raw-block-pod.yaml"
	rawAppRWOPPath         = rbdExamplePath + "raw-block-pod-rwop.yaml"
	rawPVCRWOPPath         = rbdExamplePath + "raw-block-pvc-rwop.yaml"
	pvcClonePath           = rbdExamplePath + "pvc-restore.yaml"
	pvcSmartClonePath      = rbdExamplePath + "pvc-clone.yaml"
	pvcBlockSmartClonePath = rbdExamplePath + "pvc-block-clone.yaml"
	pvcRWOPPath            = rbdExamplePath + "pvc-rwop.yaml"
	appRWOPPath            = rbdExamplePath + "pod-rwop.yaml"
	appClonePath           = rbdExamplePath + "pod-restore.yaml"
	appSmartClonePath      = rbdExamplePath + "pod-clone.yaml"
	appBlockSmartClonePath = rbdExamplePath + "block-pod-clone.yaml"
	pvcBlockRestorePath    = rbdExamplePath + "pvc-block-restore.yaml"
	appBlockRestorePath    = rbdExamplePath + "pod-block-restore.yaml"
	appEphemeralPath       = rbdExamplePath + "pod-ephemeral.yaml"
	snapshotPath           = rbdExamplePath + "snapshot.yaml"
	deployFSAppPath        = e2eTemplatesPath + "rbd-fs-deployment.yaml"
	deployBlockAppPath     = e2eTemplatesPath + "rbd-block-deployment.yaml"
	defaultCloneCount      = 10

	nbdMapOptions             = "nbd:debug-rbd=20"
	e2eDefaultCephLogStrategy = "preserve"
)

func deployRBDPlugin() {
	// delete objects deployed by rook
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		e2elog.Failf("failed to delete provisioner rbac %s: %v", rbdDirPath+rbdProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		e2elog.Failf("failed to delete nodeplugin rbac %s: %v", rbdDirPath+rbdNodePluginRBAC, err)
	}

	createORDeleteRbdResources(kubectlCreate)
}

func deleteRBDPlugin() {
	createORDeleteRbdResources(kubectlDelete)
}

func createORDeleteRbdResources(action kubectlAction) {
	csiDriver, err := os.ReadFile(rbdDirPath + csiDriverObject)
	if err != nil {
		// createORDeleteRbdResources is used for upgrade testing as csidriverObject is
		// newly added, discarding file not found error.
		if !os.IsNotExist(err) {
			e2elog.Failf("failed to read content from %s: %v", rbdDirPath+csiDriverObject, err)
		}
	} else {
		err = retryKubectlInput(cephCSINamespace, action, string(csiDriver), deployTimeout)
		if err != nil {
			e2elog.Failf("failed to %s CSIDriver object: %v", action, err)
		}
	}
	cephConf, err := os.ReadFile(examplePath + cephConfconfigMap)
	if err != nil {
		// createORDeleteRbdResources is used for upgrade testing as cephConf Configmap is
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
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisioner)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdProvisioner, err)
	}
	data = oneReplicaDeployYaml(data)
	data = enableTopologyInTemplate(data)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s rbd provisioner: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s provisioner rbac: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdProvisionerPSP, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s provisioner psp: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePlugin)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdNodePlugin, err)
	}

	domainLabel := nodeRegionLabel + "," + nodeZoneLabel
	data = addTopologyDomainsToDSYaml(data, domainLabel)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin rbac: %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s: %v", rbdDirPath+rbdNodePluginPSP, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin psp: %v", action, err)
	}
}

func validateRBDImageCount(f *framework.Framework, count int, pool string) {
	imageList, err := listRBDImages(f, pool)
	if err != nil {
		e2elog.Failf("failed to list rbd images: %v", err)
	}
	if len(imageList) != count {
		e2elog.Failf(
			"backend images not matching kubernetes resource count,image count %d kubernetes resource count %d"+
				"\nbackend image Info:\n %v",
			len(imageList),
			count,
			imageList)
	}
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("rbd")
	var c clientset.Interface
	var kernelRelease string
	// deploy RBD CSI
	BeforeEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		c = f.ClientSet
		if deployRBD {
			err := createNodeLabel(f, nodeRegionLabel, regionValue)
			if err != nil {
				e2elog.Failf("failed to create node label: %v", err)
			}
			err = createNodeLabel(f, nodeZoneLabel, zoneValue)
			if err != nil {
				e2elog.Failf("failed to create node label: %v", err)
			}
			if cephCSINamespace != defaultNs {
				err = createNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to create namespace: %v", err)
				}
			}
			deployRBDPlugin()
		}
		err := createConfigMap(rbdDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap: %v", err)
		}
		// Since helm deploys storageclass, skip storageclass creation if
		// ceph-csi is deployed via helm.
		if !helmTest {
			err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
			if err != nil {
				e2elog.Failf("failed to create storageclass: %v", err)
			}
		}
		// create rbd provisioner secret
		key, err := createCephUser(f, keyringRBDProvisionerUsername, rbdProvisionerCaps("", ""))
		if err != nil {
			e2elog.Failf("failed to create user %s: %v", keyringRBDProvisionerUsername, err)
		}
		err = createRBDSecret(f, rbdProvisionerSecretName, keyringRBDProvisionerUsername, key)
		if err != nil {
			e2elog.Failf("failed to create provisioner secret: %v", err)
		}
		// create rbd plugin secret
		key, err = createCephUser(f, keyringRBDNodePluginUsername, rbdNodePluginCaps("", ""))
		if err != nil {
			e2elog.Failf("failed to create user %s: %v", keyringRBDNodePluginUsername, err)
		}
		err = createRBDSecret(f, rbdNodePluginSecretName, keyringRBDNodePluginUsername, key)
		if err != nil {
			e2elog.Failf("failed to create node secret: %v", err)
		}
		deployVault(f.ClientSet, deployTimeout)

		// wait for provisioner deployment
		err = waitForDeploymentComplete(f.ClientSet, rbdDeploymentName, cephCSINamespace, deployTimeout)
		if err != nil {
			e2elog.Failf("timeout waiting for deployment %s: %v", rbdDeploymentName, err)
		}

		// wait for nodeplugin deamonset pods
		err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
		if err != nil {
			e2elog.Failf("timeout waiting for daemonset %s: %v", rbdDaemonsetName, err)
		}

		kernelRelease, err = getKernelVersionFromDaemonset(f, cephCSINamespace, rbdDaemonsetName, "csi-rbdplugin")
		if err != nil {
			e2elog.Failf("failed to get the kernel version: %v", err)
		}
		// default io-timeout=0, needs kernel >= 5.4
		if !util.CheckKernelSupport(kernelRelease, nbdZeroIOtimeoutSupport) {
			nbdMapOptions = "nbd:debug-rbd=20,io-timeout=330"
		}
	})

	AfterEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-rbd", c)
			// log provisioner
			logsCSIPods("app=csi-rbdplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-rbdplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			framework.DumpAllNamespaceInfo(c, cephCSINamespace)
		}

		err := deleteConfigMap(rbdDirPath)
		if err != nil {
			e2elog.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(rbdExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass: %v", err)
		}
		// deleteResource(rbdExamplePath + "snapshotclass.yaml")
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err = deleteNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to delete namespace: %v", err)
				}
			}
		}
		err = deleteNodeLabel(c, nodeRegionLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label: %v", err)
		}
		err = deleteNodeLabel(c, nodeZoneLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label: %v", err)
		}
		// Remove the CSI labels that get added
		err = deleteNodeLabel(c, nodeCSIRegionLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label: %v", err)
		}
		err = deleteNodeLabel(c, nodeCSIZoneLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label: %v", err)
		}
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			// test only if ceph-csi is deployed via helm
			if helmTest {
				By("verify PVC and app binding on helm installation", func() {
					err := validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					//  Deleting the storageclass and secret created by helm
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = deleteResource(rbdExamplePath + "secret.yaml")
					if err != nil {
						e2elog.Failf("failed to delete secret: %v", err)
					}
					// Re-create the RBD storageclass
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
				})
			}
			By("verify generic ephemeral volume support", func() {
				// generic ephemeral volume support is supported from 1.21
				if k8sVersionGreaterEquals(f.ClientSet, 1, 21) {
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
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					// validate images in trash
					err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
					}
				}
			})

			By("validate RBD migration PVC", func() {
				err := setupMigrationCMSecretAndSC(f, "")
				if err != nil {
					e2elog.Failf("failed to setup migration prerequisites: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// Block PVC resize
				err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to resize block PVC: %v", err)
				}

				// FileSystem PVC resize
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize filesystem PVC: %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = tearDownMigrationSetup(f)
				if err != nil {
					e2elog.Failf("failed to tear down migration setup: %v", err)
				}
			})

			By("validate RBD migration+static FileSystem", func() {
				err := setupMigrationCMSecretAndSC(f, "migrationsc")
				if err != nil {
					e2elog.Failf("failed to setup migration prerequisites: %v", err)
				}
				// validate filesystem pvc mount
				err = validateRBDStaticMigrationPVC(f, appPath, "migrationsc", false)
				if err != nil {
					e2elog.Failf("failed to validate rbd migrated static file mode pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = tearDownMigrationSetup(f)
				if err != nil {
					e2elog.Failf("failed to tear down migration setup: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and validate owner", func() {
				err := validateImageOwner(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate owner of pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate normal user pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a Block mode RWOP PVC and bind it to more than one app", func() {
				if k8sVersionGreaterEquals(f.ClientSet, 1, 22) {
					pvc, err := loadPVC(rawPVCRWOPPath)
					if err != nil {
						e2elog.Failf("failed to load PVC: %v", err)
					}
					pvc.Namespace = f.UniqueName

					app, err := loadApp(rawAppRWOPPath)
					if err != nil {
						e2elog.Failf("failed to load application: %v", err)
					}
					app.Namespace = f.UniqueName
					baseAppName := app.Name
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)

					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application: %v", err)
					}
					err = validateRWOPPodCreation(f, pvc, app, baseAppName)
					if err != nil {
						e2elog.Failf("failed to validate RWOP pod creation: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				}
			})

			By("create a RWOP PVC and bind it to more than one app", func() {
				if k8sVersionGreaterEquals(f.ClientSet, 1, 22) {
					pvc, err := loadPVC(pvcRWOPPath)
					if err != nil {
						e2elog.Failf("failed to load PVC: %v", err)
					}
					pvc.Namespace = f.UniqueName

					app, err := loadApp(appRWOPPath)
					if err != nil {
						e2elog.Failf("failed to load application: %v", err)
					}
					app.Namespace = f.UniqueName
					baseAppName := app.Name
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)

					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application: %v", err)
					}
					err = validateRWOPPodCreation(f, pvc, app, baseAppName)
					if err != nil {
						e2elog.Failf("failed to validate RWOP pod creation: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				}
			})

			By("create an erasure coded PVC and bind it to an app", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"dataPool": erasureCodedPool,
						"pool":     defaultRBDPool,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create pvc and application binding: %v", err)
				}
				err = checkPVCDataPoolForImageInPool(f, pvc, defaultRBDPool, "ec-pool")
				if err != nil {
					e2elog.Failf("failed to check data pool for image: %v", err)
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete pvc and application : %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create an erasure coded PVC and validate snapshot restore", func() {
				validatePVCSnapshot(
					defaultCloneCount,
					pvcPath,
					appPath,
					snapshotPath,
					pvcClonePath,
					appClonePath,
					noKMS, noKMS,
					defaultSCName,
					erasureCodedPool,
					f)
			})

			By("create an erasure coded PVC and validate PVC-PVC clone", func() {
				validatePVCClone(
					defaultCloneCount,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					erasureCodedPool,
					noKMS,
					noPVCValidation,
					f)
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with ext4 as the FS ", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"csi.storage.k8s.io/fstype": "ext4"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app using rbd-nbd mounter", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Resize rbd-nbd PVC and check application directory size", func() {
				if util.CheckKernelSupport(kernelRelease, nbdResizeSupport) {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					// Storage class with rbd-nbd mounter
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"mounter":         "rbd-nbd",
							"mapOptions":      nbdMapOptions,
							"cephLogStrategy": e2eDefaultCephLogStrategy,
						},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					// Block PVC resize
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Failf("failed to resize block PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)

					// FileSystem PVC resize
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
				}
			})

			By("create PVC with layering,fast-diff image-features and bind it to an app",
				func() {
					if util.CheckKernelSupport(kernelRelease, fastDiffSupport) {
						err := deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(
							f.ClientSet,
							f,
							defaultSCName,
							nil,
							map[string]string{
								"imageFeatures": "layering,exclusive-lock,object-map,fast-diff",
							},
							deletePolicy)
						if err != nil {
							e2elog.Failf("failed to create storageclass: %v", err)
						}
						err = validatePVCAndAppBinding(pvcPath, appPath, f)
						if err != nil {
							e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 0, defaultRBDPool)
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							e2elog.Failf("failed to create storageclass: %v", err)
						}
					}
				})

			By("create PVC with layering,deep-flatten image-features and bind it to an app",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures": "layering,deep-flatten",
						},
						deletePolicy)
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
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)

					if util.CheckKernelSupport(kernelRelease, deepFlattenSupport) {
						app, aErr := loadApp(appPath)
						if aErr != nil {
							e2elog.Failf("failed to load application: %v", aErr)
						}
						app.Namespace = f.UniqueName
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create application: %v", err)
						}
						// delete pod as we should not create snapshot for in-use pvc
						err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to delete application: %v", err)
						}

					}
					// clean up after ourselves
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				})

			By("create PVC with layering,deep-flatten image-features and bind it to an app",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures": "",
						},
						deletePolicy)
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
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)

					// checking the minimal kernel version for fast-diff as its
					// higher kernel version than other default image features.
					if util.CheckKernelSupport(kernelRelease, fastDiffSupport) {
						app, aErr := loadApp(appPath)
						if aErr != nil {
							e2elog.Failf("failed to load application: %v", aErr)
						}
						app.Namespace = f.UniqueName
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create application: %v", err)
						}
						// delete pod as we should not create snapshot for in-use pvc
						err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to delete application: %v", err)
						}

					}
					// clean up after ourselves
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				})

			By("create PVC with journaling,fast-diff image-features and bind it to an app using rbd-nbd mounter",
				func() {
					if util.CheckKernelSupport(kernelRelease, fastDiffSupport) {
						err := deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
						// Storage class with rbd-nbd mounter
						err = createRBDStorageClass(
							f.ClientSet,
							f,
							defaultSCName,
							nil,
							map[string]string{
								"mounter":       "rbd-nbd",
								"imageFeatures": "layering,journaling,exclusive-lock,object-map,fast-diff",
							},
							deletePolicy)
						if err != nil {
							e2elog.Failf("failed to create storageclass: %v", err)
						}
						err = validatePVCAndAppBinding(pvcPath, appPath, f)
						if err != nil {
							e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 0, defaultRBDPool)
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							e2elog.Failf("failed to create storageclass: %v", err)
						}
					}
				})

			// NOTE: RWX is restricted for FileSystem VolumeMode at ceph-csi,
			// see pull#261 for more details.
			By("Create RWX+Block Mode PVC and bind to multiple pods via deployment using rbd-nbd mounter", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				pvc, err := loadPVC(rawPvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				pvc.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadWriteMany}

				app, err := loadAppDeployment(deployBlockAppPath)
				if err != nil {
					e2elog.Failf("failed to load application deployment: %v", err)
				}
				app.Namespace = f.UniqueName

				err = createPVCAndDeploymentApp(f, "", pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}

				err = waitForDeploymentComplete(f.ClientSet, app.Name, app.Namespace, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment to be in running state: %v", err)
				}

				devPath := app.Spec.Template.Spec.Containers[0].VolumeDevices[0].DevicePath
				cmd := fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10", devPath)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}
				podList, err := f.PodClientNS(app.Namespace).List(context.TODO(), opt)
				if err != nil {
					e2elog.Failf("get pod list failed: %v", err)
				}
				if len(podList.Items) != int(*app.Spec.Replicas) {
					e2elog.Failf("podlist contains %d items, expected %d items", len(podList.Items), *app.Spec.Replicas)
				}
				for _, pod := range podList.Items {
					_, _, err = execCommandInPodWithName(f, cmd, pod.Name, pod.Spec.Containers[0].Name, app.Namespace)
					if err != nil {
						e2elog.Failf("command %q failed: %v", cmd, err)
					}
				}

				err = deletePVCAndDeploymentApp(f, "", pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Create ROX+FS Mode PVC and bind to multiple pods via deployment using rbd-nbd mounter", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application: %v", err)
				}

				// create clone PVC as ROX
				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				appClone, err := loadAppDeployment(deployFSAppPath)
				if err != nil {
					e2elog.Failf("failed to load application deployment: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndDeploymentApp(f, "", pvcClone, appClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}

				err = waitForDeploymentComplete(f.ClientSet, appClone.Name, appClone.Namespace, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment to be in running state: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 3, defaultRBDPool)

				filePath := appClone.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				cmd := fmt.Sprintf("echo 'Hello World' > %s", filePath)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", appClone.Name),
				}
				podList, err := f.PodClientNS(appClone.Namespace).List(context.TODO(), opt)
				if err != nil {
					e2elog.Failf("get pod list failed: %v", err)
				}
				if len(podList.Items) != int(*appClone.Spec.Replicas) {
					e2elog.Failf("podlist contains %d items, expected %d items", len(podList.Items), *appClone.Spec.Replicas)
				}
				for _, pod := range podList.Items {
					var stdErr string
					_, stdErr, err = execCommandInPodWithName(f, cmd, pod.Name, pod.Spec.Containers[0].Name, appClone.Namespace)
					if err != nil {
						e2elog.Logf("command %q failed: %v", cmd, err)
					}
					readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
					if !strings.Contains(stdErr, readOnlyErr) {
						e2elog.Failf(stdErr)
					}
				}

				err = deletePVCAndDeploymentApp(f, "", pvcClone, appClone)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Create ROX+Block Mode PVC and bind to multiple pods via deployment using rbd-nbd mounter", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				// create PVC and bind it to an app
				pvc, err := loadPVC(rawPvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				app, err := loadApp(rawAppPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application: %v", err)
				}

				// create clone PVC as ROX
				pvcClone, err := loadPVC(pvcBlockSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				volumeMode := v1.PersistentVolumeBlock
				pvcClone.Spec.VolumeMode = &volumeMode
				appClone, err := loadAppDeployment(deployBlockAppPath)
				if err != nil {
					e2elog.Failf("failed to load application deployment: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndDeploymentApp(f, "", pvcClone, appClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}

				err = waitForDeploymentComplete(f.ClientSet, appClone.Name, appClone.Namespace, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment to be in running state: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 3, defaultRBDPool)

				devPath := appClone.Spec.Template.Spec.Containers[0].VolumeDevices[0].DevicePath
				cmd := fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10", devPath)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", appClone.Name),
				}
				podList, err := f.PodClientNS(appClone.Namespace).List(context.TODO(), opt)
				if err != nil {
					e2elog.Failf("get pod list failed: %v", err)
				}
				if len(podList.Items) != int(*appClone.Spec.Replicas) {
					e2elog.Failf("podlist contains %d items, expected %d items", len(podList.Items), *appClone.Spec.Replicas)
				}
				for _, pod := range podList.Items {
					var stdErr string
					_, stdErr, err = execCommandInPodWithName(f, cmd, pod.Name, pod.Spec.Containers[0].Name, appClone.Namespace)
					if err != nil {
						e2elog.Logf("command %q failed: %v", cmd, err)
					}
					readOnlyErr := fmt.Sprintf("'%s': Operation not permitted", devPath)
					if !strings.Contains(stdErr, readOnlyErr) {
						e2elog.Failf(stdErr)
					}
				}
				err = deletePVCAndDeploymentApp(f, "", pvcClone, appClone)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("perform IO on rbd-nbd volume after nodeplugin restart", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}

				appOpt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}
				// TODO: Remove this once we ensure that rbd-nbd can sync data
				// from Filesystem layer to backend rbd image as part of its
				// detach or SIGTERM signal handler
				_, stdErr, err := execCommandInPod(
					f,
					fmt.Sprintf("sync %s", app.Spec.Containers[0].VolumeMounts[0].MountPath),
					app.Namespace,
					&appOpt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to sync, err: %v, stdErr: %v ", err, stdErr)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDaemonsetName)
				if err != nil {
					e2elog.Failf("failed to get the labels: %v", err)
				}
				// delete rbd nodeplugin pods
				err = deletePodWithLabel(selector, cephCSINamespace, false)
				if err != nil {
					e2elog.Failf("fail to delete pod: %v", err)
				}

				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset pods: %v", err)
				}

				opt := metav1.ListOptions{
					LabelSelector: selector,
				}
				uname, stdErr, err := execCommandInContainer(f, "uname -a", cephCSINamespace, "csi-rbdplugin", &opt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to run uname cmd : %v, stdErr: %v ", err, stdErr)
				}
				e2elog.Logf("uname -a: %v", uname)
				rpmv, stdErr, err := execCommandInContainer(
					f,
					"rpm -qa | grep rbd-nbd",
					cephCSINamespace,
					"csi-rbdplugin",
					&opt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to run rpm -qa cmd : %v, stdErr: %v ", err, stdErr)
				}
				e2elog.Logf("rbd-nbd package version: %v", rpmv)

				timeout := time.Duration(deployTimeout) * time.Minute
				var reason string
				err = wait.PollImmediate(poll, timeout, func() (bool, error) {
					var runningAttachCmd string
					runningAttachCmd, stdErr, err = execCommandInContainer(
						f,
						"pstree --arguments | grep [r]bd-nbd",
						cephCSINamespace,
						"csi-rbdplugin",
						&opt)
					// if the rbd-nbd process is not running the 'grep' command
					// will return with exit code 1
					if err != nil {
						if strings.Contains(err.Error(), "command terminated with exit code 1") {
							reason = fmt.Sprintf("rbd-nbd process is not running yet: %v", err)
						} else if stdErr != "" {
							reason = fmt.Sprintf("failed to run ps cmd : %v, stdErr: %v", err, stdErr)
						}
						e2elog.Logf("%s", reason)

						return false, nil
					}
					e2elog.Logf("attach command running after restart, runningAttachCmd: %v", runningAttachCmd)

					return true, nil
				})

				if errors.Is(err, wait.ErrWaitTimeout) {
					e2elog.Failf("timed out waiting for the rbd-nbd process: %s", reason)
				}
				if err != nil {
					e2elog.Failf("failed to poll: %v", err)
				}

				// Writes on kernel < 5.4 are failing due to a bug in NBD driver,
				// NBD zero cmd timeout handling is fixed with kernel >= 5.4
				// see https://github.com/ceph/ceph-csi/issues/2204#issuecomment-930941047
				if util.CheckKernelSupport(kernelRelease, nbdZeroIOtimeoutSupport) {
					filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
					_, stdErr, err = execCommandInPod(
						f,
						fmt.Sprintf("echo 'Hello World' > %s", filePath),
						app.Namespace,
						&appOpt)
					if err != nil || stdErr != "" {
						e2elog.Failf("failed to write IO, err: %v, stdErr: %v ", err, stdErr)
					}
				} else {
					e2elog.Logf("kernel %q does not meet recommendation, skipping IO test", kernelRelease)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app using rbd-nbd mounter with encryption", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
						"encrypted":       "true",
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Resize Encrypted Block PVC and check Device size", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				// FileSystem PVC resize
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize filesystem PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// Block PVC resize
				err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to resize block PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, vaultKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultTokensKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tokens-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				// name(space) of the Tenant
				tenant := f.UniqueName

				// create the Secret with Vault Token in the Tenants namespace
				token, err := getSecret(vaultExamplePath + "tenant-token.yaml")
				if err != nil {
					e2elog.Failf("failed to load tenant token from secret: %v", err)
				}
				_, err = c.CoreV1().Secrets(tenant).Create(context.TODO(), &token, metav1.CreateOptions{})
				if err != nil {
					e2elog.Failf("failed to create Secret with tenant token: %v", err)
				}

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, vaultTokensKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// delete the Secret of the Tenant
				err = c.CoreV1().Secrets(tenant).Delete(context.TODO(), token.Name, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete Secret with tenant token: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultTenantSA KMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tenant-sa-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
				if err != nil {
					e2elog.Failf("failed to create ServiceAccount: %v", err)
				}
				defer deleteTenantServiceAccount(f.UniqueName)

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, vaultTenantSAKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with SecretsMetadataKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("test RBD volume encryption with user secrets based SecretsMetadataKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "user-ns-secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				// user provided namespace where secret will be created
				namespace := cephCSINamespace

				// create user Secret
				err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create user Secret: %v", err)
				}

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// delete user secret
				err = retryKubectlFile(namespace,
					kubectlDelete,
					vaultExamplePath+vaultUserSecret,
					deployTimeout,
					"--ignore-not-found=true")
				if err != nil {
					e2elog.Failf("failed to delete user Secret: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By(
				"test RBD volume encryption with user secrets based SecretsMetadataKMS with tenant namespace",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "user-secrets-metadata-test",
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}

					// PVC creation namespace where secret will be created
					namespace := f.UniqueName

					// create user Secret
					err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create user Secret: %v", err)
					}

					err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
					if err != nil {
						e2elog.Failf("failed to validate encrypted pvc: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)

					// delete user secret
					err = retryKubectlFile(
						namespace,
						kubectlDelete,
						vaultExamplePath+vaultUserSecret,
						deployTimeout,
						"--ignore-not-found=true")
					if err != nil {
						e2elog.Failf("failed to delete user Secret: %v", err)
					}

					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
				})

			By(
				"create a PVC and Bind it to an app with journaling/exclusive-lock image-features and rbd-nbd mounter",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures":   "layering,journaling,exclusive-lock",
							"mounter":         "rbd-nbd",
							"mapOptions":      nbdMapOptions,
							"cephLogStrategy": e2eDefaultCephLogStrategy,
						},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					err = validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to validate pvc and application binding: %v", err)
					}
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
				},
			)

			By("create a PVC clone and bind it to an app", func() {
				validatePVCSnapshot(
					defaultCloneCount,
					pvcPath,
					appPath,
					snapshotPath,
					pvcClonePath,
					appClonePath,
					noKMS, noKMS,
					defaultSCName,
					noDataPool,
					f)
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				validatePVCClone(
					defaultCloneCount,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					noDataPool,
					noKMS,
					noPVCValidation,
					f)
			})

			By("create an encrypted PVC snapshot and restore it for an app with VaultKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				validatePVCSnapshot(1,
					pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath,
					vaultKMS, vaultKMS,
					defaultSCName, noDataPool,
					f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Validate PVC restore from vaultKMS to vaultTenantSAKMS", func() {
				restoreSCName := "restore-sc"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				scOpts = map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tenant-sa-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, restoreSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
				if err != nil {
					e2elog.Failf("failed to create ServiceAccount: %v", err)
				}
				defer deleteTenantServiceAccount(f.UniqueName)

				validatePVCSnapshot(1,
					pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath,
					vaultKMS, vaultTenantSAKMS,
					restoreSCName, noDataPool, f)

				err = retryKubectlArgs(cephCSINamespace, kubectlDelete, deployTimeout, "storageclass", restoreSCName)
				if err != nil {
					e2elog.Failf("failed to delete storageclass %q: %v", restoreSCName, err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create an encrypted PVC-PVC clone and bind it to an app", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				validatePVCClone(1,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					noDataPool,
					secretsMetadataKMS,
					isEncryptedPVC,
					f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create an encrypted PVC-PVC clone and bind it to an app with VaultKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				validatePVCClone(1,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					noDataPool,
					vaultKMS,
					isEncryptedPVC,
					f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a block type PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
			})
			By("create a Block mode PVC-PVC clone and bind it to an app", func() {
				_, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Failf("failed to get server version: %v", err)
				}
				validatePVCClone(
					defaultCloneCount,
					rawPvcPath,
					rawAppPath,
					pvcBlockSmartClonePath,
					appBlockSmartClonePath,
					noDataPool,
					noKMS,
					noPVCValidation,
					f)
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
					err := createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application: %v", err)
					}

				}
				// validate created backend rbd images
				validateRBDImageCount(f, totalCount, defaultRBDPool)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application: %v", err)
					}

				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to check data persist: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("Resize Filesystem PVC and check application directory size", func() {
				err := resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize filesystem PVC %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"csi.storage.k8s.io/fstype": "xfs"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize filesystem PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("Resize Block PVC and check Device size", func() {
				err := resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to resize block PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("Test unmount after nodeplugin restart", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to  load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				// delete rbd nodeplugin pods
				err = deletePodWithLabel("app=csi-rbdplugin", cephCSINamespace, false)
				if err != nil {
					e2elog.Failf("fail to delete pod: %v", err)
				}
				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset pods: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create PVC in storageClass with volumeNamePrefix", func() {
				volumeNamePrefix := "foo-bar-"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"volumeNamePrefix": volumeNamePrefix},
					deletePolicy)
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

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				// list RBD images and check if one of them has the same prefix
				foundIt := false
				images, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					e2elog.Failf("failed to list rbd images: %v", err)
				}
				for _, imgName := range images {
					fmt.Printf("Checking prefix on %s\n", imgName)
					if strings.HasPrefix(imgName, volumeNamePrefix) {
						foundIt = true

						break
					}
				}

				// clean up after ourselves
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to  delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				if !foundIt {
					e2elog.Failf("could not find image with prefix %s", volumeNamePrefix)
				}
			})

			By("validate RBD static FileSystem PVC", func() {
				err := validateRBDStaticPV(f, appPath, false, false)
				if err != nil {
					e2elog.Failf("failed to validate rbd static pv: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate RBD static Block PVC", func() {
				err := validateRBDStaticPV(f, rawAppPath, true, false)
				if err != nil {
					e2elog.Failf("failed to validate rbd block pv: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate failure of RBD static PVC without imageFeatures parameter", func() {
				err := validateRBDStaticPV(f, rawAppPath, true, true)
				if err != nil {
					e2elog.Failf("Validation of static PVC without imageFeatures parameter failed with err %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate mount options in app pod", func() {
				mountFlags := []string{"discard"}
				err := checkMountOptions(pvcPath, appPath, f, mountFlags)
				if err != nil {
					e2elog.Failf("failed to check mount options: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("creating an app with a PVC, using a topology constrained StorageClass", func() {
				By("checking node has required CSI topology labels set", func() {
					err := checkNodeHasLabel(f.ClientSet, nodeCSIRegionLabel, regionValue)
					if err != nil {
						e2elog.Failf("failed to check node label: %v", err)
					}
					err = checkNodeHasLabel(f.ClientSet, nodeCSIZoneLabel, zoneValue)
					if err != nil {
						e2elog.Failf("failed to check node label: %v", err)
					}
				})

				By("creating a StorageClass with delayed binding mode and CSI topology parameter")
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"domainSegments\":" +
					"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
					"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName,
					map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
					map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				By("creating an app using a PV from the delayed binding mode StorageClass")
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, 0)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				By("ensuring created PV has required node selector values populated")
				err = checkPVSelectorValuesForPVC(f, pvc)
				if err != nil {
					e2elog.Failf("failed to check pv selector values: %v", err)
				}
				By("ensuring created PV has its image in the topology specific pool")
				err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					e2elog.Failf("failed to check image in pool: %v", err)
				}

				By("ensuring created PV has its image journal in the topology specific pool")
				err = checkPVCImageJournalInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					e2elog.Failf("failed to check image journal: %v", err)
				}

				By("ensuring created PV has its CSI journal in the CSI journal specific pool")
				err = checkPVCCSIJournalInPool(f, pvc, "replicapool")
				if err != nil {
					e2elog.Failf("failed to check csi journal in pool: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}

				By("checking if data pool parameter is honored", func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"dataPool\":\"" + rbdTopologyDataPool +
						"\",\"domainSegments\":" +
						"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
						"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName,
						map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
						map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					By("creating an app using a PV from the delayed binding mode StorageClass with a data pool")
					pvc, app, err = createPVCAndAppBinding(pvcPath, appPath, f, 0)
					if err != nil {
						e2elog.Failf("failed to create PVC and application: %v", err)
					}

					By("ensuring created PV has its image in the topology specific pool")
					err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
					if err != nil {
						e2elog.Failf("failed to check  pvc image in pool: %v", err)
					}

					By("ensuring created image has the right data pool parameter set")
					err = checkPVCDataPoolForImageInPool(f, pvc, rbdTopologyPool, rbdTopologyDataPool)
					if err != nil {
						e2elog.Failf("failed to check data pool for image: %v", err)
					}

					// cleanup and undo changes made by the test
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application: %v", err)
					}
				})

				// cleanup and undo changes made by the test
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			// Mount pvc to pod with invalid mount option,expected that
			// mounting will fail
			By("Mount pvc to pod with invalid mount option", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					map[string]string{rbdMountOptions: "debug,invalidOption"},
					nil,
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to  load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				// create an app and wait for 1 min for it to go to running state
				err = createApp(f.ClientSet, app, 1)
				if err == nil {
					e2elog.Failf("application should not go to running state due to invalid mount option")
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create rbd clones in different pool", func() {
				clonePool := "clone-test"
				// create pool for clones
				err := createPool(f, clonePool)
				if err != nil {
					e2elog.Failf("failed to create pool %s: %v", clonePool, err)
				}
				err = createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create snapshotclass: %v", err)
				}
				cloneSC := "clone-storageclass"
				param := map[string]string{
					"pool": clonePool,
				}
				// create new storageclass with new pool
				err = createRBDStorageClass(f.ClientSet, f, cloneSC, nil, param, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validateCloneInDifferentPool(f, defaultRBDPool, cloneSC, clonePool)
				if err != nil {
					e2elog.Failf("failed to validate clones in different pool: %v", err)
				}

				err = retryKubectlArgs(
					cephCSINamespace,
					kubectlDelete,
					deployTimeout,
					"sc",
					cloneSC,
					"--ignore-not-found=true")
				if err != nil {
					e2elog.Failf("failed to delete storageclass %s: %v", cloneSC, err)
				}

				err = deleteResource(rbdExamplePath + "snapshotclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete snapshotclass: %v", err)
				}
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, clonePool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", clonePool, err)
				}
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
				}

				err = deletePool(clonePool, false, f)
				if err != nil {
					e2elog.Failf("failed to delete pool %s: %v", clonePool, err)
				}
			})

			By("create ROX PVC clone and mount it to multiple pods", func() {
				err := createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				// delete pod as we should not create snapshot for in-use pvc
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				// create clone PVC as ROX
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				// parent pvc+ snapshot + clone
				totalImages = 3
				validateRBDImageCount(f, totalImages, defaultRBDPool)

				appClone, err := loadApp(appClonePath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}

				totalCount := 2
				appClone.Namespace = f.UniqueName
				appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					label := map[string]string{
						"app": name,
					}
					appClone.Labels = label
					appClone.Name = name
					err = createApp(f.ClientSet, appClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application: %v", err)
					}
				}

				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					opt := metav1.ListOptions{
						LabelSelector: fmt.Sprintf("app=%s", name),
					}

					filePath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
					_, stdErr := execCommandInPodAndAllowFail(
						f,
						fmt.Sprintf("echo 'Hello World' > %s", filePath),
						appClone.Namespace,
						&opt)
					readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
					if !strings.Contains(stdErr, readOnlyErr) {
						e2elog.Failf(stdErr)
					}
				}

				// delete app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					appClone.Name = name
					err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application: %v", err)
					}
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate PVC mounting if snapshot and parent PVC are deleted", func() {
				err := createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}

				// delete parent PVC
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				// create clone PVC
				pvcClone.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images = snapshot + clone
				totalImages = 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)

				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot: %v", err)
				}

				// validate created backend rbd images = clone
				totalImages = 1
				validateRBDImageCount(f, totalImages, defaultRBDPool)

				appClone, err := loadApp(appClonePath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

				// create application
				err = createApp(f.ClientSet, appClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create application: %v", err)
				}

				err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application: %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By(
				"validate PVC mounting if snapshot and parent PVC are deleted chained with depth 2",
				func() {
					snapChainDepth := 2

					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}

					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"encrypted":       "true",
							"encryptionKMSID": "vault-test",
						},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}

					err = createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}

					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							e2elog.Failf("failed to create storageclass: %v", err)
						}
					}()

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
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					for i := 0; i < snapChainDepth; i++ {
						var pvcClone *v1.PersistentVolumeClaim
						snap := getSnapshot(snapshotPath)
						snap.Name = fmt.Sprintf("%s-%d", snap.Name, i)
						snap.Namespace = f.UniqueName
						snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

						err = createSnapshot(&snap, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create snapshot: %v", err)
						}
						// validate created backend rbd images
						// parent PVC + snapshot
						totalImages := 2
						validateRBDImageCount(f, totalImages, defaultRBDPool)
						pvcClone, err = loadPVC(pvcClonePath)
						if err != nil {
							e2elog.Failf("failed to load PVC: %v", err)
						}

						// delete parent PVC
						err = deletePVCAndApp("", f, pvc, app)
						if err != nil {
							e2elog.Failf("failed to delete PVC and application: %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 1, defaultRBDPool)

						// create clone PVC
						pvcClone.Name = fmt.Sprintf("%s-%d", pvcClone.Name, i)
						pvcClone.Namespace = f.UniqueName
						pvcClone.Spec.DataSource.Name = snap.Name
						err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create PVC: %v", err)
						}
						// validate created backend rbd images = snapshot + clone
						totalImages = 2
						validateRBDImageCount(f, totalImages, defaultRBDPool)

						// delete snapshot
						err = deleteSnapshot(&snap, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to delete snapshot: %v", err)
						}

						// validate created backend rbd images = clone
						totalImages = 1
						validateRBDImageCount(f, totalImages, defaultRBDPool)

						app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
						// create application
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create application: %v", err)
						}

						pvc = pvcClone
					}

					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application: %v", err)
					}
					// delete PVC clone
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				})

			By("validate PVC Clone chained with depth 2", func() {
				cloneChainDepth := 2

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}

				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "vault-test",
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
				}()

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				for i := 0; i < cloneChainDepth; i++ {
					var pvcClone *v1.PersistentVolumeClaim
					pvcClone, err = loadPVC(pvcSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to load PVC: %v", err)
					}

					// create clone PVC
					pvcClone.Name = fmt.Sprintf("%s-%d", pvcClone.Name, i)
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.DataSource.Name = pvc.Name
					err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC: %v", err)
					}

					// delete parent PVC
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application: %v", err)
					}

					app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
					// create application
					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application: %v", err)
					}

					pvc = pvcClone
				}

				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application: %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("ensuring all operations will work within a rados namespace", func() {
				updateConfigMap := func(radosNS string) {
					radosNamespace = radosNS
					err := deleteConfigMap(rbdDirPath)
					if err != nil {
						e2elog.Failf("failed to delete configmap:: %v", err)
					}
					err = createConfigMap(rbdDirPath, f.ClientSet, f)
					if err != nil {
						e2elog.Failf("failed to create configmap: %v", err)
					}
					err = createRadosNamespace(f)
					if err != nil {
						e2elog.Failf("failed to create rados namespace: %v", err)
					}
					// delete csi pods
					err = deletePodWithLabel("app in (ceph-csi-rbd, csi-rbdplugin, csi-rbdplugin-provisioner)",
						cephCSINamespace, false)
					if err != nil {
						e2elog.Failf("failed to delete pods with labels: %v", err)
					}
					// wait for csi pods to come up
					err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("timeout waiting for daemonset pods: %v", err)
					}
					err = waitForDeploymentComplete(f.ClientSet, rbdDeploymentName, cephCSINamespace, deployTimeout)
					if err != nil {
						e2elog.Failf("timeout waiting for deployment to be in running state: %v", err)
					}
				}

				updateConfigMap("e2e-ns")
				// create rbd provisioner secret
				key, err := createCephUser(
					f,
					keyringRBDNamespaceProvisionerUsername,
					rbdProvisionerCaps(defaultRBDPool, radosNamespace),
				)
				if err != nil {
					e2elog.Failf("failed to create user %s: %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceProvisionerSecretName, keyringRBDNamespaceProvisionerUsername, key)
				if err != nil {
					e2elog.Failf("failed to create provisioner secret: %v", err)
				}
				// create rbd plugin secret
				key, err = createCephUser(
					f,
					keyringRBDNamespaceNodePluginUsername,
					rbdNodePluginCaps(defaultRBDPool, radosNamespace))
				if err != nil {
					e2elog.Failf("failed to create user %s: %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceNodePluginSecretName, keyringRBDNamespaceNodePluginUsername, key)
				if err != nil {
					e2elog.Failf("failed to create node secret: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				param := make(map[string]string)
				// override existing secrets
				param["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName
				param["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName
				param["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, param, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				err = validateImageOwner(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate owner of pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// Create a PVC and bind it to an app within the namesapce
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}

				// Resize Block PVC and check Device size within the namespace
				err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to resize block PVC: %v", err)
				}

				// Resize Filesystem PVC and check application directory size
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize filesystem PVC %v", err)
				}

				// Create a PVC clone and bind it to an app within the namespace
				err = createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				pvc, pvcErr := loadPVC(pvcPath)
				if pvcErr != nil {
					e2elog.Failf("failed to load PVC: %v", pvcErr)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot: %v", err)
				}
				validateRBDImageCount(f, 2, defaultRBDPool)

				err = validatePVCAndAppBinding(pvcClonePath, appClonePath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot: %v", err)
				}
				// as snapshot is deleted the image count should be one
				validateRBDImageCount(f, 1, defaultRBDPool)

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}

				// delete RBD provisioner secret
				err = deleteCephUser(f, keyringRBDNamespaceProvisionerUsername)
				if err != nil {
					e2elog.Failf("failed to delete user %s: %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = c.CoreV1().
					Secrets(cephCSINamespace).
					Delete(context.TODO(), rbdNamespaceProvisionerSecretName, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete provisioner secret: %v", err)
				}
				// delete RBD plugin secret
				err = deleteCephUser(f, keyringRBDNamespaceNodePluginUsername)
				if err != nil {
					e2elog.Failf("failed to delete user %s: %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = c.CoreV1().
					Secrets(cephCSINamespace).
					Delete(context.TODO(), rbdNamespaceNodePluginSecretName, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete node secret: %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				updateConfigMap("")
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
					e2elog.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

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
					e2elog.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a PVC and Bind it to an app for mapped rbd image with options", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, map[string]string{
					"mapOptions":   "lock_on_read,queue_depth=1024",
					"unmapOptions": "force",
				}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding: %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("validate the functionality of controller", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass : %v", err)
				}
				scParams := map[string]string{
					"volumeNamePrefix": "test-",
				}
				err = validateController(f,
					pvcPath, appPath, rbdExamplePath+"storageclass.yaml",
					nil,
					scParams)
				if err != nil {
					e2elog.Failf("failed to validate controller : %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass : %v", err)
				}
			})

			By("validate image deletion when it is moved to trash", func() {
				// make sure pool is empty
				validateRBDImageCount(f, 0, defaultRBDPool)

				err := createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load pvc: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create pvc: %v", err)
				}

				pvcSmartClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load pvcSmartClone: %v", err)
				}
				pvcSmartClone.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create pvc: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot: %v", err)
				}

				smartCloneImageData, err := getImageInfoFromPVC(pvcSmartClone.Namespace, pvcSmartClone.Name, f)
				if err != nil {
					e2elog.Failf("failed to get ImageInfo from pvc: %v", err)
				}

				imageList, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					e2elog.Failf("failed to list rbd images: %v", err)
				}
				for _, imageName := range imageList {
					if imageName == smartCloneImageData.imageName {
						// do not move smartclone image to trash to test
						// temporary image clone cleanup.
						continue
					}
					_, _, err = execCommandInToolBoxPod(f,
						fmt.Sprintf("rbd snap purge %s %s", rbdOptions(defaultRBDPool), imageName), rookNamespace)
					if err != nil {
						e2elog.Failf(
							"failed to snap purge %s %s: %v",
							imageName,
							rbdOptions(defaultRBDPool),
							err)
					}
					_, _, err = execCommandInToolBoxPod(f,
						fmt.Sprintf("rbd trash move %s %s", rbdOptions(defaultRBDPool), imageName), rookNamespace)
					if err != nil {
						e2elog.Failf(
							"failed to move rbd image %s %s to trash: %v",
							imageName,
							rbdOptions(defaultRBDPool),
							err)
					}
				}

				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete pvc: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete pvc: %v", err)
				}

				validateRBDImageCount(f, 0, defaultRBDPool)

				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in trash %s: %v", rbdOptions(defaultRBDPool), err)
				}
			})

			By("validate stale images in trash", func() {
				err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
				}
			})

			By("restore snapshot to a bigger size PVC", func() {
				By("restore snapshot to bigger size pvc", func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					defer func() {
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
					}()
					err = createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create VolumeSnapshotClass: %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()
					// validate filesystem mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						pvcPath,
						appPath,
						snapshotPath,
						pvcClonePath,
						appClonePath)
					if err != nil {
						e2elog.Failf("failed to validate restore bigger size clone: %v", err)
					}
					// validate block mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						rawPvcPath,
						rawAppPath,
						snapshotPath,
						pvcBlockRestorePath,
						appBlockRestorePath)
					if err != nil {
						e2elog.Failf("failed to validate restore bigger size clone: %v", err)
					}
				})

				By("restore snapshot to bigger size encrypted PVC with VaultKMS", func() {
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "vault-test",
					}
					err := createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					defer func() {
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
					}()
					err = createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create VolumeSnapshotClass: %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()
					// validate filesystem mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						pvcPath,
						appPath,
						snapshotPath,
						pvcClonePath,
						appClonePath)
					if err != nil {
						e2elog.Failf("failed to validate restore bigger size clone: %v", err)
					}
					// validate block mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						rawPvcPath,
						rawAppPath,
						snapshotPath,
						pvcBlockRestorePath,
						appBlockRestorePath)
					if err != nil {
						e2elog.Failf("failed to validate restore bigger size clone: %v", err)
					}
				})

				By("validate image deletion", func() {
					validateRBDImageCount(f, 0, defaultRBDPool)
					err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
					}
				})
			})

			By("clone PVC to a bigger size PVC", func() {
				By("clone PVC to bigger size encrypted PVC with VaultKMS", func() {
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "vault-test",
					}
					err := createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					defer func() {
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass: %v", err)
						}
					}()

					// validate filesystem mode PVC
					err = validateBiggerCloneFromPVC(f,
						pvcPath,
						appPath,
						pvcSmartClonePath,
						appSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to validate bigger size clone: %v", err)
					}
					// validate block mode PVC
					err = validateBiggerCloneFromPVC(f,
						rawPvcPath,
						rawAppPath,
						pvcBlockSmartClonePath,
						appBlockSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to validate bigger size clone: %v", err)
					}
				})

				By("clone PVC to bigger size pvc", func() {
					err := createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
					// validate filesystem mode PVC
					err = validateBiggerCloneFromPVC(f,
						pvcPath,
						appPath,
						pvcSmartClonePath,
						appSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to validate bigger size clone: %v", err)
					}
					// validate block mode PVC
					err = validateBiggerCloneFromPVC(f,
						rawPvcPath,
						rawAppPath,
						pvcBlockSmartClonePath,
						appBlockSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to validate bigger size clone: %v", err)
					}
				})

				By("validate image deletion", func() {
					validateRBDImageCount(f, 0, defaultRBDPool)
					err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
					}
				})
			})

			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, false, f)
				if err != nil {
					e2elog.Failf("failed to delete PVC when pool not found: %v", err)
				}
			})
			// delete RBD provisioner secret
			err := deleteCephUser(f, keyringRBDProvisionerUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s: %v", keyringRBDProvisionerUsername, err)
			}
			// delete RBD plugin secret
			err = deleteCephUser(f, keyringRBDNodePluginUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s: %v", keyringRBDNodePluginUsername, err)
			}
		})
	})
})
