package e2e

import (
	"time"

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
			framework.Logf("failed to delete resource in %s with err %v", res, err)
		}
		deleteRBDPool()
		deleteSecret(rbdExamplePath + "secret.yaml")
		deleteStorageClass(rbdExamplePath + "storageclass.yaml")
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			By("checking provisioner statefulset is running")
			timeout := time.Duration(deployTimeout) * time.Minute
			err := framework.WaitForStatefulSetReplicasReady(rbdDeploymentName, namespace, f.ClientSet, 1*time.Second, timeout)
			if err != nil {
				Fail(err.Error())
			}

			By("checking nodeplugin deamonsets is running")
			err = waitForDaemonSets(rbdDaemonsetName, namespace, f.ClientSet, deployTimeout)
			if err != nil {
				Fail(err.Error())
			}

			By("create a PVC and Bind it to an app", func() {
				pvcPath := rbdExamplePath + "pvc.yaml"
				appPath := rbdExamplePath + "pod.yaml"
				validatePVCAndAppBinding(pvcPath, appPath, f)
			})

			By("create a PVC and Bind it to an app with normal user", func() {
				pvcPath := rbdExamplePath + "pvc.yaml"
				validateNormalUserPVCAccess(pvcPath, f)
			})
		})
	})

})
