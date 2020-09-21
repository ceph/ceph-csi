package e2e

import (
	"fmt"
	"strings"
	"sync"

	vs "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	cephfsProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephfsProvisionerPSP  = "csi-provisioner-psp.yaml"
	cephfsNodePlugin      = "csi-cephfsplugin.yaml"
	cephfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	cephfsNodePluginPSP   = "csi-nodeplugin-psp.yaml"
	cephfsDeploymentName  = "csi-cephfsplugin-provisioner"
	cephfsDeamonSetName   = "csi-cephfsplugin"
	cephfsDirPath         = "../deploy/cephfs/kubernetes/"
	cephfsExamplePath     = "../examples/cephfs/"
	subvolumegroup        = "e2e"
	fileSystemName        = "myfs"
)

func deployCephfsPlugin() {
	// delete objects deployed by rook

	data, err := replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "--ignore-not-found=true", ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Failf("failed to delete provisioner rbac %s with error %v", cephfsDirPath+cephfsProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "delete", "--ignore-not-found=true", ns, "-f", "-")

	if err != nil {
		e2elog.Failf("failed to delete nodeplugin rbac %s with error %v", cephfsDirPath+cephfsNodePluginRBAC, err)
	}

	createORDeleteCephfsResouces("create")
}

func deleteCephfsPlugin() {
	createORDeleteCephfsResouces("delete")
}

func createORDeleteCephfsResouces(action string) {
	data, err := replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisioner)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsProvisioner, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s CephFS provisioner with error %v", action, err)
	}
	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisionerRBAC)

	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s CephFS provisioner rbac with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisionerPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsProvisionerPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s CephFS provisioner psp with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePlugin)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsNodePlugin, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s CephFS nodeplugin with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s CephFS nodeplugin rbac with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePluginPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", cephfsDirPath+cephfsNodePluginPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s CephFS nodeplugin psp with error %v", action, err)
	}
}

