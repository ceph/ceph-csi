package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		// checkSum stores the md5sum of a file to verify uniqueness.
		checkSum string
	)
	const (
		pvcSize  = "2Gi"
		appKey   = "app"
		appLabel = "rbd-upgrade-testing"
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
				e2elog.Failf("failed to create namespace with error %v", err)
			}
		}

		// fetch current working directory to switch back
		// when we are done upgrading.
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			e2elog.Failf("failed to do  getwd with error %v", err)
		}

		deployVault(f.ClientSet, deployTimeout)
		err = upgradeAndDeployCSI(upgradeVersion, "rbd")
		if err != nil {
			e2elog.Failf("failed to upgrade and deploy CSI with error %v", err)
		}
		err = createConfigMap(rbdDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap with error %v", err)
		}
		err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
		if err != nil {
			e2elog.Failf("failed to create storageclass with error %v", err)
		}
		// create rbd provisioner secret
		key, err := createCephUser(f, keyringRBDProvisionerUsername, rbdProvisionerCaps("", ""))
		if err != nil {
			e2elog.Failf("failed to create user %s with error %v", keyringRBDProvisionerUsername, err)
		}
		err = createRBDSecret(f, rbdProvisionerSecretName, keyringRBDProvisionerUsername, key)
		if err != nil {
			e2elog.Failf("failed to create provisioner secret with error %v", err)
		}
		// create rbd plugin secret
		key, err = createCephUser(f, keyringRBDNodePluginUsername, rbdNodePluginCaps("", ""))
		if err != nil {
			e2elog.Failf("failed to create user %s with error %v", keyringRBDNodePluginUsername, err)
		}
		err = createRBDSecret(f, rbdNodePluginSecretName, keyringRBDNodePluginUsername, key)
		if err != nil {
			e2elog.Failf("failed to create node secret with error %v", err)
		}
		err = createRBDSnapshotClass(f)
		if err != nil {
			e2elog.Failf("failed to create snapshotclass with error %v", err)
		}

		err = createNodeLabel(f, nodeRegionLabel, regionValue)
		if err != nil {
			e2elog.Failf("failed to create node label with error %v", err)
		}
		err = createNodeLabel(f, nodeZoneLabel, zoneValue)
		if err != nil {
			e2elog.Failf("failed to create node label with error %v", err)
		}
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

			// log all details from the namespace where Ceph-CSI is deployed
			framework.DumpAllNamespaceInfo(c, cephCSINamespace)
		}

		err := deleteConfigMap(rbdDirPath)
		if err != nil {
			e2elog.Failf("failed to delete configmap with error %v", err)
		}
		err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), rbdProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete provisioner secret with error %v", err)
		}
		err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), rbdNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete node secret with error %v", err)
		}
		err = deleteResource(rbdExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass with error %v", err)
		}
		err = deleteResource(rbdExamplePath + "snapshotclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete snapshotclass with error %v", err)
		}
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err = deleteNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to delete namespace with error %v", err)
				}
			}
		}
		err = deleteNodeLabel(c, nodeRegionLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
		err = deleteNodeLabel(c, nodeZoneLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			pvcPath := rbdExamplePath + "pvc.yaml"
			appPath := rbdExamplePath + "pod.yaml"

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment %s with error %v", rbdDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset %s with error %v", rbdDaemonsetName, err)
				}
			})

			By("upgrade to latest changes and verify app re-mount", func() {
				// TODO: fetch pvc size from spec.
				var err error
				label := make(map[string]string)
				data := "check data persists"

				pvc, err = loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load pvc with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err = loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				label[appKey] = appLabel
				app.Namespace = f.UniqueName
				app.Labels = label
				pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create pvc with error %v", err)
				}
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				// fetch the path where volume is mounted.
				mountPath := app.Spec.Containers[0].VolumeMounts[0].MountPath
				filePath := filepath.Join(mountPath, "testClone")

				// create a test file at the mountPath.
				_, stdErr := execCommandInPodAndAllowFail(f, fmt.Sprintf("echo %s > %s", data, filePath), app.Namespace, &opt)
				if stdErr != "" {
					e2elog.Failf("failed to write data to a file %s", stdErr)
				}

				// force an immediate write of all cached data to disk.
				_, stdErr = execCommandInPodAndAllowFail(f, fmt.Sprintf("sync %s", filePath), app.Namespace, &opt)
				if stdErr != "" {
					e2elog.Failf("failed to sync data to a disk %s", stdErr)
				}

				opt = metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", appLabel),
				}
				e2elog.Logf("Calculating checksum of %s", filePath)
				checkSum, err = calculateSHA512sum(f, app, filePath, &opt)
				if err != nil {
					e2elog.Failf("failed to calculate checksum of %s", filePath)
				}

				// pvc clone is only supported from v1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					// Create snapshot of the pvc
					snapshotPath := rbdExamplePath + "snapshot.yaml"
					snap := getSnapshot(snapshotPath)
					snap.Name = "rbd-pvc-snapshot"
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create snapshot %v", err)
					}
				}
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application with error %v", err)
				}
				deleteRBDPlugin()

				err = os.Chdir(cwd)
				if err != nil {
					e2elog.Failf("failed to change directory with error %v", err)
				}

				deployRBDPlugin()
				// validate if the app gets bound to a pvc created by
				// an earlier release.
				app.Labels = label
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create application with error %v", err)
				}
			})

			By("Create clone from a snapshot", func() {
				pvcClonePath := rbdExamplePath + "pvc-restore.yaml"
				appClonePath := rbdExamplePath + "pod-restore.yaml"
				label := make(map[string]string)

				// pvc clone is only supported from v1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					pvcClone, err := loadPVC(pvcClonePath)
					if err != nil {
						e2elog.Failf("failed to load pvc with error %v", err)
					}
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
					pvcClone.Spec.DataSource.Name = "rbd-pvc-snapshot"
					appClone, err := loadApp(appClonePath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					label[appKey] = "validate-snap-clone"
					appClone.Namespace = f.UniqueName
					appClone.Name = "app-clone-from-snap"
					appClone.Labels = label
					err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create pvc with error %v", err)
					}
					opt := metav1.ListOptions{
						LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
					}
					mountPath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath
					testFilePath := filepath.Join(mountPath, "testClone")
					newCheckSum, err := calculateSHA512sum(f, appClone, testFilePath, &opt)
					if err != nil {
						e2elog.Failf("failed to calculate checksum of %s", testFilePath)
					}
					if strings.Compare(newCheckSum, checkSum) != 0 {
						e2elog.Failf("The checksum of files did not match, expected %s received %s  ", checkSum, newCheckSum)
					}
					e2elog.Logf("The checksum of files matched")

					// delete cloned pvc and pod
					err = deletePVCAndApp("", f, pvcClone, appClone)
					if err != nil {
						e2elog.Failf("failed to delete pvc and application with error %v", err)
					}

				}
			})

			By("Create clone from existing PVC", func() {
				pvcSmartClonePath := rbdExamplePath + "pvc-clone.yaml"
				appSmartClonePath := rbdExamplePath + "pod-clone.yaml"
				label := make(map[string]string)

				// pvc clone is only supported from v1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					pvcClone, err := loadPVC(pvcSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to load pvc with error %v", err)
					}
					pvcClone.Spec.DataSource.Name = pvc.Name
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(pvcSize)
					appClone, err := loadApp(appSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					label[appKey] = "validate-clone"
					appClone.Namespace = f.UniqueName
					appClone.Name = "appclone"
					appClone.Labels = label
					err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create pvc with error %v", err)
					}
					opt := metav1.ListOptions{
						LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
					}
					mountPath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath
					testFilePath := filepath.Join(mountPath, "testClone")
					newCheckSum, err := calculateSHA512sum(f, appClone, testFilePath, &opt)
					if err != nil {
						e2elog.Failf("failed to calculate checksum of %s", testFilePath)
					}
					if strings.Compare(newCheckSum, checkSum) != 0 {
						e2elog.Failf("The checksum of files did not match, expected %s received %s  ", checkSum, newCheckSum)
					}
					e2elog.Logf("The checksum of files matched")

					// delete cloned pvc and pod
					err = deletePVCAndApp("", f, pvcClone, appClone)
					if err != nil {
						e2elog.Failf("failed to delete pvc and application with error %v", err)
					}

				}
			})

			By("Resize pvc and verify expansion", func() {
				pvcExpandSize := "5Gi"
				label := make(map[string]string)

				// Resize 0.3.0 is only supported from v1.15+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 15) {
					label[appKey] = appLabel
					opt := metav1.ListOptions{
						LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
					}
					var err error
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
						e2elog.Failf("timeout waiting for pod to be in running state with error %v", err)
					}
					// validate if resize is successful.
					err = checkDirSize(app, f, &opt, pvcExpandSize)
					if err != nil {
						e2elog.Failf("failed to check directory size with error %v", err)
					}
				}

			})

			By("delete pvc and app", func() {
				err := deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete pvc and application with error %v", err)
				}
			})
			// delete RBD provisioner secret
			err := deleteCephUser(f, keyringRBDProvisionerUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s with error %v", keyringRBDProvisionerUsername, err)
			}
			// delete RBD plugin secret
			err = deleteCephUser(f, keyringRBDNodePluginUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s with error %v", keyringRBDNodePluginUsername, err)
			}
		})

	})
})
