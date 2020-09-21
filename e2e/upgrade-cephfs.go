package e2e

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var _ = Describe("CephFS Upgrade Testing", func() {
	f := framework.NewDefaultFramework("upgrade-test-cephfs")
	var (
		c   clientset.Interface
		pvc *v1.PersistentVolumeClaim
		app *v1.Pod
		// cwd stores the initial working directory.
		cwd string
		err error
	)
	// deploy cephfs CSI
	BeforeEach(func() {
		if !upgradeTesting || !testCephFS {
			Skip("Skipping CephFS Upgrade Test")
		}
		c = f.ClientSet
		if cephCSINamespace != defaultNs {
			err = createNamespace(c, cephCSINamespace)
			if err != nil {
				e2elog.Failf("failed to create namespace with error %v", err)
			}
		}

		// fetch current working directory to switch back
		// when we are done upgrading.
		cwd, err = os.Getwd()
		if err != nil {
			e2elog.Failf("failed to getwd with error %v", err)
		}
		err = upgradeAndDeployCSI(upgradeVersion, "cephfs")
		if err != nil {
			e2elog.Failf("failed to upgrade csi with error %v", err)
		}
		err = createConfigMap(cephfsDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap with error %v", err)
		}
		err = createCephfsSecret(f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create secret with error %v", err)
		}
		err = createCephfsStorageClass(f.ClientSet, f, true, nil)
		if err != nil {
			e2elog.Failf("failed to create storageclass with error %v", err)
		}
	})
	AfterEach(func() {
		if !testCephFS || !upgradeTesting {
			Skip("Skipping CephFS Upgrade Test")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-cephfs", c)
			// log provisoner
			logsCSIPods("app=csi-cephfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-cephfsplugin", c)
		}
		err = deleteConfigMap(cephfsDirPath)
		if err != nil {
			e2elog.Failf("failed to delete configmap with error %v", err)
		}
		err = deleteResource(cephfsExamplePath + "secret.yaml")
		if err != nil {
			e2elog.Failf("failed to delete secret with error %v", err)
		}
		err = deleteResource(cephfsExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass with error %v", err)
		}
		if deployCephFS {
			deleteCephfsPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					if err != nil {
						e2elog.Failf("failed to delete namespace with error %v", err)
					}
				}
			}
		}
	})

	Context("Cephfs Upgrade Test", func() {
		It("Cephfs Upgrade Test", func() {

			By("checking provisioner deployment is running")
			err := waitForDeploymentComplete(cephfsDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
			if err != nil {
				e2elog.Failf("timeout waiting for deployment %s with error %v", cephfsDeploymentName, err)
			}

			By("checking nodeplugin deamonset pods are running")
			err = waitForDaemonSets(cephfsDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
			if err != nil {
				e2elog.Failf("timeout waiting for daemonset %s with error%v", cephfsDeamonSetName, err)
			}

			By("upgrade to latest changes and verify app re-mount", func() {
				// TODO: fetch pvc size from spec.
				pvcSize := "2Gi"
				pvcPath := cephfsExamplePath + "pvc.yaml"
				appPath := cephfsExamplePath + "pod.yaml"

				pvc, err = loadPVC(pvcPath)
				if pvc == nil {
					e2elog.Failf("failed to load pvc with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err = loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				app.Labels = map[string]string{"app": "upgrade-testing"}
				pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create pvc and application with error %v", err)
				}
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application with error %v", err)
				}
				deleteCephfsPlugin()

				// switch back to current changes.
				err = os.Chdir(cwd)
				if err != nil {
					e2elog.Failf("failed to d chdir with error %v", err)
				}
				deployCephfsPlugin()

				app.Labels = map[string]string{"app": "upgrade-testing"}
				// validate if the app gets bound to a pvc created by
				// an earlier release.
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create application with error %v", err)
				}
			})

			By("Resize pvc and verify expansion", func() {
				var v *version.Info
				pvcExpandSize := "5Gi"
				v, err = f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Logf("failed to get server version with error %v", err)
				}
				// Resize 0.3.0 is only supported from v1.15+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "15") {
					opt := metav1.ListOptions{
						LabelSelector: "app=upgrade-testing",
					}
					pvc, err = f.ClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvc.Name, metav1.GetOptions{})
					if err != nil {
						e2elog.Failf("failed to get pvc with error %v", err)
					}

					// resize PVC
					err = expandPVCSize(f.ClientSet, pvc, pvcExpandSize, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to expand pvc with error %v", err)
					}
					// wait for application pod to come up after resize
					err = waitForPodInRunningState(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("timout waiting for pod to be in running state with error %v", err)
					}
					// validate if resize is successful.
					err = checkDirSize(app, f, &opt, pvcExpandSize)
					if err != nil {
						e2elog.Failf("failed to check directory size with error %v", err)
					}
				}

			})

			By("delete pvc and app")
			err = deletePVCAndApp("", f, pvc, app)
			if err != nil {
				e2elog.Failf("failed to delete pvc and application with error %v", err)
			}
		})
	})
})
