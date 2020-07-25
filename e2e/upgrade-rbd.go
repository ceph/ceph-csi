package e2e

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var _ = Describe("RBD Upgrade Testing", func() {
	f := framework.NewDefaultFramework("upgrade-test-rbd")
	var (
		// cwd stores the initial working directory.
		cwd string
		c   clientset.Interface
		pvc *v1.PersistentVolumeClaim
		app *v1.Pod
	)

	// deploy rbd CSI
	BeforeEach(func() {
		if !upgradeTesting || !testRBD {
			Skip("Skipping RBD Upgrade Testing")
		}
		c = f.ClientSet
		if cephCSINamespace != defaultNs {
			err := createNamespace(c, cephCSINamespace)
			if err != nil {
				Fail(err.Error())
			}
		}
		createNodeLabel(f, nodeRegionLabel, regionValue)
		createNodeLabel(f, nodeZoneLabel, zoneValue)

		// fetch current working directory to switch back
		// when we are done upgrading.
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			Fail(err.Error())
		}

		deployVault(f.ClientSet, deployTimeout)
		err = upgradeAndDeployCSI(upgradeVersion, "rbd")
		if err != nil {
			Fail(err.Error())
		}
		createConfigMap(rbdDirPath, f.ClientSet, f)
		createRBDStorageClass(f.ClientSet, f, nil, nil)
		createRBDSecret(f.ClientSet, f)
	})
	AfterEach(func() {
		if !testRBD || !upgradeTesting {
			Skip("Skipping RBD Upgrade Testing")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-rbd", c)
			// log provisoner
			logsCSIPods("app=csi-rbdplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-rbdplugin", c)
		}

		deleteConfigMap(rbdDirPath)
		deleteResource(rbdExamplePath + "secret.yaml")
		deleteResource(rbdExamplePath + "storageclass.yaml")
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					Fail(err.Error())
				}
			}
		}
		deleteNodeLabel(c, nodeRegionLabel)
		deleteNodeLabel(c, nodeZoneLabel)
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("checking nodeplugin deamonsets is running", func() {
				err := waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("upgrade to latest changes and verify app re-mount", func() {
				// TODO: fetch pvc size from spec.
				pvcSize := "2Gi"
				var err error
				pvc, err = loadPVC(pvcPath)
				if pvc == nil {
					Fail(err.Error())
				}
				pvc.Namespace = f.UniqueName
				e2elog.Logf("The PVC  template %+v", pvc)

				app, err = loadApp(appPath)
				if err != nil {
					Fail(err.Error())
				}
				app.Namespace = f.UniqueName
				app.Labels = map[string]string{"app": "upgrade-testing"}
				pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
				deleteRBDPlugin()

				err = os.Chdir(cwd)
				if err != nil {
					Fail(err.Error())
				}

				deployRBDPlugin()
				// validate if the app gets bound to a pvc created by
				// an earlier release.
				app.Labels = map[string]string{"app": "upgrade-testing"}
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					Fail(err.Error())
				}
			})

			By("Resize pvc and verify expansion", func() {
				pvcExpandSize := "5Gi"

				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
					Fail(err.Error())
				}
				// Resize 0.3.0 is only supported from v1.15+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "15") {
					opt := metav1.ListOptions{
						LabelSelector: "app=upgrade-testing",
					}
					pvc, err = f.ClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvc.Name, metav1.GetOptions{})
					if err != nil {
						Fail(err.Error())
					}

					// resize PVC
					err = expandPVCSize(f.ClientSet, pvc, pvcExpandSize, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// wait for application pod to come up after resize
					err = waitForPodInRunningState(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						Fail(err.Error())
					}
					// validate if resize is successful.
					err = checkDirSize(app, f, &opt, pvcExpandSize)
					if err != nil {
						Fail(err.Error())
					}
				}

			})

			By("delete pvc and app", func() {
				err := deletePVCAndApp("", f, pvc, app)
				if err != nil {
					Fail(err.Error())
				}
			})
		})
	})
})
