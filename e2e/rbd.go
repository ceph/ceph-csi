package e2e

import (
	"time"

	. "github.com/onsi/ginkgo" // nolint

	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
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
				e2elog.Logf("failed to delete resource in %s with err %v", res, err)
			}
		}
		deleteRBDPool()
		deleteResource(rbdExamplePath + "secret.yaml")
		deleteResource(rbdExamplePath + "storageclass.yaml")
		deleteResource(rbdExamplePath + "snapshotclass.yaml")
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"
			// rawPvcPath := rbdExamplePath + "raw-block-pvc.yaml"
			// rawAppPath := rbdExamplePath + "raw-block-pod.yaml"
			pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
			appClonePath := rbdExamplePath + "pod-restore.yaml"
			snapshotPath := rbdExamplePath + "snapshot.yaml"

			totalCount := 20

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
				validatePVCAndAppBinding(pvcPath, appPath, f)
			})

			By("create a PVC and Bind it to an app with normal user", func() {
				validateNormalUserPVCAccess(pvcPath, f)
			})

			// skipped raw pvc test in travis
			// By("create a block type PVC and Bind it to an app", func() {
			// 	validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
			// })

			By("create/delete multiple PVCs and Apps", func() {
				validatePVCAndApp(true, pvcPath, appPath, totalCount, f)
			})

			By("create/delete multiple clone PVCs with datasource=snapsphot and Apps", func() {
				createRBDSnapshotClass(f)
				validateCloneFromSnapshot(pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath, totalCount, f)
			})
		})
	})

})
