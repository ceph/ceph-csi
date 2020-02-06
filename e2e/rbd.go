package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo" // nolint

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
	rbdDirPath         = "../deploy/rbd/kubernetes/"
	rbdExamplePath     = "../examples/rbd/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	namespace          = "default"
)

func deployRBDPlugin() {
	// delete objects deployed by rook
	framework.RunKubectlOrDie("delete", "--ignore-not-found=true", "-f", rbdDirPath+rbdProvisionerRBAC)
	framework.RunKubectlOrDie("delete", "--ignore-not-found=true", "-f", rbdDirPath+rbdNodePluginRBAC)
	// deploy provisioner
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdProvisioner)
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdProvisionerRBAC)
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdProvisionerPSP)
	// deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdNodePlugin)
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdNodePluginRBAC)
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdNodePluginPSP)
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
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdProvisionerPSP)
	if err != nil {
		e2elog.Logf("failed to delete provisioner psp %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdNodePlugin)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin rbac %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", rbdDirPath+rbdNodePluginPSP)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin psp %v", err)
	}
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("rbd")
	var c clientset.Interface
	// deploy RBD CSI
	BeforeEach(func() {
		c = f.ClientSet
		createConfigMap(rbdDirPath, f.ClientSet, f)
		deployRBDPlugin()
		createRBDStorageClass(f.ClientSet, f, make(map[string]string))
		createRBDSecret(f.ClientSet, f)
		deployVault(f.ClientSet, deployTimeout)
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			// log provisoner
			logsCSIPods("app=csi-rbdplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-rbdplugin", c)
		}
		deleteRBDPlugin()
		deleteConfigMap(rbdDirPath)
		deleteResource(rbdExamplePath + "secret.yaml")
		deleteResource(rbdExamplePath + "storageclass.yaml")
		// deleteResource(rbdExamplePath + "snapshotclass.yaml")
		deleteVault()
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"
			rawPvcPath := rbdExamplePath + "raw-block-pvc.yaml"
			rawAppPath := rbdExamplePath + "raw-block-pod.yaml"
			// pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
			// appClonePath := rbdExamplePath + "pod-restore.yaml"
			// snapshotPath := rbdExamplePath + "snapshot.yaml"

			By("checking provisioner deployment is running")
			var err error
			err = waitForDeploymentComplete(rbdDeploymentName, namespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets(rbdDaemonsetName, namespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("create a PVC and Bind it to an app", func() {
				validatePVCAndAppBinding(pvcPath, appPath, f)
			})

			By("create a PVC and Bind it to an app with normal user", func() {
				validateNormalUserPVCAccess(pvcPath, f)
			})

			By("create a PVC and Bind it to an app with ext4 as the FS ", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, map[string]string{"csi.storage.k8s.io/fstype": "ext4"})
				validatePVCAndAppBinding(pvcPath, appPath, f)
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, make(map[string]string))
			})

			By("create a PVC and Bind it to an app with encrypted RBD volume", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, map[string]string{"encrypted": "true"})
				validateEncryptedPVCAndAppBinding(pvcPath, appPath, "", f)
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, make(map[string]string))
			})

			By("create a PVC and Bind it to an app with encrypted RBD volume with Vault KMS", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				createRBDStorageClass(f.ClientSet, f, scOpts)
				validateEncryptedPVCAndAppBinding(pvcPath, appPath, "vault", f)
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, make(map[string]string))
			})

			// skipping snapshot testing

			// By("create a PVC clone and Bind it to an app", func() {
			// 	createRBDSnapshotClass(f)
			// 	pvc, err := loadPVC(pvcPath)
			// 	if err != nil {
			// 		Fail(err.Error())
			// 	}

			// 	pvc.Namespace = f.UniqueName
			// 	e2elog.Logf("The PVC  template %+v", pvc)
			// 	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
			// 	if err != nil {
			// 		Fail(err.Error())
			// 	}
			// 	// validate created backend rbd images
			// 	images := listRBDImages(f)
			// 	if len(images) != 1 {
			// 		e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
			// 		Fail("validate backend image failed")
			// 	}
			// 	snap := getSnapshot(snapshotPath)
			// 	snap.Namespace = f.UniqueName
			// 	snap.Spec.Source.Name = pvc.Name
			// 	snap.Spec.Source.Kind = "PersistentVolumeClaim"
			// 	err = createSnapshot(&snap, deployTimeout)
			// 	if err != nil {
			// 		Fail(err.Error())
			// 	}
			// 	pool := "replicapool"
			// 	snapList, err := listSnapshots(f, pool, images[0])
			// 	if err != nil {
			// 		Fail(err.Error())
			// 	}
			// 	if len(snapList) != 1 {
			// 		e2elog.Logf("backend snapshot not matching kube snap count,snap count = % kube snap count %d", len(snapList), 1)
			// 		Fail("validate backend snapshot failed")
			// 	}

			// 	validatePVCAndAppBinding(pvcClonePath, appClonePath, f)

			// 	err = deleteSnapshot(&snap, deployTimeout)
			// 	if err != nil {
			// 		Fail(err.Error())
			// 	}
			// 	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
			// 	if err != nil {
			// 		Fail(err.Error())
			// 	}
			// })

			By("create a block type PVC and Bind it to an app", func() {
				validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
			})

			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
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
				// create pvc and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := createPVCAndApp(name, f, pvc, app)
					if err != nil {
						Fail(err.Error())
					}

				}
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != totalCount {
					e2elog.Logf("backend image creation not matching pvc count, image count = % pvc count %d", len(images), totalCount)
					Fail("validate multiple pvc failed")
				}

				// delete pvc and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						Fail(err.Error())
					}

				}

				// validate created backend rbd images
				images = listRBDImages(f)
				if len(images) > 0 {
					e2elog.Logf("left out rbd backend images count %d", len(images))
					Fail("validate multiple pvc failed")
				}
			})

			By("check data persist after recreating pod with same pvc", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("Resize Filesystem PVC and check application directory size", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}

				// Resize 0.3.0 is only supported from v1.15+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "15") {
					err := resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Logf("failed to resize filesystem PVC %v", err)
						Fail(err.Error())
					}

					deleteResource(rbdExamplePath + "storageclass.yaml")
					createRBDStorageClass(f.ClientSet, f, map[string]string{"csi.storage.k8s.io/fstype": "xfs"})
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Logf("failed to resize filesystem PVC %v", err)
						Fail(err.Error())

					}
				}
			})

			By("Resize Block PVC and check Device size", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}

				// Block PVC resize is supported in kubernetes 1.16+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "16") {
					err := resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Logf("failed to resize block PVC %v", err)
						Fail(err.Error())
					}
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
