package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo" // nolint

	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	cephfsProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephfsNodePlugin      = "csi-cephfsplugin.yaml"
	cephfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	cephfsDeploymentName  = "csi-cephfsplugin-provisioner"
	cephfsDeamonSetName   = "csi-cephfsplugin"
	cephfsDirPath         = "../deploy/cephfs/kubernetes/"
	cephfsExamplePath     = "../examples/cephfs/"
)

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
		createFileSystem(f.ClientSet)
		createConfigMap(f.ClientSet, f)
		deployCephfsPlugin()
		createCephfsStorageClass(f.ClientSet, f)
		createCephfsSecret(f.ClientSet, f)
	})

	AfterEach(func() {
		cephfsFiles := getFilesinDirectory(cephfsDirPath)
		for _, file := range cephfsFiles {
			res, err := framework.RunKubectl("delete", "-f", cephfsDirPath+file.Name())
			if err != nil {
				framework.Logf("failed to delete resource in %s with err %v", res, err)
			}
		}
		deleteSecret(cephfsExamplePath + "secret.yaml")
		deleteStorageClass(cephfsExamplePath + "storageclass.yaml")
		deleteFileSystem()
	})

	Context("Test cephfs CSI", func() {
		It("Test cephfs CSI", func() {
			pvcPath := cephfsExamplePath + "pvc.yaml"
			appPath := cephfsExamplePath + "pod.yaml"
			By("checking provisioner deployment is completed")
			err := waitForDeploymentComplete(cephfsDeploymentName, namespace, f.ClientSet, deployTimeout)
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

				By("create/delete multiple PVC and App", func() {
					totalCount := 2
					pvc := loadPVC(pvcPath)
					pvc.Namespace = f.UniqueName
					app := loadApp(appPath)
					app.Namespace = f.UniqueName
					//create pvc and app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						err := createPVCAndApp(name, f, pvc, app)
						if err != nil {
							Fail(err.Error())
						}

					}
					//TODO add cephfs backend validation

					//delete pvc and app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						err := deletePVCAndApp(name, f, pvc, app)
						if err != nil {
							Fail(err.Error())
						}

					}
				})
			})
		})
	})

})
