package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	apps "k8s.io/api/apps/v1"
	v1beta1 "k8s.io/api/apps/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	cephProvisioner     = "csi-cephfsplugin-provisioner.yaml"
	cephProvisionerRBAC = "csi-provisioner-rbac.yaml"
	cephProvisionerSVC  = "csi-cephfsplugin-provisioner-svc.yaml"
	cephAttacher        = "csi-cephfsplugin-attacher.yaml"
	cephAttacherRBAC    = "csi-attacher-rbac.yaml"
	cephAttacherSVC     = "csi-cephfsplugin-attacher-svc.yaml"
	cephNodePlugin      = "csi-cephfsplugin.yaml"
	cephNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
)

var (
	cephfsDirPath = "../deploy/cephfs/kubernetes/"

	cephfsExamplePath = "../examples/cephfs/"
	defaultNS         = "default"
)

func createService(c kubernetes.Interface, ns, sPath string) {
	svc := &v1.Service{}
	err := unmarshal(sPath, svc)
	framework.ExpectNoError(err)
	_, err = c.CoreV1().Services(defaultNS).Create(svc)
	framework.ExpectNoError(err)
}
func deployProvisioner(c kubernetes.Interface) {
	pro := &v1beta1.StatefulSet{}
	pPath := cephfsDirPath + cephProvisioner
	err := unmarshal(pPath, pro)
	framework.ExpectNoError(err)
	//TODO need to update the image name
	_, err = c.AppsV1beta1().StatefulSets(defaultNS).Create(pro)
	framework.ExpectNoError(err)
	sPath := cephfsDirPath + cephProvisionerSVC
	createService(c, defaultNS, sPath)
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
	pro := &v1beta1.StatefulSet{}
	pPath := cephfsDirPath + cephAttacher
	err := unmarshal(pPath, pro)
	framework.ExpectNoError(err)
	//TODO need to update the image name
	_, err = c.AppsV1beta1().StatefulSets(defaultNS).Create(pro)
	framework.ExpectNoError(err)
	//create provisoner RBAC
	sPath := cephfsDirPath + cephAttacherSVC
	createService(c, defaultNS, sPath)
	framework.RunKubectlOrDie("create", "-f", cephfsDirPath+cephAttacherRBAC)
}

var f = framework.NewDefaultFramework("cephfs")

var beforeFirst = true
var c kubernetes.Interface

//BeforeAll will get executed only once for each Describe
func BeforeAll(fn func()) {
	BeforeEach(func() {
		c = f.ClientSet
		if beforeFirst {
			fn()
			beforeFirst = false
		}
	})
}

//teardown the ceph CSI
func Cleanup() {
	cephfsFiles := getCephfsTemp()
	for _, file := range cephfsFiles {
		framework.RunKubectl("delete", "-f", cephfsDirPath+file.Name())
		deleteSecret()
		deleteSc()
	}
}

var _ = Describe("cephfs", func() {
	//f := framework.NewDefaultFramework("cephfs")
	//deploy ceph CSI
	BeforeAll(func() {
		deployProvisioner(f.ClientSet)
		deployNodePlugin(f.ClientSet)
		deployAttacher(f.ClientSet)
	})

	Describe("check ceph CSI driver is up", func() {
		It("check ceph csi is up", func() {

			By("checking provisioner statefulset is running")
			err := framework.WaitForStatefulSetReplicasReady("csi-cephfsplugin-provisioner", "default", c, 2*time.Second, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets("csi-cephfsplugin", "default", c, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}

			By("checking attacher statefulset is running")
			err = framework.WaitForStatefulSetReplicasReady("csi-cephfsplugin-attacher", "default", c, 2*time.Second, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}
		})
	})

	Describe("Test PVC Binding", func() {

		It("create storage class", func() {
			createStorageClass(c)
		})

		It("create secret", func() {
			createSecret(c, f)
		})

		By("load pvc")
		pvcPath := cephfsExamplePath + "pvc.yaml"
		pvc := loadPVC(pvcPath)

		It("create a PVC", func() {
			err := createPVCAndvalidatePV(c, pvc, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}
		})

		appPath := cephfsExamplePath + "pod.yaml"
		app := loadApp(appPath)
		It("create app and bind PVC to app", func() {
			err := createApp(c, app, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}
		})

		//TODO need to check mount writable

		It("delete app", func() {
			err := deletePod(app.Name, app.Namespace, c, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}
		})

		It("delete PVC and check PV", func() {
			err := deletePVCAndValidatePV(c, pvc, 2*time.Minute)
			if err != nil {
				Fail(err.Error())
			}
		})

		Cleanup()

	})
})
