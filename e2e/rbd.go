package e2e

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	rbdProvisioner     = "csi-rbdplugin-provisioner.yaml"
	rbdProvisionerRBAC = "csi-provisioner-rbac.yaml"
	rbdProvisionerPSP  = "csi-provisioner-psp.yaml"
	rbdNodePlugin      = "csi-rbdplugin.yaml"
	rbdNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	rbdNodePluginPSP   = "csi-nodeplugin-psp.yaml"
	configMap          = "csi-config-map.yaml"
	cephConfconfigMap  = "ceph-conf.yaml"
	csiDriverObject    = "csidriver.yaml"
	rbdDirPath         = "../deploy/rbd/kubernetes/"
	examplePath        = "../examples/"
	rbdExamplePath     = examplePath + "/rbd/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	defaultRBDPool     = "replicapool"
	// Topology related variables.
	nodeRegionLabel     = "test.failure-domain/region"
	regionValue         = "testregion"
	nodeZoneLabel       = "test.failure-domain/zone"
	zoneValue           = "testzone"
	nodeCSIRegionLabel  = "topology.rbd.csi.ceph.com/region"
	nodeCSIZoneLabel    = "topology.rbd.csi.ceph.com/zone"
	rbdTopologyPool     = "newrbdpool"
	rbdTopologyDataPool = "replicapool" // NOTE: should be different than rbdTopologyPool for test to be effective

	// yaml files required for deployment.
	pvcPath                = rbdExamplePath + "pvc.yaml"
	appPath                = rbdExamplePath + "pod.yaml"
	rawPvcPath             = rbdExamplePath + "raw-block-pvc.yaml"
	rawAppPath             = rbdExamplePath + "raw-block-pod.yaml"
	pvcClonePath           = rbdExamplePath + "pvc-restore.yaml"
	pvcSmartClonePath      = rbdExamplePath + "pvc-clone.yaml"
	pvcBlockSmartClonePath = rbdExamplePath + "pvc-block-clone.yaml"
	appClonePath           = rbdExamplePath + "pod-restore.yaml"
	appSmartClonePath      = rbdExamplePath + "pod-clone.yaml"
	appBlockSmartClonePath = rbdExamplePath + "block-pod-clone.yaml"
	snapshotPath           = rbdExamplePath + "snapshot.yaml"
	defaultCloneCount      = 10

	nbdMapOptions = "debug-rbd=20"
)

func deployRBDPlugin() {
	// delete objects deployed by rook
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		e2elog.Failf("failed to delete provisioner rbac %s with error %v", rbdDirPath+rbdProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		e2elog.Failf("failed to delete nodeplugin rbac %s with error %v", rbdDirPath+rbdNodePluginRBAC, err)
	}

	createORDeleteRbdResources(kubectlCreate)
}

func deleteRBDPlugin() {
	createORDeleteRbdResources(kubectlDelete)
}

func createORDeleteRbdResources(action kubectlAction) {
	csiDriver, err := ioutil.ReadFile(rbdDirPath + csiDriverObject)
	if err != nil {
		// createORDeleteRbdResources is used for upgrade testing as csidriverObject is
		// newly added, discarding file not found error.
		if !os.IsNotExist(err) {
			e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+csiDriverObject, err)
		}
	} else {
		err = retryKubectlInput(cephCSINamespace, action, string(csiDriver), deployTimeout)
		if err != nil {
			e2elog.Failf("failed to %s CSIDriver object with error %v", action, err)
		}
	}
	cephConf, err := ioutil.ReadFile(examplePath + cephConfconfigMap)
	if err != nil {
		// createORDeleteRbdResources is used for upgrade testing as cephConf Configmap is
		// newly added, discarding file not found error.
		if !os.IsNotExist(err) {
			e2elog.Failf("failed to read content from %s with error %v", examplePath+cephConfconfigMap, err)
		}
	} else {
		err = retryKubectlInput(cephCSINamespace, action, string(cephConf), deployTimeout)
		if err != nil {
			e2elog.Failf("failed to %s ceph-conf configmap object with error %v", action, err)
		}
	}
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisioner)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisioner, err)
	}
	data = oneReplicaDeployYaml(data)
	data = enableTopologyInTemplate(data)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s rbd provisioner with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s provisioner rbac with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdProvisionerPSP, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s provisioner psp with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePlugin)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePlugin, err)
	}

	domainLabel := nodeRegionLabel + "," + nodeZoneLabel
	data = addTopologyDomainsToDSYaml(data, domainLabel)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin rbac with error %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginPSP)
	if err != nil {
		e2elog.Failf("failed to read content from %s with error %v", rbdDirPath+rbdNodePluginPSP, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to %s nodeplugin psp with error %v", action, err)
	}
}

