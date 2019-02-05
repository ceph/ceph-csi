package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = BeforeSuite(func() {
	cephfsFiles := getCephfsTemp()
	for _, file := range cephfsFiles {
		framework.RunKubectlOrDie("create", "-f", cephfsDirPath+file.Name())
	}

})

var _ = AfterSuite(func() {
	cephfsFiles := getCephfsTemp()
	for _, file := range cephfsFiles {
		framework.RunKubectl("delete", "-f", cephfsDirPath+file.Name())
		deleteSecret()
		deleteSc()
	}
})

var _ = Describe("cephfs", func() {

	f := framework.NewDefaultFramework("cephfs")

	var c kubernetes.Interface
	BeforeEach(func() {
		//set the client object
		c = f.ClientSet

	})

	Describe("check ceph CSI driver is up", func() {
		It("check ceph csi is up", func() {

			By("checking provisioner statefulset is running")
			err := framework.WaitForStatefulSetReplicasReady("csi-cephfsplugin-provisioner", "default", c, 2*time.Second, 2*time.Minute)
			framework.ExpectNoError(err)

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets("csi-cephfsplugin", "default", c, 2*time.Minute)
			framework.ExpectNoError(err)

			By("checking attacher statefulset is running")
			err = framework.WaitForStatefulSetReplicasReady("csi-cephfsplugin-attacher", "default", c, 2*time.Second, 2*time.Minute)
			framework.ExpectNoError(err)
		})
	})

	Describe("create a PVC and Bind it to an app", func() {
		It("create a PVC and Bind it to an app", func() {

			By("create storage class")
			//TODO need to move all resource to test namespace
			createStorageClass(c)
			createSecret(c, f)
		})
	})
})
