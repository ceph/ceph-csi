package e2e

import (
	"time"

	. "github.com/onsi/ginkgo" // nolint

	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	cephfsProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephfsNodePlugin      = "csi-cephfsplugin.yaml"
	cephfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
)

var (
	cephfsDirPath = "../deploy/cephfs/kubernetes/"

	cephfsExamplePath = "../examples/cephfs/"
)

func deployCephfsPlugin() {
	//deploy provisioner
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsProvisioner)
	framework.RunKubectlOrDie("apply", "-f", cephfsDirPath+cephfsProvisionerRBAC)
	//deploy nodeplugin
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephfsNodePlugin)
	framework.RunKubectlOrDie("apply", "-f", cephfsDirPath+cephfsNodePluginRBAC)
}

var _ = Describe("cephfs", func() {
	f := framework.NewDefaultFramework("cephfs")
	//deploy cephfs CSI
	BeforeEach(func() {
		createFileSystem(f.ClientSet)
		deployCephfsPlugin()
		createCephfsStorageClass(f.ClientSet)
		createCephfsSecret(f.ClientSet, f)
	})

	AfterEach(func() {
		cephfsFiles := getFilesinDirectory(cephfsDirPath)
		for _, file := range cephfsFiles {
			res, err := framework.RunKubectl("delete", "-f", cephfsDirPath+file.Name())
			framework.Logf("failed to delete resource in %s with err %v", res, err)
		}
		deleteSecret(cephfsExamplePath + "secret.yaml")
		deleteStorageClass(cephfsExamplePath + "storageclass.yaml")
		deleteFileSystem()
	})

	Context("Test cephfs CSI", func() {
		It("Test cephfs CSI", func() {
			By("checking provisioner statefulset is running")
			timeout := time.Duration(deployTimeout) * time.Minute
			err := framework.WaitForStatefulSetReplicasReady("csi-cephfsplugin-provisioner", "default", f.ClientSet, 1*time.Second, timeout)
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets("csi-cephfsplugin", "default", f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("create and delete a PVC", func() {
				By("create a PVC and Bind it to an app", func() {
					pvcPath := cephfsExamplePath + "pvc.yaml"
					appPath := cephfsExamplePath + "pod.yaml"
					validatePVCAndAppBinding(pvcPath, appPath, f)

				})

			})
		})
	})

})
