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
	"sync"
	"time"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2" //nolint:golint // e2e uses By() and other Ginkgo functions
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edebug "k8s.io/kubernetes/test/e2e/framework/debug"
	"k8s.io/pod-security-admission/api"
)

var (
	nfsProvisioner     = "csi-nfsplugin-provisioner.yaml"
	nfsProvisionerRBAC = "csi-provisioner-rbac.yaml"
	nfsNodePlugin      = "csi-nfsplugin.yaml"
	nfsNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	nfsRookCephNFS     = "rook-nfs.yaml"
	nfsDeploymentName  = "csi-nfsplugin-provisioner"
	nfsDeamonSetName   = "csi-nfsplugin"
	nfsContainerName   = "csi-nfsplugin"
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
		framework.Failf("failed to delete provisioner rbac %s: %v", nfsDirPath+nfsProvisionerRBAC, err)
	}

	err = deleteResource(nfsDirPath + nfsNodePluginRBAC)
	if err != nil {
		framework.Failf("failed to delete nodeplugin rbac %s: %v", nfsDirPath+nfsNodePluginRBAC, err)
	}

	// the pool should not be deleted, as it may contain configurations
	// from non-e2e related CephNFS objects
	err = createPool(f, nfsPoolName)
	if err != nil {
		framework.Failf("failed to create pool for NFS config %q: %v", nfsPoolName, err)
	}

	createORDeleteNFSResources(f, kubectlCreate)
}

func deleteNFSPlugin() {
	createORDeleteNFSResources(nil, kubectlDelete)
}

func createORDeleteNFSResources(f *framework.Framework, action kubectlAction) {
	cephConfigFile := getConfigFile(cephConfconfigMap, deployPath, examplePath)
	resources := []ResourceDeployer{
		// shared resources
		&yamlResource{
			filename:     nfsDirPath + csiDriverObject,
			allowMissing: true,
		},
		&yamlResource{
			filename:     cephConfigFile,
			allowMissing: true,
		},
		// dependencies for provisioner
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsProvisionerRBAC,
			namespace: cephCSINamespace,
		},
		// the provisioner itself
		&yamlResourceNamespaced{
			filename:   nfsDirPath + nfsProvisioner,
			namespace:  cephCSINamespace,
			oneReplica: true,
		},
		// dependencies for the node-plugin
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsNodePluginRBAC,
			namespace: cephCSINamespace,
		},
		// the node-plugin itself
		&yamlResourceNamespaced{
			filename:  nfsDirPath + nfsNodePlugin,
			namespace: cephCSINamespace,
		},
		// NFS-export management by Rook
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
			framework.Failf("failed to %s resource: %v", action, err)
		}
	}
}

func createNFSStorageClass(
	c clientset.Interface,
	f *framework.Framework,
	enablePool bool,
	params map[string]string,
) error {
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

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err = c.StorageV1().StorageClasses().Create(ctx, &sc, metav1.CreateOptions{})
		if err != nil {
			framework.Logf("error creating StorageClass %q: %v", sc.Name, err)
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
		framework.Logf("Error occurred getting pod %s in namespace %s", appName, f.UniqueName)

		return fmt.Errorf("failed to get pod: %w", err)
	}
	pvc, err := f.ClientSet.CoreV1().
		PersistentVolumeClaims(f.UniqueName).
		Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		framework.Logf("Error occurred getting PVC %s in namespace %s", pvcName, f.UniqueName)

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
		"csi-nfsplugin", // name of the container
		cephCSINamespace)
	if stdErr != "" {
		framework.Logf("StdErr occurred: %s", stdErr)
	}

	return err
}

