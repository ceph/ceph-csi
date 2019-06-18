package e2e

import (
	"fmt"
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
			if err != nil {
				framework.Logf("failed to delete resource in %s with err %v", res, err)
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

			By("create a PVC clone and Bind it to an app", func() {
				createRBDSnapshotClass(f)
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
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != 1 {
					framework.Logf("backend image count %d expected image count %d", len(images), 1)
					Fail("validate backend image failed")
				}
				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.Name = pvc.Name
				snap.Spec.Source.Kind = "PersistentVolumeClaim"
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				pool := "replicapool"
				snapList, err := listSnapshots(f, pool, images[0])
				if err != nil {
					Fail(err.Error())
				}
				if len(snapList) != 1 {
					framework.Logf("backend snapshot not matching kube snap count,snap count = % kube snap count %d", len(snapList), 1)
					Fail("validate backend snapshot failed")
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

			// skipped raw pvc test in travis
			// By("create a block type PVC and Bind it to an app", func() {
			// 	validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
			// })

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
				// validate created backend rbd images
				images := listRBDImages(f)
				if len(images) != totalCount {
					framework.Logf("backend image creation not matching pvc count, image count = % pvc count %d", len(images), totalCount)
					Fail("validate multiple pvc failed")
				}

				// delete pvc and app
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