func validateSubvolumeCount(f *framework.Framework, count int, fileSystemName, subvolumegroup string) {
	subVol, err := listCephFSSubVolumes(f, fileSystemName, subvolumegroup)
	if err != nil {
		e2elog.Failf("failed to list CephFS subvolumes with error %v", err)
	}
	if len(subVol) != count {
		e2elog.Failf("subvolumes [%v]. subvolume count %d not matching expected count %v", subVol, len(subVol), count)
	}
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
					e2elog.Failf("failed to create namespace %s with error %v", cephCSINamespace, err)
				}
			}
			deployCephfsPlugin()
		}
		err := createConfigMap(cephfsDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap with error %v", err)
		}
		err = createCephfsSecret(f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create secret with error %v", err)
		}
	})

	AfterEach(func() {
		if !testCephFS || upgradeTesting {
			Skip("Skipping CephFS E2E")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-cephfs", c)
			// log provisoner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)
		}
		err := deleteConfigMap(cephfsDirPath)
		if err != nil {
			e2elog.Failf("failed to delete configmap with error %v", err)
		}
		err = deleteResource(cephfsExamplePath + "secret.yaml")
		if err != nil {
			e2elog.Failf("failed to delete secret with error %v", err)
		}
		err = deleteResource(cephfsExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass with error %v", err)
		}
		if deployCephFS {
			deleteCephfsPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to delete namespace %s with error %v", cephCSINamespace, err)
				}
			}
		}
	})

	Context("Test CephFS CSI", func() {
		It("Test CephFS CSI", func() {
			pvcPath := cephfsExamplePath + "pvc.yaml"
			appPath := cephfsExamplePath + "pod.yaml"
			pvcClonePath := cephfsExamplePath + "pvc-restore.yaml"
			pvcSmartClonePath := cephfsExamplePath + "pvc-clone.yaml"
			appClonePath := cephfsExamplePath + "pod-restore.yaml"
			appSmartClonePath := cephfsExamplePath + "pod-clone.yaml"
			snapshotPath := cephfsExamplePath + "snapshot.yaml"

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(cephfsDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment %s with error %v", cephfsDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(cephfsDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset %s with error %v", cephfsDeamonSetName, err)
				}
			})

			By("check static PVC", func() {
				scPath := cephfsExamplePath + "secret.yaml"
				err := validateCephFsStaticPV(f, appPath, scPath)
				if err != nil {
					e2elog.Failf("failed to validate CephFS static pv with error %v", err)
				}
			})

			By("create a storageclass with pool and a PVC then bind it to an app", func() {
				err := createCephfsStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding with error %v", err)
				}
				err = deleteResource(cephfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass with error %v", err)
				}
			})

			By("create a storageclass with ceph-fuse and a PVC then bind it to an app", func() {
				params := map[string]string{
					"mounter": "fuse",
				}
				err := createCephfsStorageClass(f.ClientSet, f, true, params)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding with error %v", err)
				}
				err = deleteResource(cephfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass with error %v", err)
				}
			})

			By("create a storageclass with ceph-fuse, mount-options and a PVC then bind it to an app", func() {
				params := map[string]string{
					"mounter":          "fuse",
					"fuseMountOptions": "debug",
				}
				err := createCephfsStorageClass(f.ClientSet, f, true, params)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding with error %v", err)
				}
				err = deleteResource(cephfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app", func() {
				err := createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application  binding with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate normal user CephFS pvc and application binding with error %v", err)
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
					err = createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC or application with error %v", err)
					}

				}

				validateSubvolumeCount(f, totalCount, fileSystemName, subvolumegroup)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC or application with error %v", err)
					}

				}
				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to check data persist in pvc with error %v", err)
				}
			})

			By("create PVC, delete backing subvolume and check pv deletion", func() {
				pvc, err := loadPVC(pvcPath)
				if pvc == nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}

				err = deleteBackingCephFSVolume(f, pvc)
				if err != nil {
					e2elog.Failf("failed to delete CephFS subvolume with error %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC with error %v", err)
				}
			})

			By("validate multiple subvolumegroup creation", func() {
				err := deleteResource(cephfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				// re-define configmap with information of multiple clusters.
				subvolgrpInfo := map[string]string{
					"clusterID-1": "subvolgrp1",
					"clusterID-2": "subvolgrp2",
				}
				err = createCustomConfigMap(f.ClientSet, cephfsDirPath, subvolgrpInfo)
				if err != nil {
					e2elog.Failf("failed to create configmap with error %v", err)
				}
				params := map[string]string{
					"clusterID": "clusterID-1",
				}
				err = createCephfsStorageClass(f.ClientSet, f, false, params)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application with error %v", err)
				}
				err = deleteResource(cephfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				// verify subvolumegroup creation.
				err = validateSubvolumegroup(f, "subvolgrp1")
				if err != nil {
					e2elog.Failf("failed to validate subvolume group with error %v", err)
				}

				// create resources and verify subvolume group creation
				// for the second cluster.
				params["clusterID"] = "clusterID-2"
				err = createCephfsStorageClass(f.ClientSet, f, false, params)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application with error %v", err)
				}
				err = deleteResource(cephfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = validateSubvolumegroup(f, "subvolgrp2")
				if err != nil {
					e2elog.Failf("failed to validate subvolume group with error %v", err)
				}
				err = deleteConfigMap(cephfsDirPath)
				if err != nil {
					e2elog.Failf("failed to delete configmap with error %v", err)
				}
				err = createConfigMap(cephfsDirPath, f.ClientSet, f)
				if err != nil {
					e2elog.Failf("failed to create configmap with error %v", err)
				}
				err = createCephfsStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("Resize PVC and check application directory size", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Failf("failed to get server version with error with error %v", err)
				}

				// Resize 0.3.0 is only supported from v1.15+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "15") {
					err := resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize PVC with error %v", err)
					}
				}
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
					e2elog.Failf("failed to create PVC or application with error %v", err)
				}

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
					e2elog.Failf("failed to delete PVC or application with error %v", err)
				}
			})

			By("create a PVC clone and bind it to an app", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Failf("failed to get server version with error with error %v", err)
				}
				// snapshot beta is only supported from v1.17+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "17") {
					var wg sync.WaitGroup
					totalCount := 3
					// totalSubvolumes represents the subvolumes in backend
					// always totalCount+parentPVC
					totalSubvolumes := totalCount + 1
					wg.Add(totalCount)
					err = createCephFSSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to delete CephFS storageclass with error %v", err)
					}
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}

					app, err := loadApp(appPath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}

					app.Namespace = f.UniqueName
					app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
					wErr := writeDataInPod(app, f)
					if wErr != nil {
						e2elog.Failf("failed to  write data  with error %v", wErr)
					}

					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
					// create snapshot
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, s vs.VolumeSnapshot) {
							s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
							err = createSnapshot(&s, deployTimeout)
							if err != nil {
								e2elog.Failf("failed to create snapshot with error %v", err)
							}
							w.Done()
						}(&wg, i, snap)
					}
					wg.Wait()

					pvcClone, err := loadPVC(pvcClonePath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}
					appClone, err := loadApp(appClonePath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
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
								e2elog.Failf("failed to create PVC and app with error %v", err)
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)

					wg.Add(totalCount)
					// delete clone and app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = deletePVCAndApp(name, f, &p, &a)
							if err != nil {
								e2elog.Failf("failed to delete PVC and app with error %v", err)
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					parentPVCCount := totalSubvolumes - totalCount
					validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
					// create clones from different snapshosts and bind it to an
					// app
					wg.Add(totalCount)
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = createPVCAndApp(name, f, &p, &a, deployTimeout)
							if err != nil {
								e2elog.Failf("failed to create PVC and app with error %v", err)
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)

					wg.Add(totalCount)
					// delete snapshot
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, s vs.VolumeSnapshot) {
							s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
							err = deleteSnapshot(&s, deployTimeout)
							if err != nil {
								e2elog.Failf("failed to delete snapshot with error %v", err)
							}
							w.Done()
						}(&wg, i, snap)
					}
					wg.Wait()

					wg.Add(totalCount)
					// delete clone and app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = deletePVCAndApp(name, f, &p, &a)
							if err != nil {
								e2elog.Failf("failed to delete PVC and app with error %v", err)
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
					// delete parent pvc
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC or application with error %v", err)
					}

					validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				}
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Failf("failed to get server version with error with error %v", err)
				}
				// pvc clone is only supported from v1.16+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "16") {
					var wg sync.WaitGroup
					totalCount := 3
					// totalSubvolumes represents the subvolumes in backend
					// always totalCount+parentPVC
					totalSubvolumes := totalCount + 1
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}
					app, err := loadApp(appPath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					app.Namespace = f.UniqueName
					app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
					wErr := writeDataInPod(app, f)
					if wErr != nil {
						e2elog.Failf("failed to write data from application %v", wErr)
					}

					pvcClone, err := loadPVC(pvcSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}
					pvcClone.Spec.DataSource.Name = pvc.Name
					pvcClone.Namespace = f.UniqueName
					appClone, err := loadApp(appSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					appClone.Namespace = f.UniqueName
					wg.Add(totalCount)
					// create clone and bind it to an app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							err = createPVCAndApp(name, f, &p, &a, deployTimeout)
							if err != nil {
								e2elog.Failf("failed to create PVC or application with error %v", err)
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)

					// delete parent pvc
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC or application with error %v", err)
					}

					wg.Add(totalCount)
					// delete clone and app
					for i := 0; i < totalCount; i++ {
						go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
							name := fmt.Sprintf("%s%d", f.UniqueName, n)
							p.Spec.DataSource.Name = name
							err = deletePVCAndApp(name, f, &p, &a)
							if err != nil {
								e2elog.Failf("failed to delete PVC or application with error %v", err)
							}
							w.Done()
						}(&wg, i, *pvcClone, *appClone)
					}
					wg.Wait()

					validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				}
			})

			By("Create ROX PVC and bind it to an app", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}

				app.Namespace = f.UniqueName
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				err = createPVCAndApp("", f, pvcClone, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC or application with error %v", err)
				}

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr := execCommandInPodAndAllowFail(f, fmt.Sprintf("echo 'Hello World' > %s", filePath), app.Namespace, &opt)
				readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
				if !strings.Contains(stdErr, readOnlyErr) {
					e2elog.Failf(stdErr)
				}

				// delete cloned ROX pvc and app
				err = deletePVCAndApp("", f, pvcClone, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application with error %v", err)
				}

				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC with error %v", err)
				}
			})
			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, true, f)
				if err != nil {
					e2elog.Failf("failed to delete PVC with error %v", err)
				}
			})

		})
	})

})
