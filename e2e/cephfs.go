package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	apps "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	cephProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephAttacher        = "csi-cephfsplugin-attacher.yaml"
	cephAttacherRBAC    = "csi-attacher-rbac.yaml"
	cephNodePlugin      = "csi-cephfsplugin.yaml"
	cephNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
)

var (
	cephfsDirPath = "../deploy/cephfs/kubernetes/"

	cephfsExamplePath = "../examples/cephfs/"
	defaultNS         = "default"
)

func deployProvisioner(c kubernetes.Interface) {
	pro := &apps.StatefulSet{}
	pPath := cephfsDirPath + cephProvisioner
	err := unmarshal(pPath, pro)
	framework.ExpectNoError(err)
	//TODO need to update the image name
	_, err = c.AppsV1().StatefulSets(defaultNS).Create(pro)
	framework.ExpectNoError(err)
	//create provisoner RBAC
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephProvisionerRBAC)
}

func deployNodePlugin(c kubernetes.Interface) {
	pro := &apps.DaemonSet{}
	pPath := cephfsDirPath + cephNodePlugin
	err := unmarshal(pPath, pro)
	framework.ExpectNoError(err)
	//TODO need to update the image name
	_, err = c.AppsV1().DaemonSets(defaultNS).Create(pro)
	framework.ExpectNoError(err)
	//create provisoner RBAC
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephNodePluginRBAC)
}

func deployAttacher(c kubernetes.Interface) {
	pro := &apps.StatefulSet{}
	pPath := cephfsDirPath + cephAttacher
	err := unmarshal(pPath, pro)
	framework.ExpectNoError(err)
	//TODO need to update the image name
	_, err = c.AppsV1().StatefulSets(defaultNS).Create(pro)
	framework.ExpectNoError(err)
	//create provisoner RBAC
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephAttacherRBAC)
}

var f = framework.NewDefaultFramework("cephfs")

var _ = BeforeSuite(func() {
	deployProvisioner(f.ClientSet)
	deployNodePlugin(f.ClientSet)
	deployAttacher(f.ClientSet)
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
	//f := framework.NewDefaultFramework("cephfs")
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
			By("create secret")
			createSecret(c, f)
			By("create PVC and check PVC state")
			pvcPath := cephfsExamplePath + "pvc.yaml"
			err := createPVC(c, pvcPath, 2*time.Minute)
			framework.ExpectNoError(err)
			//defer to  delete  PVC
			//TODO check  pvc  and PV after deleting
			defer func(pvc string) {
				framework.RunKubectl("delete", "-f", pvc)
			}(pvcPath)

			By("create app and bind PVC to app")
			appPath := cephfsExamplePath + "pod.yaml"
			err = createApp(c, appPath, 2*time.Minute)
			framework.ExpectNoError(err)
			//TODO check app is deleted or  not
			framework.RunKubectl("delete", "-f", appPath)

		})
	})
})
