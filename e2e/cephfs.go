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
	// deploy provisioner
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsProvisioner)
	framework.RunKubectlOrDie("apply", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	// deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsNodePlugin)
	framework.RunKubectlOrDie("apply", "-f", cephfsDirPath+cephfsNodePluginRBAC)
}

var _ = Describe("cephfs", func() {
	f := framework.NewDefaultFramework("cephfs")
	// deploy cephfs CSI
	BeforeEach(func() {
		updateCephfsDirPath(f.ClientSet)
		createFileSystem(f.ClientSet)
		createConfigMap(cephfsDirPath, f.ClientSet, f)
		deployCephfsPlugin()
		createCephfsStorageClass(f.ClientSet, f)
		createCephfsSecret(f.ClientSet, f)
	})

	AfterEach(func() {
		cephfsFiles := getFilesinDirectory(cephfsDirPath)
		for _, file := range cephfsFiles {
			res, err := framework.RunKubectl("delete", "-f", cephfsDirPath+file.Name())
			if err != nil {
				e2elog.Logf("failed to delete resource in %s with err %v", res, err)
			}
		}
		deleteResource(cephfsExamplePath + "secret.yaml")
		deleteResource(cephfsExamplePath + "storageclass.yaml")
		deleteFileSystem()
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

			})

		})
	})

})
