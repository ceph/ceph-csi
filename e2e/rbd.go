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
		deleteResource(rbdExamplePath + "secret.yaml")
		deleteResource(rbdExamplePath + "storageclass.yaml")
		deleteResource(rbdExamplePath + "snapshotclass.yaml")
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

			By("create a PVC clone and Bind it to an app", func() {
				createRBDSnapshotClass(f)
				pvcPath := rbdExamplePath + "pvc.yaml"
				pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
				appClonePath := rbdExamplePath + "pod-restore.yaml"
				snapshotPath := rbdExamplePath + "snapshot.yaml"
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					Fail(err.Error())
				}

				pvc.Namespace = f.UniqueName
				framework.Logf("The PVC  template %+v", pvc)
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.Name = pvc.Name
				snap.Spec.Source.Kind = "PersistentVolumeClaim"
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}

				validatePVCAndAppBinding(pvcClonePath, appClonePath, f)

				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("create a block type PVC and Bind it to an app", func() {
				pvcPath := rbdExamplePath + "raw-block-pvc.yaml"
				appPath := rbdExamplePath + "raw-block-pod.yaml"
				validatePVCAndAppBinding(pvcPath, appPath, f)
			})
		})
	})

})
