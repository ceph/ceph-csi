package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	csiDriverObject    = "csidriver.yaml"
	rbdDirPath         = "../deploy/rbd/kubernetes/"
	rbdExamplePath     = "../examples/rbd/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	defaultRBDPool     = "replicapool"
	// Topology related variables
	nodeRegionLabel     = "test.failure-domain/region"
	regionValue         = "testregion"
	nodeZoneLabel       = "test.failure-domain/zone"
	zoneValue           = "testzone"
	nodeCSIRegionLabel  = "topology.rbd.csi.ceph.com/region"
	nodeCSIZoneLabel    = "topology.rbd.csi.ceph.com/zone"
	rbdTopologyPool     = "newrbdpool"
	rbdTopologyDataPool = "replicapool" // NOTE: should be different than rbdTopologyPool for test to be effective
)

func deployRBDPlugin() {
	// delete objects deployed by rook
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "--ignore-not-found=true", ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Failf("failed to delete provisioner rbac %s with error %v", rbdDirPath+rbdProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "delete", "--ignore-not-found=true", ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to delete nodeplugin rbac %s with error %v", rbdDirPath+rbdNodePluginRBAC, err)
	}

	createORDeleteRbdResources("create")
}

func deleteRBDPlugin() {
	createORDeleteRbdResources("delete")
}

func createORDeleteRbdResources(action string) {
	csiDriver, err := ioutil.ReadFile(rbdDirPath + csiDriverObject)
	if err != nil {
		// createORDeleteRbdResources is used for upgrade testing as csidriverObject is
		// newly added, discarding file not found error.
		if !os.IsNotExist(err) {
			e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+csiDriverObject, err)
		}
	} else {
		_, err = framework.RunKubectlInput(cephCSINamespace, string(csiDriver), action, "-f", "-")
		if err != nil {
			e2elog.Failf("failed to %s CSIDriver object with error %v", action, err)
		}
	}
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisioner)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisioner, err)
	}
	data = oneReplicaDeployYaml(data)
	data = enableTopologyInTemplate(data)
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s rbd provisioner with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s provisioner rbac with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisionerPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s provisioner psp with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePlugin)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePlugin, err)
	}

	domainLabel := nodeRegionLabel + "," + nodeZoneLabel
	data = addTopologyDomainsToDSYaml(data, domainLabel)
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin rbac with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePluginPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin psp with error %v", action, err)
	}
}