var _ = Describe("nfs", func() {
	f := framework.NewDefaultFramework("nfs")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged
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
					framework.Failf("failed to create namespace %s: %v", cephCSINamespace, err)
				}
			}
			deployNFSPlugin(f)
		}

		// cephfs testing might have changed the default subvolumegroup
		subvolumegroup = defaultSubvolumegroup
		err := createConfigMap(nfsDirPath, f.ClientSet, f)
		if err != nil {
			framework.Failf("failed to create configmap: %v", err)
		}
		// create nfs provisioner secret
		key, err := createCephUser(f, keyringCephFSProvisionerUsername, cephFSProvisionerCaps())
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringCephFSProvisionerUsername, err)
		}
		err = createCephfsSecret(f, cephFSProvisionerSecretName, keyringCephFSProvisionerUsername, key)
		if err != nil {
			framework.Failf("failed to create provisioner secret: %v", err)
		}
		// create nfs plugin secret
		key, err = createCephUser(f, keyringCephFSNodePluginUsername, cephFSNodePluginCaps())
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringCephFSNodePluginUsername, err)
		}
		err = createCephfsSecret(f, cephFSNodePluginSecretName, keyringCephFSNodePluginUsername, key)
		if err != nil {
			framework.Failf("failed to create node secret: %v", err)
		}
	})

	AfterEach(func() {
		if !testNFS || upgradeTesting {
			Skip("Skipping NFS E2E")
		}
		if CurrentSpecReport().Failed() {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-nfs", c)
			// log provisioner
			logsCSIPods("app=csi-nfsplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-nfsplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), c, cephCSINamespace)
		}
		err := deleteConfigMap(nfsDirPath)
		if err != nil {
			framework.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), cephFSNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(nfsExamplePath + "storageclass.yaml")
		if err != nil {
			framework.Failf("failed to delete storageclass: %v", err)
		}
		if deployNFS {
			deleteNFSPlugin()
			if cephCSINamespace != defaultNs {
				err := deleteNamespace(c, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to delete namespace %s: %v", cephCSINamespace, err)
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
			pvcSmartClonePath := nfsExamplePath + "pvc-clone.yaml"
			appSmartClonePath := nfsExamplePath + "pod-clone.yaml"
			pvcClonePath := nfsExamplePath + "pvc-restore.yaml"
			appClonePath := nfsExamplePath + "pod-restore.yaml"
			snapshotPath := nfsExamplePath + "snapshot.yaml"

			metadataPool, getErr := getCephFSMetadataPoolName(f, fileSystemName)
			if getErr != nil {
				framework.Failf("failed getting cephFS metadata pool name: %v", getErr)
			}

			By("checking provisioner deployment is running", func() {
				err := waitForDeploymentComplete(f.ClientSet, nfsDeploymentName, cephCSINamespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment %s: %v", nfsDeploymentName, err)
				}
			})

			By("checking nodeplugin deamonset pods are running", func() {
				err := waitForDaemonSets(nfsDeamonSetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for daemonset %s: %v", nfsDeamonSetName, err)
				}
			})

			By("verify mountOptions support", func() {
				err := createNFSStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create NFS storageclass: %v", err)
				}

				err = verifySeLinuxMountOption(f, pvcPath, appPath,
					nfsDeamonSetName, nfsContainerName, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to verify mount options: %v", err)
				}

				err = deleteResource(nfsExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete NFS storageclass: %v", err)
				}
			})

			By("verify RWOP volume support", func() {
				err := createNFSStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create NFS storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcRWOPPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				// create application
				app, err := loadApp(appRWOPPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				baseAppName := app.Name

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					if rwopMayFail(err) {
						framework.Logf("RWOP is not supported: %v", err)

						return
					}
					framework.Failf("failed to create PVC: %v", err)
				}
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
				validateSubvolumeCount(f, 1, fileSystemName, defaultSubvolumegroup)

				err = validateRWOPPodCreation(f, pvc, app, baseAppName)
				if err != nil {
					framework.Failf("failed to validate RWOP pod creation: %v", err)
				}
				validateSubvolumeCount(f, 0, fileSystemName, defaultSubvolumegroup)
				err = deleteResource(nfsExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete NFS storageclass: %v", err)
				}
			})

			By("create a storageclass with pool and a PVC then bind it to an app", func() {
				err := createNFSStorageClass(f.ClientSet, f, true, nil)
				if err != nil {
					framework.Failf("failed to create NFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate NFS pvc and application binding: %v", err)
				}
				err = deleteResource(nfsExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete NFS storageclass: %v", err)
				}
			})

			By("create a storageclass with sys,krb5i security and a PVC then bind it to an app", func() {
				err := createNFSStorageClass(f.ClientSet, f, false, map[string]string{
					"secTypes": "sys,krb5i",
				})
				if err != nil {
					framework.Failf("failed to create NFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate NFS pvc and application binding: %v", err)
				}
				err = deleteResource(nfsExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete NFS storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app", func() {
				err := createNFSStorageClass(f.ClientSet, f, false, nil)
				if err != nil {
					framework.Failf("failed to create NFS storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate NFS pvc and application  binding: %v", err)
				}
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					framework.Failf("failed to validate normal user NFS pvc and application binding: %v", err)
				}
			})

			By("create/delete multiple PVCs and Apps", func() {
				totalCount := 2
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				// create PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC or application: %v", err)
					}
					err = validateSubvolumePath(f, pvc.Name, pvc.Namespace, fileSystemName, defaultSubvolumegroup)
					if err != nil {
						framework.Failf("failed to validate subvolumePath: %v", err)
					}
				}

				validateSubvolumeCount(f, totalCount, fileSystemName, defaultSubvolumegroup)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err = deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						framework.Failf("failed to delete PVC or application: %v", err)
					}

				}
				validateSubvolumeCount(f, 0, fileSystemName, defaultSubvolumegroup)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to check data persist in pvc: %v", err)
				}
			})

			By("Create PVC, bind it to an app, unmount volume and check app deletion", func() {
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC or application: %v", err)
				}

				err = unmountNFSVolume(f, app.Name, pvc.Name)
				if err != nil {
					framework.Failf("failed to unmount volume: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}
			})

			By("Mount pvc as readonly in pod", func() {
				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
					framework.Failf("failed to create PVC or application: %v", err)
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
					framework.Failf(stdErr)
				}

				// delete PVC and app
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}
			})

			// delete nfs provisioner secret
			err := deleteCephUser(f, keyringCephFSProvisionerUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringCephFSProvisionerUsername, err)
			}
			// delete nfs plugin secret
			err = deleteCephUser(f, keyringCephFSNodePluginUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringCephFSNodePluginUsername, err)
			}

			By("Resize PVC and check application directory size", func() {
				err := resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize PVC: %v", err)
				}
			})

			By("create a PVC clone and bind it to an app", func() {
				var wg sync.WaitGroup
				totalCount := 3
				wgErrs := make([]error, totalCount)
				chErrs := make([]error, totalCount)
				// totalSubvolumes represents the subvolumes in backend
				// always totalCount+parentPVC
				totalSubvolumes := totalCount + 1
				wg.Add(totalCount)
				err := createNFSSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to delete NFS snapshotclass: %v", err)
				}
				defer func() {
					err = deleteNFSSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}

				app.Namespace = f.UniqueName
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				label := make(map[string]string)
				label[appKey] = appLabel
				app.Labels = label
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				checkSum, err := writeDataAndCalChecksum(app, &opt, f)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}

				_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
				if err != nil {
					framework.Failf("failed to get PV object for %s: %v", pvc.Name, err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				// create snapshot
				for i := 0; i < totalCount; i++ {
					go func(n int, s snapapi.VolumeSnapshot) {
						s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = createSnapshot(&s, deployTimeout)
						wg.Done()
					}(i, snap)
				}
				wg.Wait()

				failed := 0
				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to create snapshot (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("creating snapshots failed, %d errors were logged", failed)
				}
				validateCephFSSnapshotCount(f, totalCount, defaultSubvolumegroup, pv)

				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				appClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)
				appClone.Labels = label

				// create multiple PVC from same snapshot
				wg.Add(totalCount)
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
						if wgErrs[n] == nil {
							err = validateSubvolumePath(f, p.Name, p.Namespace, fileSystemName, subvolumegroup)
							if err != nil {
								wgErrs[n] = err
							}
							filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
							var checkSumClone string
							framework.Logf("Calculating checksum clone for filepath %s", filePath)
							checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
							framework.Logf("checksum for clone is %s", checkSumClone)
							if chErrs[n] != nil {
								framework.Logf("Failed calculating checksum clone %s", chErrs[n])
							}
							if checkSumClone != checkSum {
								framework.Logf("checksum didn't match. checksum=%s and checksumclone=%s", checkSum, checkSumClone)
							}
						}
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to create PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("creating PVCs and apps failed, %d errors were logged", failed)
				}

				for i, err := range chErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("calculating checksum failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalSubvolumes, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, snapsType)

				wg.Add(totalCount)
				// delete clone and app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to delete PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				parentPVCCount := totalSubvolumes - totalCount
				validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
				validateOmapCount(f, parentPVCCount, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, snapsType)
				// create clones from different snapshots and bind it to an app
				wg.Add(totalCount)
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
						if wgErrs[n] == nil {
							err = validateSubvolumePath(f, p.Name, p.Namespace, fileSystemName, subvolumegroup)
							if err != nil {
								wgErrs[n] = err
							}
							filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
							var checkSumClone string
							framework.Logf("Calculating checksum clone for filepath %s", filePath)
							checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
							framework.Logf("checksum for clone is %s", checkSumClone)
							if chErrs[n] != nil {
								framework.Logf("Failed calculating checksum clone %s", chErrs[n])
							}
							if checkSumClone != checkSum {
								framework.Logf("checksum didn't match. checksum=%s and checksumclone=%s", checkSum, checkSumClone)
							}
						}
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to create PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("creating PVCs and apps failed, %d errors were logged", failed)
				}

				for i, err := range chErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("calculating checksum failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalSubvolumes, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, totalCount, cephfsType, metadataPool, snapsType)

				wg.Add(totalCount)
				// delete snapshot
				for i := 0; i < totalCount; i++ {
					go func(n int, s snapapi.VolumeSnapshot) {
						s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = deleteSnapshot(&s, deployTimeout)
						wg.Done()
					}(i, snap)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to delete snapshot (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting snapshots failed, %d errors were logged", failed)
				}

				validateCephFSSnapshotCount(f, 0, defaultSubvolumegroup, pv)

				wg.Add(totalCount)
				// delete clone and app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to delete PVC and app (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, parentPVCCount, fileSystemName, subvolumegroup)
				validateOmapCount(f, parentPVCCount, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, 0, cephfsType, metadataPool, snapsType)
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
				validateOmapCount(f, 0, cephfsType, metadataPool, snapsType)
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				var wg sync.WaitGroup
				totalCount := 3
				wgErrs := make([]error, totalCount)
				chErrs := make([]error, totalCount)
				// totalSubvolumes represents the subvolumes in backend
				// always totalCount+parentPVC
				totalSubvolumes := totalCount + 1
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
				label := make(map[string]string)
				label[appKey] = appLabel
				app.Labels = label
				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
				}
				checkSum, err := writeDataAndCalChecksum(app, &opt, f)
				if err != nil {
					framework.Failf("failed to calculate checksum: %v", err)
				}

				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				appClone, err := loadApp(appSmartClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Labels = label
				wg.Add(totalCount)
				// create clone and bind it to an app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
						if wgErrs[n] == nil {
							filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
							var checkSumClone string
							framework.Logf("Calculating checksum clone for filepath %s", filePath)
							checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
							framework.Logf("checksum for clone is %s", checkSumClone)
							if chErrs[n] != nil {
								framework.Logf("Failed calculating checksum clone %s", chErrs[n])
							}
							if checkSumClone != checkSum {
								framework.Logf("checksum didn't match. checksum=%s and checksumclone=%s", checkSum, checkSumClone)
							}
						}
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				failed := 0
				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to create PVC or application (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				for i, err := range chErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("calculating checksum failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, totalSubvolumes, fileSystemName, subvolumegroup)
				validateOmapCount(f, totalSubvolumes, cephfsType, metadataPool, volumesType)

				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC or application: %v", err)
				}

				wg.Add(totalCount)
				// delete clone and app
				for i := 0; i < totalCount; i++ {
					go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
						name := fmt.Sprintf("%s%d", f.UniqueName, n)
						p.Spec.DataSource.Name = name
						wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
						wg.Done()
					}(i, *pvcClone, *appClone)
				}
				wg.Wait()

				for i, err := range wgErrs {
					if err != nil {
						// not using Failf() as it aborts the test and does not log other errors
						framework.Logf("failed to delete PVC or application (%s%d): %v", f.UniqueName, i, err)
						failed++
					}
				}
				if failed != 0 {
					framework.Failf("deleting PVCs and apps failed, %d errors were logged", failed)
				}

				validateSubvolumeCount(f, 0, fileSystemName, subvolumegroup)
				validateOmapCount(f, 0, cephfsType, metadataPool, volumesType)
			})
		})
	})
})
