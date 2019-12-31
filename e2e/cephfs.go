package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo" // nolint

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	cephfsProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephfsNodePlugin      = "csi-cephfsplugin.yaml"
	cephfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	cephfsDeploymentName  = "csi-cephfsplugin-provisioner"
	cephfsDeamonSetName   = "csi-cephfsplugin"
	cephfsDirPath         = "../deploy/cephfs/kubernetes"
	cephfsExamplePath     = "../examples/cephfs/"
)

func updateCephfsDirPath(c clientset.Interface) {
	version := getKubeVersionToDeploy(c)
	cephfsDirPath = fmt.Sprintf("%s/%s/", cephfsDirPath, version)
}

func deployCephfsPlugin() {
	// delete objects deployed by rook
	framework.RunKubectlOrDie("delete", "--ignore-not-found=true", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	framework.RunKubectlOrDie("delete", "--ignore-not-found=true", "-f", cephfsDirPath+cephfsNodePluginRBAC)
	// deploy provisioner
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsProvisioner)
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	// deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsNodePlugin)
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsNodePluginRBAC)
}

func deleteCephfsPlugin() {
	_, err := framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsProvisioner)
	if err != nil {
		e2elog.Logf("failed to delete cephfs provisioner %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to delete cephfs provisioner rbac %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsNodePlugin)
	if err != nil {
		e2elog.Logf("failed to delete cephfs nodeplugin %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", cephfsDirPath+cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to delete cephfs nodeplugin rbac %v", err)
	}
}

var _ = Describe("cephfs", func() {
	f := framework.NewDefaultFramework("cephfs")
	var c clientset.Interface
	// deploy cephfs CSI
	BeforeEach(func() {
		c = f.ClientSet
		updateCephfsDirPath(f.ClientSet)
		createConfigMap(cephfsDirPath, f.ClientSet, f)
		deployCephfsPlugin()
		createCephfsSecret(f.ClientSet, f)
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			// log provisoner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)
		}
		deleteCephfsPlugin()
		deleteConfigMap(cephfsDirPath)
		deleteResource(cephfsExamplePath + "secret.yaml")
		deleteResource(cephfsExamplePath + "storageclass.yaml")
	})

	Context("Test cephfs CSI", func() {
		It("Test cephfs CSI", func() {
			pvcPath := cephfsExamplePath + "pvc.yaml"
			appPath := cephfsExamplePath + "pod.yaml"

			By("checking provisioner statefulset/deployment is running")
			timeout := time.Duration(deployTimeout) * time.Minute
			var err error
			sts := deployProvAsSTS(f.ClientSet)
			if sts {
				err = framework.WaitForStatefulSetReplicasReady(cephfsDeploymentName, namespace, f.ClientSet, 1*time.Second, timeout)
			} else {
				err = waitForDeploymentComplete(cephfsDeploymentName, namespace, f.ClientSet, deployTimeout)
			}
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets(cephfsDeamonSetName, namespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("create a storage class with pool and a PVC then Bind it to an app", func() {
				createCephfsStorageClass(f.ClientSet, f, true)
				validatePVCAndAppBinding(pvcPath, appPath, f)
				deleteResource(cephfsExamplePath + "storageclass.yaml")
			})

			createCephfsStorageClass(f.ClientSet, f, false)

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
						err := createPVCAndApp(name, f, pvc, app)
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

			})

		})
	})

})
