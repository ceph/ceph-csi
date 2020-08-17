package e2e

import (
	"fmt"
	"strings"
	"sync"

	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
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
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "--ignore-not-found=true", ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Logf("failed to delete provisioner rbac %s %v", rbdDirPath+rbdProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "delete", "--ignore-not-found=true", ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin rbac %s %v", rbdDirPath+rbdNodePluginRBAC, err)
	}

	createORDeleteRbdResouces("create")
}

func deleteRBDPlugin() {
	createORDeleteRbdResouces("delete")
}

func createORDeleteRbdResouces(action string) {
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisioner)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdProvisioner, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s rbd provisioner %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s provisioner rbac %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerPSP)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdProvisionerPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s provisioner psp %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePlugin)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdNodePlugin, err)
	}

	domainLabel := nodeRegionLabel + "," + nodeZoneLabel
	data = addTopologyDomainsToDSYaml(data, domainLabel)
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s nodeplugin %v", action, err)
		Fail(err.Error())
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s nodeplugin rbac %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginPSP)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", rbdDirPath+rbdNodePluginPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s nodeplugin psp %v", action, err)
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
			createNodeLabel(f, nodeRegionLabel, regionValue)
			createNodeLabel(f, nodeZoneLabel, zoneValue)
			if cephCSINamespace != defaultNs {
				err := createNamespace(c, cephCSINamespace)
				if err != nil {
					Fail(err.Error())
				}
			}
			deployRBDPlugin()
		}
		createConfigMap(rbdDirPath, f.ClientSet, f)
		createRBDStorageClass(f.ClientSet, f, nil, nil)
		createRBDSecret(f.ClientSet, f)
		deployVault(f.ClientSet, deployTimeout)
	})

	AfterEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-rbd", c)
			// log provisoner
			logsCSIPods("app=csi-rbdplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-rbdplugin", c)
		}

		deleteConfigMap(rbdDirPath)
		deleteResource(rbdExamplePath + "secret.yaml")
		deleteResource(rbdExamplePath + "storageclass.yaml")
		// deleteResource(rbdExamplePath + "snapshotclass.yaml")
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					Fail(err.Error())
				}
			}
		}
		deleteNodeLabel(c, nodeRegionLabel)
		deleteNodeLabel(c, nodeZoneLabel)
		// Remove the CSI labels that get added
		deleteNodeLabel(c, nodeCSIRegionLabel)
		deleteNodeLabel(c, nodeCSIZoneLabel)
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"
			rawPvcPath := rbdExamplePath + "raw-block-pvc.yaml"
			rawAppPath := rbdExamplePath + "raw-block-pod.yaml"
			pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
			pvcSmartClonePath := rbdExamplePath + "pvc-clone.yaml"
			appClonePath := rbdExamplePath + "pod-restore.yaml"
			appSmartClonePath := rbdExamplePath + "pod-clone.yaml"
			snapshotPath := rbdExamplePath + "snapshot.yaml"

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("checking nodeplugin deamonsets is running", func() {
				err := waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("create a PVC and Bind it to an app", func() {
				validatePVCAndAppBinding(pvcPath, appPath, f)
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			By("create a PVC and Bind it to an app with normal user", func() {
				validateNormalUserPVCAccess(pvcPath, f)
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			By("create a PVC and Bind it to an app with ext4 as the FS ", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"csi.storage.k8s.io/fstype": "ext4"})
				validatePVCAndAppBinding(pvcPath, appPath, f)
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, nil)
			})

			By("create a PVC and Bind it to an app with encrypted RBD volume", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"encrypted": "true"})
				validateEncryptedPVCAndAppBinding(pvcPath, appPath, "", f)
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, nil)
			})

			By("create a PVC and Bind it to an app with encrypted RBD volume with Vault KMS", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				createRBDStorageClass(f.ClientSet, f, nil, scOpts)
				validateEncryptedPVCAndAppBinding(pvcPath, appPath, "vault", f)
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, nil)
			})

			By("create a PVC clone and bind it to an app", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}
				// snapshot beta is only supported from v1.17+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "17") {
					var wg sync.WaitGroup
					totalCount := 10
					wg.Add(totalCount)
					createRBDSnapshotClass(f)
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						Fail(err.Error())
					}

					pvc.Namespace = f.UniqueName
					e2elog.Logf("The PVC template %+v", pvc)
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					images := listRBDImages(f)
					if len(images) != 1 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
						Fail("validate backend image failed")
					}
					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
					// create snapshot
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, s v1beta1.VolumeSnapshot) {
							s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
							err = createSnapshot(&s, deployTimeout)
							if err != nil {
								e2elog.Logf("failed to create snapshot %v", err)
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, snap)
					}
					wg.Wait()

					imageList := listRBDImages(f)
					// total images in cluster is 1 parent rbd image+ total snaps
					if len(imageList) != totalCount+1 {
						e2elog.Logf("backend images not matching kubernetes pvc,snap count,image count %d kubernetes resource count %d", len(imageList), totalCount+1)
						Fail("validate backend images failed")
					}

					pvcClone, err := loadPVC(pvcClonePath)
					if err != nil {
						Fail(err.Error())
					}
					appClone, err := loadApp(appClonePath)
					if err != nil {
						Fail(err.Error())
					}
					pvcClone.Namespace = f.UniqueName
					appClone.Namespace = f.UniqueName
					pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)

					// create multiple PVC from same snapshot
					wg.Add(totalCount)
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							err = createPVCAndApp(name, f, &p, &a, deployTimeout)
							if err != nil {
								e2elog.Logf("failed to create pvc and app %v", err)
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					imageList = listRBDImages(f)
					// total images in cluster is 1 parent rbd image+ total
					// snaps+ total clones
					totalCloneCount := totalCount + totalCount + 1
					if len(imageList) != totalCloneCount {
						e2elog.Logf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(imageList), totalCount+totalCount+1)
						Fail("validate backend images failed")
					}

					wg.Add(totalCount)
					// delete clone and app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = deletePVCAndApp(name, f, &p, &a)
							if err != nil {
								e2elog.Logf("failed to delete pvc and app %v", err)
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					imageList = listRBDImages(f)
					// total images in cluster is 1 parent rbd image+ total snaps
					if len(imageList) != totalCount+1 {
						e2elog.Logf("backend images not matching kubernetes pvc,snap count,image count %d kubernetes resource count %d", len(imageList), totalCount+1)
						Fail("validate backend images failed")
					}
					// create clones from different snapshosts and bind it to an
					// app
					wg.Add(totalCount)
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = createPVCAndApp(name, f, &p, &a, deployTimeout)
							if err != nil {
								e2elog.Logf("failed to create pvc and app %v", err)
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					imageList = listRBDImages(f)
					// total images in cluster is 1 parent rbd image+ total
					// snaps+ total clones
					totalCloneCount = totalCount + totalCount + 1
					if len(imageList) != totalCloneCount {
						e2elog.Logf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(imageList), totalCount+totalCount+1)
						Fail("validate backend images failed")
					}

					// delete parent pvc
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}

					imageList = listRBDImages(f)
					totalSnapCount := totalCount + totalCount
					// total images in cluster is total snaps+ total clones
					if len(imageList) != totalSnapCount {
						e2elog.Logf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(imageList), totalCount+totalCount)
						Fail("validate backend images failed")
					}
					wg.Add(totalCount)
					// delete snapshot
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, s v1beta1.VolumeSnapshot) {
							s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
							err = deleteSnapshot(&s, deployTimeout)
							if err != nil {
								e2elog.Logf("failed to delete snapshot %v", err)
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, snap)
					}
					wg.Wait()

					imageList = listRBDImages(f)
					if len(imageList) != totalCount {
						e2elog.Logf("backend images not matching kubernetes snap count,image count %d kubernetes resource count %d", len(imageList), totalCount)
						Fail("validate backend images failed")
					}
					wg.Add(totalCount)
					// delete clone and app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = deletePVCAndApp(name, f, &p, &a)
							if err != nil {
								e2elog.Logf("failed to delete pvc and app %v", err)
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()
					// validate created backend rbd images
					images = listRBDImages(f)
					if len(images) != 0 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
						Fail("validate backend image failed")
					}
				}
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}
				// pvc clone is only supported from v1.16+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "16") {
					var wg sync.WaitGroup
					totalCount := 10
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						Fail(err.Error())
					}

					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					images := listRBDImages(f)
					if len(images) != 1 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
						Fail("validate backend image failed")
					}

					pvcClone, err := loadPVC(pvcSmartClonePath)
					if err != nil {
						Fail(err.Error())
					}
					pvcClone.Spec.DataSource.Name = pvc.Name
					pvcClone.Namespace = f.UniqueName
					appClone, err := loadApp(appSmartClonePath)
					if err != nil {
						Fail(err.Error())
					}
					appClone.Namespace = f.UniqueName
					wg.Add(totalCount)
					// create clone and bind it to an app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							err = createPVCAndApp(name, f, &p, &a, deployTimeout)
							if err != nil {
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					images = listRBDImages(f)
					// total images in cluster is 1 parent rbd image+ total
					// temporary clone+ total clones
					totalCloneCount := totalCount + totalCount + 1
					if len(images) != totalCloneCount {
						e2elog.Logf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(images), totalCloneCount)
						Fail("validate backend images failed")
					}

					// delete parent pvc
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}

					images = listRBDImages(f)
					totalCloneCount = totalCount + totalCount
					// total images in cluster is total snaps+ total clones
					if len(images) != totalCloneCount {
						e2elog.Logf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(images), totalCloneCount)
						Fail("validate backend images failed")
					}
					wg.Add(totalCount)
					// delete clone and app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = deletePVCAndApp(name, f, &p, &a)
							if err != nil {
								Fail(err.Error())
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()
					images = listRBDImages(f)
					if len(images) != 0 {
						e2elog.Logf("backend images not matching kubernetes snap count,image count %d kubernetes resource count %d", len(images), 0)
						Fail("validate backend images failed")
					}
				}
			})

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
					err := createPVCAndApp(name, f, pvc, app, deployTimeout)
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
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
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
					createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"csi.storage.k8s.io/fstype": "xfs"})
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Logf("failed to resize filesystem PVC %v", err)
						Fail(err.Error())

					}
					// validate created backend rbd images
					images := listRBDImages(f)
					if len(images) != 0 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
						Fail("validate backend image failed")
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
					// validate created backend rbd images
					images := listRBDImages(f)
					if len(images) != 0 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
						Fail("validate backend image failed")
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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}

				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 1 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
					Fail("validate backend image failed")
				}
				// delete rbd nodeplugin pods
				err = deletePodWithLabel("app=csi-rbdplugin", cephCSINamespace, false)
				if err != nil {
					Fail(err.Error())
				}
				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images = listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			By("create PVC in storageClass with volumeNamePrefix", func() {
				volumeNamePrefix := "foo-bar-"
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, map[string]string{"volumeNamePrefix": volumeNamePrefix})

				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					Fail(err.Error())
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}

				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 1 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
					Fail("validate backend image failed")
				}
				// list RBD images and check if one of them has the same prefix
				foundIt := false
				for _, imgName := range listRBDImages(f) {
					fmt.Printf("Checking prefix on %s\n", imgName)
					if strings.HasPrefix(imgName, volumeNamePrefix) {
						foundIt = true
						break
					}
				}

				// clean up after ourselves
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images = listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, nil)

				if !foundIt {
					Fail(fmt.Sprintf("could not find image with prefix %s", volumeNamePrefix))
				}
			})

			By("validate RBD static FileSystem PVC", func() {
				err := validateRBDStaticPV(f, appPath, false)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			By("validate RBD static Block PVC", func() {
				err := validateRBDStaticPV(f, rawAppPath, true)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			By("validate mount options in app pod", func() {
				mountFlags := []string{"discard"}
				err := checkMountOptions(pvcPath, appPath, f, mountFlags)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			By("creating an app with a PVC, using a topology constrained StorageClass", func() {
				By("checking node has required CSI topology labels set", func() {
					checkNodeHasLabel(f.ClientSet, nodeCSIRegionLabel, regionValue)
					checkNodeHasLabel(f.ClientSet, nodeCSIZoneLabel, zoneValue)
				})

				By("creating a StorageClass with delayed binding mode and CSI topology parameter")
				deleteResource(rbdExamplePath + "storageclass.yaml")
				topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"domainSegments\":" +
					"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
					"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
				createRBDStorageClass(f.ClientSet, f,
					map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
					map[string]string{"topologyConstrainedPools": topologyConstraint})

				By("creating an app using a PV from the delayed binding mode StorageClass")
				pvc, app := createPVCAndAppBinding(pvcPath, appPath, f, 0)

				By("ensuring created PV has required node selector values populated")
				checkPVSelectorValuesForPVC(f, pvc)

				By("ensuring created PV has its image in the topology specific pool")
				err := checkPVCImageInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					Fail(err.Error())
				}

				By("ensuring created PV has its image journal in the topology specific pool")
				err = checkPVCImageJournalInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					Fail(err.Error())
				}

				By("ensuring created PV has its CSI journal in the CSI journal specific pool")
				err = checkPVCCSIJournalInPool(f, pvc, "replicapool")
				if err != nil {
					Fail(err.Error())
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}

				By("checking if data pool parameter is honored", func() {
					deleteResource(rbdExamplePath + "storageclass.yaml")
					topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"dataPool\":\"" + rbdTopologyDataPool +
						"\",\"domainSegments\":" +
						"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
						"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
					createRBDStorageClass(f.ClientSet, f,
						map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
						map[string]string{"topologyConstrainedPools": topologyConstraint})

					By("creating an app using a PV from the delayed binding mode StorageClass with a data pool")
					pvc, app = createPVCAndAppBinding(pvcPath, appPath, f, 0)

					By("ensuring created PV has its image in the topology specific pool")
					err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
					if err != nil {
						Fail(err.Error())
					}

					By("ensuring created image has the right data pool parameter set")
					err = checkPVCDataPoolForImageInPool(f, pvc, rbdTopologyPool, rbdTopologyDataPool)
					if err != nil {
						Fail(err.Error())
					}

					// cleanup and undo changes made by the test
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						Fail(err.Error())
					}
				})

				// cleanup and undo changes made by the test
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, nil)
			})

			// Mount pvc to pod with invalid mount option,expected that
			// mounting will fail
			By("Mount pvc to pod with invalid mount option", func() {
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, map[string]string{rbdmountOptions: "debug,invalidOption"}, nil)
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
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 1 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
					Fail("validate backend image failed")
				}
				// create an app and wait for 1 min for it to go to running state
				err = createApp(f.ClientSet, app, 1)
				if err == nil {
					Fail("application should not go to running state due to invalid mount option")
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}

				// validate created backend rbd images
				images = listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
				deleteResource(rbdExamplePath + "storageclass.yaml")
				createRBDStorageClass(f.ClientSet, f, nil, nil)
			})

			By("create ROX PVC clone and mount it to multiple pods", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}
				// snapshot beta is only supported from v1.17+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "17") {
					// create pvc and bind it to an app
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
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					images := listRBDImages(f)
					if len(images) != 1 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
						Fail("validate backend image failed")
					}
					// delete pod as we should not create snapshot for in-use pvc
					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}

					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					images = listRBDImages(f)
					// parent PVC + snapshot
					totalImages := 2
					if len(images) != totalImages {
						e2elog.Logf("backend image count %d expected image count %d", len(images), totalImages)
						Fail("validate backend image failed")
					}
					pvcClone, err := loadPVC(pvcClonePath)
					if err != nil {
						Fail(err.Error())
					}

					// create clone PVC as ROX
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
					err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					// parent pvc+ snapshot + clone
					totalImages = 3
					images = listRBDImages(f)
					if len(images) != totalImages {
						e2elog.Logf("backend image count %d expected image count %d", len(images), totalImages)
						Fail("validate backend image failed")
					}
					appClone, err := loadApp(appClonePath)
					if err != nil {
						Fail(err.Error())
					}

					totalCount := 2
					appClone.Namespace = f.UniqueName
					appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

					// create pvc and app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						label := map[string]string{
							"app": name,
						}
						appClone.Labels = label
						appClone.Name = name
						err = createApp(f.ClientSet, appClone, deployTimeout)
						if err != nil {
							Fail(err.Error())
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
							Fail(stdErr)
						}
					}

					// delete app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						appClone.Name = name
						err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
						if err != nil {
							Fail(err.Error())
						}
					}
					// delete pvc clone
					err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// delete snapshot
					err = deleteSnapshot(&snap, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// delete parent pvc
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					images = listRBDImages(f)
					if len(images) != 0 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
						Fail("validate backend image failed")
					}
				}
			})

			By("ensuring all operations will work within a rados namespace", func() {
				updateConfigMap := func(radosNS string) {
					radosNamespace = radosNS
					deleteConfigMap(rbdDirPath)
					createConfigMap(rbdDirPath, f.ClientSet, f)
					createRadosNamespace(f)

					// delete csi pods
					err := deletePodWithLabel("app in (ceph-csi-rbd, csi-rbdplugin, csi-rbdplugin-provisioner)",
						cephCSINamespace, false)
					if err != nil {
						Fail(err.Error())
					}
					// wait for csi pods to come up
					err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					err = waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
				}

				updateConfigMap("e2e-ns")

				// Create a PVC and Bind it to an app within the namesapce
				validatePVCAndAppBinding(pvcPath, appPath, f)

				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}

				// Resize Block PVC and check Device size within the namespace
				// Block PVC resize is supported in kubernetes 1.16+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "16") {
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Logf("failed to resize block PVC %v", err)
						Fail(err.Error())
					}
				}

				// Create a PVC clone and bind it to an app within the namespace
				// snapshot beta is only supported from v1.17+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "17") {
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						Fail(err.Error())
					}

					pvc.Namespace = f.UniqueName
					e2elog.Logf("The PVC  template %+v", pvc)
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate created backend rbd images
					images := listRBDImages(f)
					if len(images) != 1 {
						e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
						Fail("validate backend image failed")
					}
					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					expectedImages := len(images) + 1
					images = listRBDImages(f)
					if len(images) != expectedImages {
						e2elog.Logf("backend images not matching kubernetes resource count,image count %d kubernetes resource count %d", len(images), 2)
						Fail("validate backend images failed")
					}

					validatePVCAndAppBinding(pvcClonePath, appClonePath, f)

					err = deleteSnapshot(&snap, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
				}

				updateConfigMap("")
			})

			By("Mount pvc as readonly in pod", func() {
				// create pvc and bind it to an app
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
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				app.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 1 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
					Fail("validate backend image failed")
				}

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr := execCommandInPodAndAllowFail(f, fmt.Sprintf("echo 'Hello World' > %s", filePath), app.Namespace, &opt)
				readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
				if !strings.Contains(stdErr, readOnlyErr) {
					Fail(stdErr)
				}

				// delete pvc and app
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}
				// validate created backend rbd images
				images = listRBDImages(f)
				if len(images) != 0 {
					e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
					Fail("validate backend image failed")
				}
			})

			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and Delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, false, f)
				if err != nil {
					Fail(err.Error())
				}
			})
		})
	})
})
