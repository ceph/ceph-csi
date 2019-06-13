package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo" // nolint

	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	rbdProvisioner     = "csi-rbdplugin-provisioner.yaml"
	rbdProvisionerRBAC = "csi-provisioner-rbac.yaml"
	rbdNodePlugin      = "csi-rbdplugin.yaml"
	rbdNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	rbdConfigMap       = "csi-config-map.yaml"
	rbdDirPath         = "../deploy/rbd/kubernetes/"
	rbdExamplePath     = "../examples/rbd/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	namespace          = "default"
)

func deployRBDPlugin() {
	// deploy provisioner
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdProvisioner)
	framework.RunKubectlOrDie("apply", "-f", rbdDirPath+rbdProvisionerRBAC)
	// deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", rbdDirPath+rbdNodePlugin)
	framework.RunKubectlOrDie("apply", "-f", rbdDirPath+rbdNodePluginRBAC)
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("rbd")
	// deploy RBD CSI
	BeforeEach(func() {
		createRBDPool()
		createConfigMap(f.ClientSet, f)
		deployRBDPlugin()
		createRBDStorageClass(f.ClientSet, f)
		createRBDSecret(f.ClientSet, f)
	})

	AfterEach(func() {
		rbdFiles := getFilesinDirectory(rbdDirPath)
		for _, file := range rbdFiles {
			res, err := framework.RunKubectl("delete", "-f", rbdDirPath+file.Name())
			if err != nil {
				framework.Logf("failed to delete resource in %s with err %v", res, err)
			}
		}
		deleteRBDPool()
		deleteSecret(rbdExamplePath + "secret.yaml")
		deleteStorageClass(rbdExamplePath + "storageclass.yaml")
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {

			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"
			By("checking provisioner deployment is completed")
			err := waitForDeploymentComplete(rbdDeploymentName, namespace, f.ClientSet, deployTimeout)
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
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != totalCount {
					framework.Logf("backend image creation not matching pvc count, image count = % pvc count %d", len(images), totalCount)
					Fail("validate multiple pvc failed")
				}

				//delete pvc and app
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
					framework.Logf("left out rbd backend images count %d", len(images))
					Fail("validate multiple pvc failed")
				}
			})
		})
	})

})