func validateRBDImageCount(f *framework.Framework, count int) {
	imageList, err := listRBDImages(f)
	if err != nil {
		e2elog.Failf("failed to list rbd images with error %v", err)
	}
	if len(imageList) != count {
		e2elog.Failf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(imageList), count)
	}
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("rbd")
	var c clientset.Interface
	// deploy RBD CSI
	BeforeEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		c = f.ClientSet
		if deployRBD {
			err := createNodeLabel(f, nodeRegionLabel, regionValue)
			if err != nil {
				e2elog.Failf("failed to create node label with error %v", err)
			}
			err = createNodeLabel(f, nodeZoneLabel, zoneValue)
			if err != nil {
				e2elog.Failf("failed to create node label with error %v", err)
			}
			if cephCSINamespace != defaultNs {
				err = createNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to create namespace with error %v", err)
				}
			}
			deployRBDPlugin()
		}
		err := createConfigMap(rbdDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap with error %v", err)
		}
		err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
		if err != nil {
			e2elog.Failf("failed to create storageclass with error %v", err)
		}
		// create rbd provisioner secret
		key, err := createCephUser(f, keyringRBDProvisionerUsername, rbdProvisionerCaps("", ""))
		if err != nil {
			e2elog.Failf("failed to create user %s with error %v", keyringRBDProvisionerUsername, err)
		}
		err = createRBDSecret(f, rbdProvisionerSecretName, keyringRBDProvisionerUsername, key)
		if err != nil {
			e2elog.Failf("failed to create provisioner secret with error %v", err)
		}
		// create rbd plugin secret
		key, err = createCephUser(f, keyringRBDNodePluginUsername, rbdNodePluginCaps("", ""))
		if err != nil {
			e2elog.Failf("failed to create user %s with error %v", keyringRBDNodePluginUsername, err)
		}
		err = createRBDSecret(f, rbdNodePluginSecretName, keyringRBDNodePluginUsername, key)
		if err != nil {
			e2elog.Failf("failed to create node secret with error %v", err)
		}
		deployVault(f.ClientSet, deployTimeout)
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
			e2elog.Failf("failed to delete configmap with error %v", err)
		}
		err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), rbdProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete provisioner secret with error %v", err)
		}
		err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), rbdNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete node secret with error %v", err)
		}
		err = deleteResource(rbdExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass with error %v", err)
		}
		// deleteResource(rbdExamplePath + "snapshotclass.yaml")
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err = deleteNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to delete namespace with error %v", err)
				}
			}
		}
		err = deleteNodeLabel(c, nodeRegionLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
		err = deleteNodeLabel(c, nodeZoneLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
		// Remove the CSI labels that get added
		err = deleteNodeLabel(c, nodeCSIRegionLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
		err = deleteNodeLabel(c, nodeCSIZoneLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"
			rawPvcPath := rbdExamplePath + "raw-block-pvc.yaml"
			rawAppPath := rbdExamplePath + "raw-block-pod.yaml"
			pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
			pvcSmartClonePath := rbdExamplePath + "pvc-clone.yaml"
			pvcBlockSmartClonePath := rbdExamplePath + "pvc-block-clone.yaml"
			appClonePath := rbdExamplePath + "pod-restore.yaml"
			appSmartClonePath := rbdExamplePath + "pod-clone.yaml"
			appBlockSmartClonePath := rbdExamplePath + "block-pod-clone.yaml"
			snapshotPath := rbdExamplePath + "snapshot.yaml"
			defaultCloneCount := 10

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment %s with error %v", rbdDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset %s with error %v", rbdDaemonsetName, err)
				}
			})

			By("create a PVC and validate owner", func() {
				err := validateImageOwner(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate owner of pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("create a PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate normal user pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("create a PVC and bind it to an app with ext4 as the FS ", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"csi.storage.k8s.io/fstype": "ext4"}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"encrypted": "true"}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, "", f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, "vault", f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultTokensKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tokens-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
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

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, "vaulttokens", f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)

				// delete the Secret of the Tenant
				err = c.CoreV1().Secrets(tenant).Delete(context.TODO(), token.Name, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete Secret with tenant token: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with SecretsMetadataKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, "", f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			// TODO: enable this test when we support rbd-nbd mounter in E2E.
			// nbd module should be present on the host machine to run use the
			// rbd-nbd mounter.

			// By("create a PVC and Bind it to an app with journaling/exclusive-lock image-features and rbd-nbd mounter", func() {
			// 	deleteResource(rbdExamplePath + "storageclass.yaml")
			// 	createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"imageFeatures": "layering,journaling,exclusive-lock", "mounter": "rbd-nbd"})
			// 	validatePVCAndAppBinding(pvcPath, appPath, f)
			// 	deleteResource(rbdExamplePath + "storageclass.yaml")
			// 	createRBDStorageClass(f.ClientSet, f, nil, make(map[string]string))
			// })

			By("create a PVC clone and bind it to an app", func() {
				// snapshot beta is only supported from v1.17+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					validatePVCSnapshot(defaultCloneCount, pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath, "", false, f)
				}
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				// pvc clone is only supported from v1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					validatePVCClone(defaultCloneCount, pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, noPVCValidation, f)
				}
			})

			By("create a thick-provisioned PVC-PVC clone and bind it to an app", func() {
				// pvc clone is only supported from v1.16+
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{
					"thickProvision": "true"}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCClone(1, pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, isThickPVC, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create an encrypted PVC snapshot and restore it for an app with VaultKMS", func() {
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCSnapshot(1, pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath, "vault", true, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create an encrypted PVC-PVC clone and bind it to an app", func() {
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCClone(1, pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, isEncryptedPVC, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a block type PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
			})
			By("create a Block mode PVC-PVC clone and bind it to an app", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Failf("failed to get server version with error %v", err)
				}
				// pvc clone is only supported from v1.16+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "16") {
					validatePVCClone(defaultCloneCount, rawPvcPath, rawAppPath, pvcBlockSmartClonePath, appBlockSmartClonePath, noPVCValidation, f)
				}
			})
			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}

				}
				// validate created backend rbd images
				validateRBDImageCount(f, totalCount)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application with error %v", err)
					}

				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to check data persist with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("Resize Filesystem PVC and check application directory size", func() {
				// Resize 0.3.0 is only supported from v1.15+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 15) {
					err := resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC %v", err)
					}

					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"csi.storage.k8s.io/fstype": "xfs"}, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC with error %v", err)

					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0)
				}
			})

			By("Resize Block PVC and check Device size", func() {
				// Block PVC resize is supported in kubernetes 1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					err := resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Failf("failed to resize block PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0)
				}
			})

			By("Test unmount after nodeplugin restart", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to  load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1)
				// delete rbd nodeplugin pods
				err = deletePodWithLabel("app=csi-rbdplugin", cephCSINamespace, false)
				if err != nil {
					e2elog.Failf("fail to delete pod with error %v", err)
				}
				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset pods with error %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("create PVC in storageClass with volumeNamePrefix", func() {
				volumeNamePrefix := "foo-bar-"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"volumeNamePrefix": volumeNamePrefix}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1)
				// list RBD images and check if one of them has the same prefix
				foundIt := false
				images, err := listRBDImages(f)
				if err != nil {
					e2elog.Failf("failed to list rbd images with error %v", err)
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
					e2elog.Failf("failed to  delete PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				if !foundIt {
					e2elog.Failf("could not find image with prefix %s", volumeNamePrefix)
				}
			})

			By("validate RBD static FileSystem PVC", func() {
				err := validateRBDStaticPV(f, appPath, false)
				if err != nil {
					e2elog.Failf("failed to validate rbd static pv with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("validate RBD static Block PVC", func() {
				err := validateRBDStaticPV(f, rawAppPath, true)
				if err != nil {
					e2elog.Failf("failed to validate rbd block pv with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("validate mount options in app pod", func() {
				mountFlags := []string{"discard"}
				err := checkMountOptions(pvcPath, appPath, f, mountFlags)
				if err != nil {
					e2elog.Failf("failed to check mount options with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("creating an app with a PVC, using a topology constrained StorageClass", func() {
				By("checking node has required CSI topology labels set", func() {
					err := checkNodeHasLabel(f.ClientSet, nodeCSIRegionLabel, regionValue)
					if err != nil {
						e2elog.Failf("failed to check node label with error %v", err)
					}
					err = checkNodeHasLabel(f.ClientSet, nodeCSIZoneLabel, zoneValue)
					if err != nil {
						e2elog.Failf("failed to check node label with error %v", err)
					}
				})

				By("creating a StorageClass with delayed binding mode and CSI topology parameter")
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"domainSegments\":" +
					"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
					"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
				err = createRBDStorageClass(f.ClientSet, f,
					map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
					map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				By("creating an app using a PV from the delayed binding mode StorageClass")
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, 0)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				By("ensuring created PV has required node selector values populated")
				err = checkPVSelectorValuesForPVC(f, pvc)
				if err != nil {
					e2elog.Failf("failed to check pv selector values with error %v", err)
				}
				By("ensuring created PV has its image in the topology specific pool")
				err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					e2elog.Failf("failed to check image in pool with error %v", err)
				}

				By("ensuring created PV has its image journal in the topology specific pool")
				err = checkPVCImageJournalInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					e2elog.Failf("failed to check image journal with error %v", err)
				}

				By("ensuring created PV has its CSI journal in the CSI journal specific pool")
				err = checkPVCCSIJournalInPool(f, pvc, "replicapool")
				if err != nil {
					e2elog.Failf("failed to check csi journal in pool with error %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}

				By("checking if data pool parameter is honored", func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"dataPool\":\"" + rbdTopologyDataPool +
						"\",\"domainSegments\":" +
						"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
						"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
					err = createRBDStorageClass(f.ClientSet, f,
						map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
						map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					By("creating an app using a PV from the delayed binding mode StorageClass with a data pool")
					pvc, app, err = createPVCAndAppBinding(pvcPath, appPath, f, 0)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}

					By("ensuring created PV has its image in the topology specific pool")
					err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
					if err != nil {
						e2elog.Failf("failed to check  pvc image in pool with error %v", err)
					}

					By("ensuring created image has the right data pool parameter set")
					err = checkPVCDataPoolForImageInPool(f, pvc, rbdTopologyPool, rbdTopologyDataPool)
					if err != nil {
						e2elog.Failf("failed to check data pool for image with error %v", err)
					}

					// cleanup and undo changes made by the test
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application with error %v", err)
					}
				})

				// cleanup and undo changes made by the test
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			// Mount pvc to pod with invalid mount option,expected that
			// mounting will fail
			By("Mount pvc to pod with invalid mount option", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, map[string]string{rbdMountOptions: "debug,invalidOption"}, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to  load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1)

				// create an app and wait for 1 min for it to go to running state
				err = createApp(f.ClientSet, app, 1)
				if err == nil {
					e2elog.Failf("application should not go to running state due to invalid mount option")
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create ROX PVC clone and mount it to multiple pods", func() {
				// snapshot beta is only supported from v1.17+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					err := createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
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
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					pvc.Namespace = f.UniqueName
					app, err := loadApp(appPath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					app.Namespace = f.UniqueName
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1)
					// delete pod as we should not create snapshot for in-use pvc
					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application with error %v", err)
					}

					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create snapshot with error %v", err)
					}
					// validate created backend rbd images
					// parent PVC + snapshot
					totalImages := 2
					validateRBDImageCount(f, totalImages)
					pvcClone, err := loadPVC(pvcClonePath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					// create clone PVC as ROX
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
					err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}
					// validate created backend rbd images
					// parent pvc+ snapshot + clone
					totalImages = 3
					validateRBDImageCount(f, totalImages)

					appClone, err := loadApp(appClonePath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
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
							e2elog.Failf("failed to create application with error %v", err)
						}
					}

					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						opt := metav1.ListOptions{
							LabelSelector: fmt.Sprintf("app=%s", name),
						}

						filePath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
						_, stdErr := execCommandInPodAndAllowFail(f, fmt.Sprintf("echo 'Hello World' > %s", filePath), appClone.Namespace, &opt)
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
							e2elog.Failf("failed to delete application with error %v", err)
						}
					}
					// delete PVC clone
					err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					// delete snapshot
					err = deleteSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete snapshot with error %v", err)
					}
					// delete parent pvc
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0)
				}
			})

			By("validate PVC mounting if snapshot and parent PVC are deleted", func() {
				// snapshot beta is only supported from v1.17+
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					Skip("pvc restore is only supported from v1.17+")
				}
				err := createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
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
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				pvc.Namespace = f.UniqueName
				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1)

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot with error %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				// delete parent PVC
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1)

				// create clone PVC
				pvcClone.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}
				// validate created backend rbd images = snapshot + clone
				totalImages = 2
				validateRBDImageCount(f, totalImages)

				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot with error %v", err)
				}

				// validate created backend rbd images = clone
				totalImages = 1
				validateRBDImageCount(f, totalImages)

				appClone, err := loadApp(appClonePath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

				// create application
				err = createApp(f.ClientSet, appClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create application with error %v", err)
				}

				err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application with error %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("ensuring all operations will work within a rados namespace", func() {
				updateConfigMap := func(radosNS string) {
					radosNamespace = radosNS
					err := deleteConfigMap(rbdDirPath)
					if err != nil {
						e2elog.Failf("failed to delete configmap with Error: %v", err)
					}
					err = createConfigMap(rbdDirPath, f.ClientSet, f)
					if err != nil {
						e2elog.Failf("failed to create configmap with error %v", err)
					}
					err = createRadosNamespace(f)
					if err != nil {
						e2elog.Failf("failed to create rados namespace with error %v", err)
					}
					// delete csi pods
					err = deletePodWithLabel("app in (ceph-csi-rbd, csi-rbdplugin, csi-rbdplugin-provisioner)",
						cephCSINamespace, false)
					if err != nil {
						e2elog.Failf("failed to delete pods with labels with error %v", err)
					}
					// wait for csi pods to come up
					err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("timeout waiting for daemonset pods with error %v", err)
					}
					err = waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("timeout waiting for deployment to be in running state with error %v", err)
					}
				}

				updateConfigMap("e2e-ns")
				// create rbd provisioner secret
				key, err := createCephUser(f, keyringRBDNamespaceProvisionerUsername, rbdProvisionerCaps(defaultRBDPool, radosNamespace))
				if err != nil {
					e2elog.Failf("failed to create user %s with error %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceProvisionerSecretName, keyringRBDNamespaceProvisionerUsername, key)
				if err != nil {
					e2elog.Failf("failed to create provisioner secret with error %v", err)
				}
				// create rbd plugin secret
				key, err = createCephUser(f, keyringRBDNamespaceNodePluginUsername, rbdNodePluginCaps(defaultRBDPool, radosNamespace))
				if err != nil {
					e2elog.Failf("failed to create user %s with error %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceNodePluginSecretName, keyringRBDNamespaceNodePluginUsername, key)
				if err != nil {
					e2elog.Failf("failed to create node secret with error %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				param := make(map[string]string)
				// override existing secrets
				param["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName
				param["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName
				param["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

				err = createRBDStorageClass(f.ClientSet, f, nil, param, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				err = validateImageOwner(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate owner of pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)

				// Create a PVC and bind it to an app within the namesapce
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}

				// Resize Block PVC and check Device size within the namespace
				// Block PVC resize is supported in kubernetes 1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Failf("failed to resize block PVC with error %v", err)
					}
				}

				// Resize Filesystem PVC and check application directory size
				// Resize 0.3.0 is only supported from v1.15+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 15) {
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC %v", err)
					}
				}

				// Create a PVC clone and bind it to an app within the namespace
				// snapshot beta is only supported from v1.17+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					err = createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()

					var pvc = &v1.PersistentVolumeClaim{}
					pvc, err = loadPVC(pvcPath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1)

					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create snapshot with error %v", err)
					}
					validateRBDImageCount(f, 2)

					err = validatePVCAndAppBinding(pvcClonePath, appClonePath, f)
					if err != nil {
						e2elog.Failf("failed to validate pvc and application binding with error %v", err)
					}
					err = deleteSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete snapshot with error %v", err)
					}
					// as snapshot is deleted the image count should be one
					validateRBDImageCount(f, 1)

					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					validateRBDImageCount(f, 0)
				}

				// delete RBD provisioner secret
				err = deleteCephUser(f, keyringRBDNamespaceProvisionerUsername)
				if err != nil {
					e2elog.Failf("failed to delete user %s with error %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), rbdNamespaceProvisionerSecretName, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete provisioner secret with error %v", err)
				}
				// delete RBD plugin secret
				err = deleteCephUser(f, keyringRBDNamespaceNodePluginUsername)
				if err != nil {
					e2elog.Failf("failed to delete user %s with error %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), rbdNamespaceNodePluginSecretName, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete node secret with error %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				updateConfigMap("")
			})

			By("Mount pvc as readonly in pod", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
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
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr := execCommandInPodAndAllowFail(f, fmt.Sprintf("echo 'Hello World' > %s", filePath), app.Namespace, &opt)
				readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
				if !strings.Contains(stdErr, readOnlyErr) {
					e2elog.Failf(stdErr)
				}

				// delete PVC and app
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
			})

			By("create a thick-provisioned PVC", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{
					"thickProvision": "true"}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				pvc, err := loadPVC(rawPvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error: %v", err)
				}

				pvcSizes := []string{
					// original value from the yaml file (100MB)
					"100Mi",
					// half the size (50MB), is not stripe-size roundable
					"50Mi",
				}

				for _, pvcSize := range pvcSizes {
					err = validateThickPVC(f, pvc, pvcSize)
					if err != nil {
						e2elog.Failf("validating thick-provisioning failed: %v", err)
					}
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and Bind it to an app for mapped rbd image with options", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, map[string]string{
					"mapOptions":   "lock_on_read,queue_depth=1024",
					"unmapOptions": "force"}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("validate the functionality of controller", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = validateController(f, pvcPath, appPath, rbdExamplePath+"storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to validate controller with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0)
				err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

			})
			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, false, f)
				if err != nil {
					e2elog.Failf("failed to delete PVC when pool not found with error %v", err)
				}
			})
			// delete RBD provisioner secret
			err := deleteCephUser(f, keyringRBDProvisionerUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s with error %v", keyringRBDProvisionerUsername, err)
			}
			// delete RBD plugin secret
			err = deleteCephUser(f, keyringRBDNodePluginUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s with error %v", keyringRBDNodePluginUsername, err)
			}
		})
	})
})
