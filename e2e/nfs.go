/*
Copyright 2022 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo" // nolint
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	nfsProvisioner     = "csi-nfsplugin-provisioner.yaml"
	nfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	nfsProvisionerPSP  = "csi-provisioner-psp.yaml"
	nfsNodePlugin      = "csi-nfsplugin.yaml"
	nfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	nfsNodePluginPSP   = "csi-nodeplugin-psp.yaml"
	nfsRookCephNFS     = "rook-nfs.yaml"
	nfsDeploymentName  = "csi-nfsplugin-provisioner"
	nfsDeamonSetName   = "csi-nfs-node"
	nfsDirPath         = "../deploy/nfs/kubernetes/"
	nfsExamplePath     = examplePath + "nfs/"
	nfsPoolName        = ".nfs"

	// FIXME: some tests change the subvolumegroup to "e2e".
	defaultSubvolumegroup = "csi"
)

func deployNFSPlugin(f *framework.Framework) {
	// delete objects deployed by rook

	err := deleteResource(nfsDirPath + nfsProvisionerRBAC)
	if err != nil {
		e2elog.Failf("failed to delete provisioner rbac %s: %v", nfsDirPath+nfsProvisionerRBAC, err)
	}

	err = deleteResource(nfsDirPath + nfsNodePluginRBAC)
	if err != nil {
		e2elog.Failf("failed to delete nodeplugin rbac %s: %v", nfsDirPath+nfsNodePluginRBAC, err)
	}

	// the pool should not be deleted, as it may contain configurations
	// from non-e2e related CephNFS objects
	err = createPool(f, nfsPoolName)
	if err != nil {
		e2elog.Failf("failed to create pool for NFS config %q: %v", nfsPoolName, err)
	}

	createORDeleteNFSResources(f, kubectlCreate)
}

func deleteNFSPlugin() {
	createORDeleteNFSResources(nil, kubectlDelete)
}

func createORDeleteNFSResources(f *framework.Framework, action kubectlAction) {
	resources := []ResourceDeployer{
		&yamlResource{
			filename:     nfsDirPath + csiDriverObject,
			allowMissing: true,
		},
		&yamlResource{
			filename:     examplePath + cephConfconfigMap,
			allowMissing: true,
		},
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsProvisionerRBAC,
			namespace: cephCSINamespace,
		},
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsProvisionerPSP,
			namespace: cephCSINamespace,
		},
		&yamlResourceNamespaced{
			filename:   nfsDirPath + nfsProvisioner,
			namespace:  cephCSINamespace,
			oneReplica: true,
		},
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsNodePluginRBAC,
			namespace: cephCSINamespace,
		},
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsNodePluginPSP,
			namespace: cephCSINamespace,
		},
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsNodePlugin,
			namespace: cephCSINamespace,
		},
		&rookNFSResource{
			f:           f,
			modules:     []string{"rook", "nfs"},
			orchBackend: "rook",
		},
		&yamlResourceNamespaced{
			filename:  nfsExamplePath + nfsRookCephNFS,
			namespace: rookNamespace,
		},
	}

	for _, r := range resources {
		err := r.Do(action)
		if err != nil {
			e2elog.Failf("failed to %s resource: %v", action, err)
		}
	}
}

func createNFSStorageClass(
	c clientset.Interface,
	f *framework.Framework,
	enablePool bool,
	params map[string]string) error {
	scPath := fmt.Sprintf("%s/%s", nfsExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return err
	}
	sc.Parameters["nfsCluster"] = "my-nfs"
	sc.Parameters["server"] = "rook-ceph-nfs-my-nfs-a." + rookNamespace + ".svc.cluster.local"

	// standard CephFS parameters
	sc.Parameters["fsName"] = fileSystemName
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = cephFSNodePluginSecretName

	if enablePool {
		sc.Parameters["pool"] = "myfs-replicated"
	}

	// overload any parameters that were passed
	if params == nil {
		// create an empty params, so that params["clusterID"] below
		// does not panic
		params = map[string]string{}
	}
	for param, value := range params {
		sc.Parameters[param] = value
	}

	// fetch and set fsID from the cluster if not set in params
	if _, found := params["clusterID"]; !found {
		var fsID string
		fsID, err = getClusterID(f)
		if err != nil {
			return fmt.Errorf("failed to get clusterID: %w", err)
		}
		sc.Parameters["clusterID"] = fsID
	}

	sc.Provisioner = nfsDriverName

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err = c.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
		if err != nil {
			e2elog.Logf("error creating StorageClass %q: %v", sc.Name, err)
			if apierrs.IsAlreadyExists(err) {
				return true, nil
			}
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to create StorageClass %q: %w", sc.Name, err)
		}

		return true, nil
	})
}

// unmountNFSVolume unmounts a NFS volume mounted on a pod.
func unmountNFSVolume(f *framework.Framework, appName, pvcName string) error {
	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		e2elog.Logf("Error occurred getting pod %s in namespace %s", appName, f.UniqueName)

		return fmt.Errorf("failed to get pod: %w", err)
	}
	pvc, err := f.ClientSet.CoreV1().
		PersistentVolumeClaims(f.UniqueName).
		Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		e2elog.Logf("Error occurred getting PVC %s in namespace %s", pvcName, f.UniqueName)

		return fmt.Errorf("failed to get pvc: %w", err)
	}
	cmd := fmt.Sprintf(
		"umount /var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvc.Spec.VolumeName)
	stdErr, err := execCommandInDaemonsetPod(
		f,
		cmd,
		nfsDeamonSetName,
		pod.Spec.NodeName,
		"nfs", // name of the container
		cephCSINamespace)
	if stdErr != "" {
		e2elog.Logf("StdErr occurred: %s", stdErr)
	}

	return err
}

var _ = Describe("nfs", func() {
	f := framework.NewDefaultFramework("nfs")
	var c clientset.Interface
	// deploy CephFS CSI
	BeforeEach(func() {
		if !testNFS || upgradeTesting || helmTest {
			Skip("Skipping NFS E2E")
		}
		c = f.ClientSet
		if deployNFS {
			if cephCSINamespace != defaultNs {
				err := createNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to create namespace %s: %v", cephCSINamespace, err)
				}
			}
			deployNFSPlugin(f)
		}

		// cephfs testing might have changed the default subvolumegroup
		subvolumegroup = defaultSubvolumegroup
		err := createConfigMap(nfsDirPath, f.ClientSet, f)
		if err != nil {
			e2elog.Failf("failed to create configmap: %v", err)
		}
		// create nfs provisioner secret
		key, err := createCephUser(f, keyringCephFSProvisionerUsername, cephFSProvisionerCaps())
		if err != nil {
			e2elog.Failf("failed to create user %s: %v", keyringCephFSProvisionerUsername, err)
		}
		err = createCephfsSecret(f, cephFSProvisionerSecretName, keyringCephFSProvisionerUsername, key)
		if err != nil {
			e2elog.Failf("failed to create provisioner secret: %v", err)
		}
		// create nfs plugin secret
		key, err = createCephUser(f, keyringCephFSNodePluginUsername, cephFSNodePluginCaps())
		if err != nil {
			e2elog.Failf("failed to create user %s: %v", keyringCephFSNodePluginUsername, err)
		}
		err = createCephfsSecret(f, cephFSNodePluginSecretName, keyringCephFSNodePluginUsername, key)
		if err != nil {
			e2elog.Failf("failed to create node secret: %v", err)
		}
	})

	AfterEach(func() {
		if !testNFS || upgradeTesting {
			Skip("Skipping NFS E2E")
		}
		if CurrentGinkgoTestDescription().Failed {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-nfs", c)
			// log provisioner
			logsCSIPods("app=csi-nfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-nfs-node", c)

			// log all details from the namespace where Ceph-CSI is deployed
			framework.DumpAllNamespaceInfo(c, cephCSINamespace)
		}
		err := deleteConfigMap(nfsDirPath)
		if err != nil {
			e2elog.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			e2elog.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(nfsExamplePath + "storageclass.yaml")
		if err != nil {
			e2elog.Failf("failed to delete storageclass: %v", err)
		}
		if deployNFS {
			deleteNFSPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					e2elog.Failf("failed to delete namespace %s: %v", cephCSINamespace, err)
				}
			}
		}
	})

	Context("Test NFS CSI", func() {
		if !testNFS {
			return
		}

		It("Test NFS CSI", func() {
			pvcPath := nfsExamplePath + "pvc.yaml"
			appPath := nfsExamplePath + "pod.yaml"
			appRWOPPath := nfsExamplePath + "pod-rwop.yaml"
			pvcRWOPPath := nfsExamplePath + "pvc-rwop.yaml"
			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(f.ClientSet, nfsDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for deployment %s: %v", nfsDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(nfsDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					e2elog.Failf("timeout waiting for daemonset %s: %v", nfsDeamonSetName, err)
				}
			})

			By("verify RWOP volume support", func() {
				if k8sVersionGreaterEquals(f.ClientSet, 1, 22) {
					err := createNFSStorageClass(f.ClientSet, f, false, nil)
					if err != nil {
						e2elog.Failf("failed to create CephFS storageclass: %v", err)
					}
					pvc, err := loadPVC(pvcRWOPPath)
					if err != nil {
						e2elog.Failf("failed to load PVC: %v", err)
					}
					pvc.Namespace = f.UniqueName

					// create application
					app, err := loadApp(appRWOPPath)
					if err != nil {
						e2elog.Failf("failed to load application: %v", err)
					}
					app.Namespace = f.UniqueName
					baseAppName := app.Name

					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						if rwopMayFail(err) {
							e2elog.Logf("RWOP is not supported: %v", err)

							return
						}
						e2elog.Failf("failed to create PVC: %v", err)
					}
					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create application: %v", err)
					}
					validateSubvolumeCount(f, 1, fileSystemName, defaultSubvolumegroup)

					err = validateRWOPPodCreation(f, pvc, app, baseAppName)
					if err != nil {
						e2elog.Failf("failed to validate RWOP pod creation: %v", err)
					}
					validateSubvolumeCount(f, 0, fileSystemName, defaultSubvolumegroup)
					err = deleteResource(nfsExamplePath + "storageclass.yaml")
					if err != nil {
						e2elog.Failf("failed to delete CephFS storageclass: %v", err)
					}
				}
			})

			By("create a storageclass with pool and a PVC then bind it to an app", func() {
				err := createNFSStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application binding: %v", err)
				}
				err = deleteResource(nfsExamplePath + "storageclass.yaml")
				if err != nil {
					e2elog.Failf("failed to delete CephFS storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app", func() {
				err := createNFSStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					e2elog.Failf("failed to create CephFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to validate CephFS pvc and application  binding: %v", err)
				}
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					e2elog.Failf("failed to validate normal user CephFS pvc and application binding: %v", err)
				}
			})

			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						e2elog.Failf("failed to create PVC or application: %v", err)
					}
					err = validateSubvolumePath(f, pvc.Name, pvc.Namespace, fileSystemName, defaultSubvolumegroup)
					if err != nil {
						e2elog.Failf("failed to validate subvolumePath: %v", err)
					}
				}

				validateSubvolumeCount(f, totalCount, fileSystemName, defaultSubvolumegroup)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						e2elog.Failf("failed to delete PVC or application: %v", err)
					}

				}
				validateSubvolumeCount(f, 0, fileSystemName, defaultSubvolumegroup)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					e2elog.Failf("failed to check data persist in pvc: %v", err)
				}
			})

			By("Create PVC, bind it to an app, unmount volume and check app deletion", func() {
				// TODO: update nfs node-plugin that has kubernetes-csi/csi-driver-nfs#319
				if true {
					e2elog.Logf("skipping test, needs kubernetes-csi/csi-driver-nfs#319")

					return
				}

				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					e2elog.Failf("failed to create PVC or application: %v", err)
				}

				err = unmountNFSVolume(f, app.Name, pvc.Name)
				if err != nil {
					e2elog.Failf("failed to unmount volume: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}
			})

			By("Mount pvc as readonly in pod", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					e2elog.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					e2elog.Failf("failed to load application: %v", err)
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
					e2elog.Failf("failed to create PVC or application: %v", err)
				}

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
					e2elog.Failf("failed to delete PVC or application: %v", err)
				}
			})

			// delete nfs provisioner secret
			err := deleteCephUser(f, keyringCephFSProvisionerUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s: %v", keyringCephFSProvisionerUsername, err)
			}
			// delete nfs plugin secret
			err = deleteCephUser(f, keyringCephFSNodePluginUsername)
			if err != nil {
				e2elog.Failf("failed to delete user %s: %v", keyringCephFSNodePluginUsername, err)
			}
		})
	})
})
