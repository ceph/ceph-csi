package e2e

import (
	"fmt"
	"strings"

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
)

func deployCephfsPlugin() {
	// delete objects deployed by rook

	data, err := replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "--ignore-not-found=true", ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Logf("failed to delete provisioner rbac %s %v", cephfsDirPath+cephfsProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, "delete", "--ignore-not-found=true", ns, "-f", "-")

	if err != nil {
		e2elog.Logf("failed to delete nodeplugin rbac %s %v", cephfsDirPath+cephfsNodePluginRBAC, err)
	}

	createORDeleteCephfsResouces("create")
}

func deleteCephfsPlugin() {
	createORDeleteCephfsResouces("delete")
}

func createORDeleteCephfsResouces(action string) {
	data, err := replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisioner)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsProvisioner, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s cephfs provisioner %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsProvisionerRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s cephfs provisioner rbac %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsProvisionerPSP)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsProvisionerPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s cephfs provisioner psp %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePlugin)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsNodePlugin, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s cephfs nodeplugin %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsNodePluginRBAC, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s cephfs nodeplugin rbac %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(cephfsDirPath + cephfsNodePluginPSP)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", cephfsDirPath+cephfsNodePluginPSP, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Logf("failed to %s cephfs nodeplugin psp %v", action, err)
	}
}

var _ = Describe("cephfs", func() {
	f := framework.NewDefaultFramework("cephfs")
	var c clientset.Interface
	// deploy cephfs CSI
	BeforeEach(func() {
		if !testCephFS || upgradeTesting {
			Skip("Skipping CephFS E2E")
		}
		c = f.ClientSet
		if deployCephFS {
			if cephCSINamespace != defaultNs {
				err := createNamespace(c, cephCSINamespace)
				if err != nil {
					Fail(err.Error())
				}
			}
			deployCephfsPlugin()
		}
		createConfigMap(cephfsDirPath, f.ClientSet, f)
		createCephfsSecret(f.ClientSet, f)
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
		deleteConfigMap(cephfsDirPath)
		deleteResource(cephfsExamplePath + "secret.yaml")
		deleteResource(cephfsExamplePath + "storageclass.yaml")
		if deployCephFS {
			deleteCephfsPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					Fail(err.Error())
				}
			}
		}
	})

	Context("Test cephfs CSI", func() {
		It("Test cephfs CSI", func() {
			pvcPath := cephfsExamplePath + "pvc.yaml"
			appPath := cephfsExamplePath + "pod.yaml"

			By("checking provisioner deployment is running")
			var err error
			err = waitForDeploymentComplete(cephfsDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets(cephfsDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("check static PVC", func() {
				scPath := cephfsExamplePath + "secret.yaml"
				err := validateCephFsStaticPV(f, appPath, scPath)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("create a storage class with pool and a PVC then Bind it to an app", func() {
				createCephfsStorageClass(f.ClientSet, f, true, "")
				validatePVCAndAppBinding(pvcPath, appPath, f)
				deleteResource(cephfsExamplePath + "storageclass.yaml")
			})

			createCephfsStorageClass(f.ClientSet, f, false, "")

			By("create and delete a PVC", func() {
				By("create a PVC and Bind it to an app", func() {
					validatePVCAndAppBinding(pvcPath, appPath, f)

				})

				By("create a PVC and Bind it to an app with normal user", func() {
					validateNormalUserPVCAccess(pvcPath, f)
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
					// TODO add cephfs backend validation

					// delete pvc and app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						err := deletePVCAndApp(name, f, pvc, app)
						if err != nil {
							Fail(err.Error())
						}

					}
				})

				By("check data persist after recreating pod with same pvc", func() {
					err := checkDataPersist(pvcPath, appPath, f)
					if err != nil {
						Fail(err.Error())
					}
				})

				By("creating a PVC, deleting backing subvolume, and checking successful PV deletion", func() {
					pvc, err := loadPVC(pvcPath)
					if pvc == nil {
						Fail(err.Error())
					}
					pvc.Namespace = f.UniqueName

					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}

					err = deleteBackingCephFSVolume(f, pvc)
					if err != nil {
						Fail(err.Error())
					}

					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
				})

				By("validate multiple subvolumegroup creation", func() {
					deleteResource(cephfsExamplePath + "storageclass.yaml")
					// re-define configmap with information of multiple clusters.
					subvolgrpInfo := map[string]string{
						"clusterID-1": "subvolgrp1",
						"clusterID-2": "subvolgrp2",
					}
					createCustomConfigMap(f.ClientSet, cephfsDirPath, subvolgrpInfo)
					createCephfsStorageClass(f.ClientSet, f, false, "clusterID-1")
					validatePVCAndAppBinding(pvcPath, appPath, f)
					deleteResource(cephfsExamplePath + "storageclass.yaml")
					// verify subvolumegroup creation.
					err := validateSubvolumegroup(f, "subvolgrp1")
					if err != nil {
						Fail(err.Error())
					}

					// create resources and verify subvolume group creation
					// for the second cluster.
					createCephfsStorageClass(f.ClientSet, f, false, "clusterID-2")
					validatePVCAndAppBinding(pvcPath, appPath, f)
					deleteResource(cephfsExamplePath + "storageclass.yaml")
					err = validateSubvolumegroup(f, "subvolgrp2")
					if err != nil {
						Fail(err.Error())
					}
					deleteConfigMap(cephfsDirPath)
				})

				createConfigMap(cephfsDirPath, f.ClientSet, f)
				createCephfsStorageClass(f.ClientSet, f, false, "")

				By("Resize PVC and check application directory size", func() {
					v, err := f.ClientSet.Discovery().ServerVersion()
					if err != nil {
						e2elog.Logf("failed to get server version with error %v", err)
						Fail(err.Error())
					}

					// Resize 0.3.0 is only supported from v1.15+
					if v.Major > "1" || (v.Major == "1" && v.Minor >= "15") {
						err := resizePVCAndValidateSize(pvcPath, appPath, f)
						if err != nil {
							e2elog.Logf("failed to resize PVC %v", err)
							Fail(err.Error())
						}
					}

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
				})

				By("Create ROX PVC and Bind it to an app", func() {
					// create pvc and bind it to an app
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						Fail(err.Error())
					}

					pvc.Namespace = f.UniqueName
					pvc.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
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
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						Fail(err.Error())
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
				})
				// Make sure this should be last testcase in this file, because
				// it deletes pool
				By("Create a PVC and Delete PVC when backend pool deleted", func() {
					err := pvcDeleteWhenPoolNotFound(pvcPath, true, f)
					if err != nil {
						Fail(err.Error())
					}
				})

			})

		})
	})

})