func validateRBDImageCount(f *framework.Framework, count int, pool string) {
	imageList, err := listRBDImages(f, pool)
	if err != nil {
		e2elog.Failf("failed to list rbd images with error %v", err)
	}
	if len(imageList) != count {
		e2elog.Failf(
			"backend images not matching kubernetes resource count,image count %d kubernetes resource count %d"+
				"\nbackend image Info:\n %v",
			len(imageList),
			count,
			imageList)
	}
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("rbd")
	var c clientset.Interface
	var kernelRelease string
	// deploy RBD CSI
	BeforeEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		c = f.ClientSet
		if deployRBD {
			err := createNodeLabel(f, nodeRegionLabel, regionValue)
			if err != nil {
				e2elog.Failf("failed to create node label with error %v", err)
			}
			err = createNodeLabel(f, nodeZoneLabel, zoneValue)
			if err != nil {
				e2elog.Failf("failed to create node label with error %v", err)
			}
			if cephCSINamespace != defaultNs {
				err = createNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to create namespace with error %v", err)
				}
			}
			deployRBDPlugin()
		}
		err := createConfigMap(rbdDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap with error %v", err)
		}
		// Since helm deploys storageclass, skip storageclass creation if
		// ceph-csi is deployed via helm.
		if !helmTest {
			err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
			if err != nil {
				e2elog.Failf("failed to create storageclass with error %v", err)
			}
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
		deployVault(f.ClientSet, deployTimeout)

		// wait for provisioner deployment
		err = waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
		if err != nil {
			e2elog.Failf("timeout waiting for deployment %s with error %v", rbdDeploymentName, err)
		}

		// wait for nodeplugin deamonset pods
		err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
		if err != nil {
			e2elog.Failf("timeout waiting for daemonset %s with error %v", rbdDaemonsetName, err)
		}

		kernelRelease, err = getKernelVersionFromDaemonset(f, cephCSINamespace, rbdDaemonsetName, "csi-rbdplugin")
		if err != nil {
			e2elog.Failf("failed to get the kernel version with error %v", err)
		}
		// default io-timeout=0, needs kernel >= 5.4
		if !util.CheckKernelSupport(kernelRelease, nbdZeroIOtimeoutSupport) {
			nbdMapOptions = "debug-rbd=20,io-timeout=330"
		}
	})

	AfterEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-rbd", c)
			// log provisioner
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
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete provisioner secret with error %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete node secret with error %v", err)
		}
		err = deleteResource(rbdExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass with error %v", err)
		}
		// deleteResource(rbdExamplePath + "snapshotclass.yaml")
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
		// Remove the CSI labels that get added
		err = deleteNodeLabel(c, nodeCSIRegionLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
		err = deleteNodeLabel(c, nodeCSIZoneLabel)
		if err != nil {
			e2elog.Failf("failed to delete node label with error %v", err)
		}
	})

	Context("Test RBD CSI", func() {
		It("Test RBD CSI", func() {
			// test only if ceph-csi is deployed via helm
			if helmTest {
				By("verify PVC and app binding on helm installation", func() {
					err := validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to validate CephFS pvc and application binding with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					//  Deleting the storageclass and secret created by helm
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = deleteResource(rbdExamplePath + "secret.yaml")
					if err != nil {
						e2elog.Failf("failed to delete secret with error %v", err)
					}
					// Re-create the RBD storageclass
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
				})
			}

			By("create a PVC and validate owner", func() {
				err := validateImageOwner(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate owner of pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate normal user pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a PVC and bind it to an app with ext4 as the FS ", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"csi.storage.k8s.io/fstype": "ext4"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app using rbd-nbd mounter", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":    "rbd-nbd",
						"mapOptions": nbdMapOptions,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("Resize rbd-nbd PVC and check application directory size", func() {
				if util.CheckKernelSupport(kernelRelease, nbdResizeSupport) {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					// Storage class with rbd-nbd mounter
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"mounter":    "rbd-nbd",
							"mapOptions": nbdMapOptions,
						},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					// Block PVC resize
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Failf("failed to resize block PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)

					// FileSystem PVC resize
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
				}
			})

			By("perform IO on rbd-nbd volume after nodeplugin restart", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":    "rbd-nbd",
						"mapOptions": nbdMapOptions,
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}

				app.Namespace = f.UniqueName
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}

				appOpt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}
				// TODO: Remove this once we ensure that rbd-nbd can sync data
				// from Filesystem layer to backend rbd image as part of its
				// detach or SIGTERM signal handler
				_, stdErr, err := execCommandInPod(
					f,
					fmt.Sprintf("sync %s", app.Spec.Containers[0].VolumeMounts[0].MountPath),
					app.Namespace,
					&appOpt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to sync, err: %v, stdErr: %v ", err, stdErr)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDaemonsetName)
				if err != nil {
					e2elog.Failf("failed to get the labels with error %v", err)
				}
				// delete rbd nodeplugin pods
				err = deletePodWithLabel(selector, cephCSINamespace, false)
				if err != nil {
					e2elog.Failf("fail to delete pod with error %v", err)
				}

				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset pods with error %v", err)
				}

				opt := metav1.ListOptions{
					LabelSelector: selector,
				}
				uname, stdErr, err := execCommandInContainer(f, "uname -a", cephCSINamespace, "csi-rbdplugin", &opt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to run uname cmd : %v, stdErr: %v ", err, stdErr)
				}
				e2elog.Logf("uname -a: %v", uname)
				rpmv, stdErr, err := execCommandInContainer(
					f,
					"rpm -qa | grep rbd-nbd",
					cephCSINamespace,
					"csi-rbdplugin",
					&opt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to run rpm -qa cmd : %v, stdErr: %v ", err, stdErr)
				}
				e2elog.Logf("rbd-nbd package version: %v", rpmv)

				timeout := time.Duration(deployTimeout) * time.Minute
				var reason string
				err = wait.PollImmediate(poll, timeout, func() (bool, error) {
					var runningAttachCmd string
					runningAttachCmd, stdErr, err = execCommandInContainer(
						f,
						"ps -eo 'cmd' | grep [r]bd-nbd",
						cephCSINamespace,
						"csi-rbdplugin",
						&opt)
					// if the rbd-nbd process is not running the ps | grep command
					// will return with exit code 1
					if err != nil {
						if strings.Contains(err.Error(), "command terminated with exit code 1") {
							reason = fmt.Sprintf("rbd-nbd process is not running yet: %v", err)
						} else if stdErr != "" {
							reason = fmt.Sprintf("failed to run ps cmd : %v, stdErr: %v", err, stdErr)
						}
						e2elog.Logf("%s", reason)

						return false, nil
					}
					e2elog.Logf("attach command running after restart, runningAttachCmd: %v", runningAttachCmd)

					return true, nil
				})

				if errors.Is(err, wait.ErrWaitTimeout) {
					e2elog.Failf("timed out waiting for the rbd-nbd process: %s", reason)
				}
				if err != nil {
					e2elog.Failf("failed to poll: %v", err)
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr, err = execCommandInPod(
					f,
					fmt.Sprintf("echo 'Hello World' > %s", filePath),
					app.Namespace,
					&appOpt)
				if err != nil || stdErr != "" {
					e2elog.Failf("failed to write IO, err: %v, stdErr: %v ", err, stdErr)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app using rbd-nbd mounter with encryption", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":    "rbd-nbd",
						"mapOptions": nbdMapOptions,
						"encrypted":  "true",
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("Resize Encrypted Block PVC and check Device size", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true"},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				// FileSystem PVC resize
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to resize filesystem PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// Block PVC resize
				err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to resize block PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, vaultKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultTokensKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tokens-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				// name(space) of the Tenant
				tenant := f.UniqueName

				// create the Secret with Vault Token in the Tenants namespace
				token, err := getSecret(vaultExamplePath + "tenant-token.yaml")
				if err != nil {
					e2elog.Failf("failed to load tenant token from secret: %v", err)
				}
				_, err = c.CoreV1().Secrets(tenant).Create(context.TODO(), &token, metav1.CreateOptions{})
				if err != nil {
					e2elog.Failf("failed to create Secret with tenant token: %v", err)
				}

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, vaultTokensKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// delete the Secret of the Tenant
				err = c.CoreV1().Secrets(tenant).Delete(context.TODO(), token.Name, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete Secret with tenant token: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with VaultTenantSA KMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tenant-sa-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
				if err != nil {
					e2elog.Failf("failed to create ServiceAccount: %v", err)
				}
				defer deleteTenantServiceAccount(f.UniqueName)

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, vaultTenantSAKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume with SecretsMetadataKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("test RBD volume encryption with user secrets based SecretsMetadataKMS", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "user-ns-secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}

				// user provided namespace where secret will be created
				namespace := cephCSINamespace

				// create user Secret
				err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create user Secret: %v", err)
				}

				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					e2elog.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// delete user secret
				err = retryKubectlFile(namespace,
					kubectlDelete,
					vaultExamplePath+vaultUserSecret,
					deployTimeout,
					"--ignore-not-found=true")
				if err != nil {
					e2elog.Failf("failed to delete user Secret: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass: %v", err)
				}
			})

			By(
				"test RBD volume encryption with user secrets based SecretsMetadataKMS with tenant namespace",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "user-secrets-metadata-test",
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}

					// PVC creation namespace where secret will be created
					namespace := f.UniqueName

					// create user Secret
					err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create user Secret: %v", err)
					}

					err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
					if err != nil {
						e2elog.Failf("failed to validate encrypted pvc: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)

					// delete user secret
					err = retryKubectlFile(
						namespace,
						kubectlDelete,
						vaultExamplePath+vaultUserSecret,
						deployTimeout,
						"--ignore-not-found=true")
					if err != nil {
						e2elog.Failf("failed to delete user Secret: %v", err)
					}

					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass: %v", err)
					}
				})

			By(
				"create a PVC and Bind it to an app with journaling/exclusive-lock image-features and rbd-nbd mounter",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures": "layering,journaling,exclusive-lock",
							"mounter":       "rbd-nbd",
							"mapOptions":    nbdMapOptions,
						},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					err = validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to validate pvc and application binding with error %v", err)
					}
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
				},
			)

			By("create a PVC clone and bind it to an app", func() {
				// snapshot beta is only supported from v1.17+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					validatePVCSnapshot(
						defaultCloneCount,
						pvcPath,
						appPath,
						snapshotPath,
						pvcClonePath,
						appClonePath,
						noKMS,
						f)
				}
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				// pvc clone is only supported from v1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					validatePVCClone(
						defaultCloneCount,
						pvcPath,
						appPath,
						pvcSmartClonePath,
						appSmartClonePath,
						noKMS,
						noPVCValidation,
						f)
				}
			})

			By("create a thick-provisioned PVC-PVC clone and bind it to an app", func() {
				// pvc clone is only supported from v1.16+
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, map[string]string{
					"thickProvision": "true",
				}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCClone(1, pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, noKMS, isThickPVC, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create an encrypted PVC snapshot and restore it for an app with VaultKMS", func() {
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCSnapshot(1, pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath, vaultKMS, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create an encrypted PVC-PVC clone and bind it to an app", func() {
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCClone(1, pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, secretsMetadataKMS, isEncryptedPVC, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create an encrypted PVC-PVC clone and bind it to an app with VaultKMS", func() {
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					Skip("pvc clone is only supported from v1.16+")
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				validatePVCClone(1, pvcPath, appPath, pvcSmartClonePath, appSmartClonePath, vaultKMS, isEncryptedPVC, f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a block type PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
			})
			By("create a Block mode PVC-PVC clone and bind it to an app", func() {
				v, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					e2elog.Failf("failed to get server version with error %v", err)
				}
				// pvc clone is only supported from v1.16+
				if v.Major > "1" || (v.Major == "1" && v.Minor >= "16") {
					validatePVCClone(
						defaultCloneCount,
						rawPvcPath,
						rawAppPath,
						pvcBlockSmartClonePath,
						appBlockSmartClonePath,
						noKMS,
						noPVCValidation,
						f)
				}
			})
			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}

				}
				// validate created backend rbd images
				validateRBDImageCount(f, totalCount, defaultRBDPool)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application with error %v", err)
					}

				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to check data persist with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("Resize Filesystem PVC and check application directory size", func() {
				// Resize 0.3.0 is only supported from v1.15+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 15) {
					err := resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC %v", err)
					}

					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{"csi.storage.k8s.io/fstype": "xfs"},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				}
			})

			By("Resize Block PVC and check Device size", func() {
				// Block PVC resize is supported in kubernetes 1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					err := resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Failf("failed to resize block PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				}
			})

			By("Test unmount after nodeplugin restart", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to  load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				// delete rbd nodeplugin pods
				err = deletePodWithLabel("app=csi-rbdplugin", cephCSINamespace, false)
				if err != nil {
					e2elog.Failf("fail to delete pod with error %v", err)
				}
				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset pods with error %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create PVC in storageClass with volumeNamePrefix", func() {
				volumeNamePrefix := "foo-bar-"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"volumeNamePrefix": volumeNamePrefix},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				// list RBD images and check if one of them has the same prefix
				foundIt := false
				images, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					e2elog.Failf("failed to list rbd images with error %v", err)
				}
				for _, imgName := range images {
					fmt.Printf("Checking prefix on %s\n", imgName)
					if strings.HasPrefix(imgName, volumeNamePrefix) {
						foundIt = true

						break
					}
				}

				// clean up after ourselves
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to  delete PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				if !foundIt {
					e2elog.Failf("could not find image with prefix %s", volumeNamePrefix)
				}
			})

			By("validate RBD static FileSystem PVC", func() {
				err := validateRBDStaticPV(f, appPath, false, false)
				if err != nil {
					e2elog.Failf("failed to validate rbd static pv with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate RBD static Block PVC", func() {
				err := validateRBDStaticPV(f, rawAppPath, true, false)
				if err != nil {
					e2elog.Failf("failed to validate rbd block pv with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate failure of RBD static PVC without imageFeatures parameter", func() {
				err := validateRBDStaticPV(f, rawAppPath, true, true)
				if err != nil {
					e2elog.Failf("Validation of static PVC without imageFeatures parameter failed with err %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("validate mount options in app pod", func() {
				mountFlags := []string{"discard"}
				err := checkMountOptions(pvcPath, appPath, f, mountFlags)
				if err != nil {
					e2elog.Failf("failed to check mount options with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("creating an app with a PVC, using a topology constrained StorageClass", func() {
				By("checking node has required CSI topology labels set", func() {
					err := checkNodeHasLabel(f.ClientSet, nodeCSIRegionLabel, regionValue)
					if err != nil {
						e2elog.Failf("failed to check node label with error %v", err)
					}
					err = checkNodeHasLabel(f.ClientSet, nodeCSIZoneLabel, zoneValue)
					if err != nil {
						e2elog.Failf("failed to check node label with error %v", err)
					}
				})

				By("creating a StorageClass with delayed binding mode and CSI topology parameter")
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"domainSegments\":" +
					"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
					"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName,
					map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
					map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				By("creating an app using a PV from the delayed binding mode StorageClass")
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, 0)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				By("ensuring created PV has required node selector values populated")
				err = checkPVSelectorValuesForPVC(f, pvc)
				if err != nil {
					e2elog.Failf("failed to check pv selector values with error %v", err)
				}
				By("ensuring created PV has its image in the topology specific pool")
				err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					e2elog.Failf("failed to check image in pool with error %v", err)
				}

				By("ensuring created PV has its image journal in the topology specific pool")
				err = checkPVCImageJournalInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					e2elog.Failf("failed to check image journal with error %v", err)
				}

				By("ensuring created PV has its CSI journal in the CSI journal specific pool")
				err = checkPVCCSIJournalInPool(f, pvc, "replicapool")
				if err != nil {
					e2elog.Failf("failed to check csi journal in pool with error %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}

				By("checking if data pool parameter is honored", func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"dataPool\":\"" + rbdTopologyDataPool +
						"\",\"domainSegments\":" +
						"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
						"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName,
						map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
						map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					By("creating an app using a PV from the delayed binding mode StorageClass with a data pool")
					pvc, app, err = createPVCAndAppBinding(pvcPath, appPath, f, 0)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}

					By("ensuring created PV has its image in the topology specific pool")
					err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
					if err != nil {
						e2elog.Failf("failed to check  pvc image in pool with error %v", err)
					}

					By("ensuring created image has the right data pool parameter set")
					err = checkPVCDataPoolForImageInPool(f, pvc, rbdTopologyPool, rbdTopologyDataPool)
					if err != nil {
						e2elog.Failf("failed to check data pool for image with error %v", err)
					}

					// cleanup and undo changes made by the test
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application with error %v", err)
					}
				})

				// cleanup and undo changes made by the test
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			// Mount pvc to pod with invalid mount option,expected that
			// mounting will fail
			By("Mount pvc to pod with invalid mount option", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					map[string]string{rbdMountOptions: "debug,invalidOption"},
					nil,
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to  load PVC with error %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				// create an app and wait for 1 min for it to go to running state
				err = createApp(f.ClientSet, app, 1)
				if err == nil {
					e2elog.Failf("application should not go to running state due to invalid mount option")
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create rbd clones in different pool", func() {
				// snapshot beta is only supported from v1.17+
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					Skip("pvc restore is only supported from v1.17+")
				}
				clonePool := "clone-test"
				// create pool for clones
				err := createPool(f, clonePool)
				if err != nil {
					e2elog.Failf("failed to create pool %s with error %v", clonePool, err)
				}
				err = createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create snapshotclass with error %v", err)
				}
				cloneSC := "clone-storageclass"
				param := map[string]string{
					"pool": clonePool,
				}
				// create new storageclass with new pool
				err = createRBDStorageClass(f.ClientSet, f, cloneSC, nil, param, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validateCloneInDifferentPool(f, defaultRBDPool, cloneSC, clonePool)
				if err != nil {
					e2elog.Failf("failed to validate clones in different pool with error %v", err)
				}

				err = retryKubectlArgs(
					cephCSINamespace,
					kubectlDelete,
					deployTimeout,
					"sc",
					cloneSC,
					"--ignore-not-found=true")
				if err != nil {
					e2elog.Failf("failed to delete storageclass %s with error %v", cloneSC, err)
				}

				err = deleteResource(rbdExamplePath + "snapshotclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete snapshotclass with error %v", err)
				}
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, clonePool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash with error %v", clonePool, err)
				}
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash with error %v", defaultRBDPool, err)
				}

				err = deletePool(clonePool, false, f)
				if err != nil {
					e2elog.Failf("failed to delete pool %s with error %v", clonePool, err)
				}
			})

			By("create ROX PVC clone and mount it to multiple pods", func() {
				// snapshot beta is only supported from v1.17+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					err := createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()

					// create PVC and bind it to an app
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					pvc.Namespace = f.UniqueName
					app, err := loadApp(appPath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					app.Namespace = f.UniqueName
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					// delete pod as we should not create snapshot for in-use pvc
					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application with error %v", err)
					}

					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create snapshot with error %v", err)
					}
					// validate created backend rbd images
					// parent PVC + snapshot
					totalImages := 2
					validateRBDImageCount(f, totalImages, defaultRBDPool)
					pvcClone, err := loadPVC(pvcClonePath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					// create clone PVC as ROX
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
					err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}
					// validate created backend rbd images
					// parent pvc+ snapshot + clone
					totalImages = 3
					validateRBDImageCount(f, totalImages, defaultRBDPool)

					appClone, err := loadApp(appClonePath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}

					totalCount := 2
					appClone.Namespace = f.UniqueName
					appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

					// create PVC and app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						label := map[string]string{
							"app": name,
						}
						appClone.Labels = label
						appClone.Name = name
						err = createApp(f.ClientSet, appClone, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create application with error %v", err)
						}
					}

					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						opt := metav1.ListOptions{
							LabelSelector: fmt.Sprintf("app=%s", name),
						}

						filePath := appClone.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
						_, stdErr := execCommandInPodAndAllowFail(
							f,
							fmt.Sprintf("echo 'Hello World' > %s", filePath),
							appClone.Namespace,
							&opt)
						readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
						if !strings.Contains(stdErr, readOnlyErr) {
							e2elog.Failf(stdErr)
						}
					}

					// delete app
					for i := 0; i < totalCount; i++ {
						name := fmt.Sprintf("%s%d", f.UniqueName, i)
						appClone.Name = name
						err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to delete application with error %v", err)
						}
					}
					// delete PVC clone
					err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					// delete snapshot
					err = deleteSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete snapshot with error %v", err)
					}
					// delete parent pvc
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				}
			})

			By("validate PVC mounting if snapshot and parent PVC are deleted", func() {
				// snapshot beta is only supported from v1.17+
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					Skip("pvc restore is only supported from v1.17+")
				}
				err := createRBDSnapshotClass(f)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				pvc.Namespace = f.UniqueName
				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create snapshot with error %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				// delete parent PVC
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				// create clone PVC
				pvcClone.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC with error %v", err)
				}
				// validate created backend rbd images = snapshot + clone
				totalImages = 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)

				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete snapshot with error %v", err)
				}

				// validate created backend rbd images = clone
				totalImages = 1
				validateRBDImageCount(f, totalImages, defaultRBDPool)

				appClone, err := loadApp(appClonePath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

				// create application
				err = createApp(f.ClientSet, appClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create application with error %v", err)
				}

				err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application with error %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By(
				"validate PVC mounting if snapshot and parent PVC are deleted chained with depth 2",
				func() {
					// snapshot beta is only supported from v1.17+
					if !k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
						Skip("pvc restore is only supported from v1.17+")
					}
					snapChainDepth := 2

					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}

					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"encrypted":       "true",
							"encryptionKMSID": "vault-test",
						},
						deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}

					err = createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}

					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							e2elog.Failf("failed to delete storageclass with error %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							e2elog.Failf("failed to create storageclass with error %v", err)
						}
					}()

					// create PVC and bind it to an app
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					pvc.Namespace = f.UniqueName
					app, err := loadApp(appPath)
					if err != nil {
						e2elog.Failf("failed to load application with error %v", err)
					}
					app.Namespace = f.UniqueName
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC and application with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					for i := 0; i < snapChainDepth; i++ {
						var pvcClone *v1.PersistentVolumeClaim
						snap := getSnapshot(snapshotPath)
						snap.Name = fmt.Sprintf("%s-%d", snap.Name, i)
						snap.Namespace = f.UniqueName
						snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

						err = createSnapshot(&snap, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create snapshot with error %v", err)
						}
						// validate created backend rbd images
						// parent PVC + snapshot
						totalImages := 2
						validateRBDImageCount(f, totalImages, defaultRBDPool)
						pvcClone, err = loadPVC(pvcClonePath)
						if err != nil {
							e2elog.Failf("failed to load PVC with error %v", err)
						}

						// delete parent PVC
						err = deletePVCAndApp("", f, pvc, app)
						if err != nil {
							e2elog.Failf("failed to delete PVC and application with error %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 1, defaultRBDPool)

						// create clone PVC
						pvcClone.Name = fmt.Sprintf("%s-%d", pvcClone.Name, i)
						pvcClone.Namespace = f.UniqueName
						pvcClone.Spec.DataSource.Name = snap.Name
						err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create PVC with error %v", err)
						}
						// validate created backend rbd images = snapshot + clone
						totalImages = 2
						validateRBDImageCount(f, totalImages, defaultRBDPool)

						// delete snapshot
						err = deleteSnapshot(&snap, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to delete snapshot with error %v", err)
						}

						// validate created backend rbd images = clone
						totalImages = 1
						validateRBDImageCount(f, totalImages, defaultRBDPool)

						app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
						// create application
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							e2elog.Failf("failed to create application with error %v", err)
						}

						pvc = pvcClone
					}

					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete application with error %v", err)
					}
					// delete PVC clone
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
				})

			By("validate PVC Clone chained with depth 2", func() {
				// snapshot beta is only supported from v1.17+
				if !k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					Skip("pvc restore is only supported from v1.17+")
				}
				cloneChainDepth := 2

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}

				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "vault-test",
					},
					deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				defer func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete storageclass with error %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
				}()

				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				pvc.Namespace = f.UniqueName
				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				for i := 0; i < cloneChainDepth; i++ {
					var pvcClone *v1.PersistentVolumeClaim
					pvcClone, err = loadPVC(pvcSmartClonePath)
					if err != nil {
						e2elog.Failf("failed to load PVC with error %v", err)
					}

					// create clone PVC
					pvcClone.Name = fmt.Sprintf("%s-%d", pvcClone.Name, i)
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.DataSource.Name = pvc.Name
					err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}

					// delete parent PVC
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC and application with error %v", err)
					}

					app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
					// create application
					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application with error %v", err)
					}

					pvc = pvcClone
				}

				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete application with error %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to delete PVC with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("ensuring all operations will work within a rados namespace", func() {
				updateConfigMap := func(radosNS string) {
					radosNamespace = radosNS
					err := deleteConfigMap(rbdDirPath)
					if err != nil {
						e2elog.Failf("failed to delete configmap with Error: %v", err)
					}
					err = createConfigMap(rbdDirPath, f.ClientSet, f)
					if err != nil {
						e2elog.Failf("failed to create configmap with error %v", err)
					}
					err = createRadosNamespace(f)
					if err != nil {
						e2elog.Failf("failed to create rados namespace with error %v", err)
					}
					// delete csi pods
					err = deletePodWithLabel("app in (ceph-csi-rbd, csi-rbdplugin, csi-rbdplugin-provisioner)",
						cephCSINamespace, false)
					if err != nil {
						e2elog.Failf("failed to delete pods with labels with error %v", err)
					}
					// wait for csi pods to come up
					err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("timeout waiting for daemonset pods with error %v", err)
					}
					err = waitForDeploymentComplete(rbdDeploymentName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						e2elog.Failf("timeout waiting for deployment to be in running state with error %v", err)
					}
				}

				updateConfigMap("e2e-ns")
				// create rbd provisioner secret
				key, err := createCephUser(
					f,
					keyringRBDNamespaceProvisionerUsername,
					rbdProvisionerCaps(defaultRBDPool, radosNamespace),
				)
				if err != nil {
					e2elog.Failf("failed to create user %s with error %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceProvisionerSecretName, keyringRBDNamespaceProvisionerUsername, key)
				if err != nil {
					e2elog.Failf("failed to create provisioner secret with error %v", err)
				}
				// create rbd plugin secret
				key, err = createCephUser(
					f,
					keyringRBDNamespaceNodePluginUsername,
					rbdNodePluginCaps(defaultRBDPool, radosNamespace))
				if err != nil {
					e2elog.Failf("failed to create user %s with error %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceNodePluginSecretName, keyringRBDNamespaceNodePluginUsername, key)
				if err != nil {
					e2elog.Failf("failed to create node secret with error %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				param := make(map[string]string)
				// override existing secrets
				param["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName
				param["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName
				param["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
				param["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, param, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				err = validateImageOwner(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate owner of pvc with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// Create a PVC and bind it to an app within the namesapce
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}

				// Resize Block PVC and check Device size within the namespace
				// Block PVC resize is supported in kubernetes 1.16+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 16) {
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						e2elog.Failf("failed to resize block PVC with error %v", err)
					}
				}

				// Resize Filesystem PVC and check application directory size
				// Resize 0.3.0 is only supported from v1.15+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 15) {
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						e2elog.Failf("failed to resize filesystem PVC %v", err)
					}
				}

				// Create a PVC clone and bind it to an app within the namespace
				// snapshot beta is only supported from v1.17+
				if k8sVersionGreaterEquals(f.ClientSet, 1, 17) {
					err = createRBDSnapshotClass(f)
					if err != nil {
						e2elog.Failf("failed to create storageclass with error %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()

					pvc, pvcErr := loadPVC(pvcPath)
					if pvcErr != nil {
						e2elog.Failf("failed to load PVC with error %v", pvcErr)
					}

					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC with error %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)

					snap := getSnapshot(snapshotPath)
					snap.Namespace = f.UniqueName
					snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
					err = createSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create snapshot with error %v", err)
					}
					validateRBDImageCount(f, 2, defaultRBDPool)

					err = validatePVCAndAppBinding(pvcClonePath, appClonePath, f)
					if err != nil {
						e2elog.Failf("failed to validate pvc and application binding with error %v", err)
					}
					err = deleteSnapshot(&snap, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete snapshot with error %v", err)
					}
					// as snapshot is deleted the image count should be one
					validateRBDImageCount(f, 1, defaultRBDPool)

					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to delete PVC with error %v", err)
					}
					validateRBDImageCount(f, 0, defaultRBDPool)

					err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to validate rbd images in pool %s trash with error %v", rbdOptions(defaultRBDPool), err)
					}
				}

				// delete RBD provisioner secret
				err = deleteCephUser(f, keyringRBDNamespaceProvisionerUsername)
				if err != nil {
					e2elog.Failf("failed to delete user %s with error %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = c.CoreV1().
					Secrets(cephCSINamespace).
					Delete(context.TODO(), rbdNamespaceProvisionerSecretName, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete provisioner secret with error %v", err)
				}
				// delete RBD plugin secret
				err = deleteCephUser(f, keyringRBDNamespaceNodePluginUsername)
				if err != nil {
					e2elog.Failf("failed to delete user %s with error %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = c.CoreV1().
					Secrets(cephCSINamespace).
					Delete(context.TODO(), rbdNamespaceNodePluginSecretName, metav1.DeleteOptions{})
				if err != nil {
					e2elog.Failf("failed to delete node secret with error %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				updateConfigMap("")
			})

			By("Mount pvc as readonly in pod", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error %v", err)
				}

				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application with error %v", err)
				}

				app.Namespace = f.UniqueName
				label := map[string]string{
					"app": app.Name,
				}
				app.Labels = label
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				app.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}

				filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				_, stdErr := execCommandInPodAndAllowFail(
					f,
					fmt.Sprintf("echo 'Hello World' > %s", filePath),
					app.Namespace,
					&opt)
				readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
				if !strings.Contains(stdErr, readOnlyErr) {
					e2elog.Failf(stdErr)
				}

				// delete PVC and app
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC and application with error %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
			})

			By("create a thick-provisioned PVC", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, map[string]string{
					"thickProvision": "true",
				}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}

				pvc, err := loadPVC(rawPvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC with error: %v", err)
				}

				pvcSizes := []string{
					// original value from the yaml file (100MB)
					"100Mi",
					// half the size (50MB), is not stripe-size roundable
					"50Mi",
				}

				for _, pvcSize := range pvcSizes {
					err = validateThickPVC(f, pvc, pvcSize)
					if err != nil {
						e2elog.Failf("validating thick-provisioning failed: %v", err)
					}
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("create a PVC and Bind it to an app for mapped rbd image with options", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, map[string]string{
					"mapOptions":   "lock_on_read,queue_depth=1024",
					"unmapOptions": "force",
				}, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate pvc and application binding with error %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass with error %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass with error %v", err)
				}
			})

			By("validate the functionality of controller", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass : %v", err)
				}
				scParams := map[string]string{
					"volumeNamePrefix": "test-",
				}
				err = validateController(f,
					pvcPath, appPath, rbdExamplePath+"storageclass.yaml",
					nil,
					scParams)
				if err != nil {
					e2elog.Failf("failed to validate controller : %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass : %v", err)
				}
			})

			By("validate the functionality of controller with encryption and thick-provisioning", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete storageclass : %v", err)
				}
				scParams := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "user-secrets-metadata-test",
					"thickProvision":  "true",
				}

				// PVC creation namespace where secret will be created
				namespace := f.UniqueName

				// create user Secret
				err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create user Secret: %v", err)
				}

				err = validateController(f,
					pvcPath, appPath, rbdExamplePath+"storageclass.yaml",
					nil,
					scParams)
				if err != nil {
					e2elog.Failf("failed to validate controller : %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				// delete user secret
				err = retryKubectlFile(namespace,
					kubectlDelete,
					vaultExamplePath+vaultUserSecret,
					deployTimeout,
					"--ignore-not-found=true")
				if err != nil {
					e2elog.Failf("failed to delete user Secret: %v", err)
				}

				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					e2elog.Failf("failed to create storageclass : %v", err)
				}
			})

			By("validate stale images in trash", func() {
				err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to validate rbd images in pool %s trash with error %v", defaultRBDPool, err)
				}
			})

			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, false, f)
				if err != nil {
					e2elog.Failf("failed to delete PVC when pool not found with error %v", err)
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
