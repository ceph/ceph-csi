/*
Copyright 2021 The Ceph-CSI Authors.

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

	"github.com/ceph/ceph-csi/internal/util"

	. "github.com/onsi/ginkgo/v2" //nolint:golint // e2e uses By() and other Ginkgo functions
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edebug "k8s.io/kubernetes/test/e2e/framework/debug"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/pod-security-admission/api"
)

var (
	rbdProvisioner     = "csi-rbdplugin-provisioner.yaml"
	rbdProvisionerRBAC = "csi-provisioner-rbac.yaml"
	rbdNodePlugin      = "csi-rbdplugin.yaml"
	rbdNodePluginRBAC  = "csi-nodeplugin-rbac.yaml"
	configMap          = "csi-config-map.yaml"
	cephConfconfigMap  = "ceph-conf.yaml"
	csiDriverObject    = "csidriver.yaml"
	deployPath         = "../deploy/"
	rbdDirPath         = deployPath + "/rbd/kubernetes/"
	examplePath        = "../examples/"
	rbdExamplePath     = examplePath + "/rbd/"
	e2eTemplatesPath   = "../e2e/templates/"
	rbdDeploymentName  = "csi-rbdplugin-provisioner"
	rbdDaemonsetName   = "csi-rbdplugin"
	rbdContainerName   = "csi-rbdplugin"
	defaultRBDPool     = "replicapool"
	erasureCodedPool   = "ec-pool"
	noDataPool         = ""
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
	rawAppRWOPPath         = rbdExamplePath + "raw-block-pod-rwop.yaml"
	rawPVCRWOPPath         = rbdExamplePath + "raw-block-pvc-rwop.yaml"
	pvcClonePath           = rbdExamplePath + "pvc-restore.yaml"
	pvcSmartClonePath      = rbdExamplePath + "pvc-clone.yaml"
	pvcBlockSmartClonePath = rbdExamplePath + "pvc-block-clone.yaml"
	pvcRWOPPath            = rbdExamplePath + "pvc-rwop.yaml"
	appRWOPPath            = rbdExamplePath + "pod-rwop.yaml"
	appClonePath           = rbdExamplePath + "pod-restore.yaml"
	appSmartClonePath      = rbdExamplePath + "pod-clone.yaml"
	appBlockSmartClonePath = rbdExamplePath + "block-pod-clone.yaml"
	pvcBlockRestorePath    = rbdExamplePath + "pvc-block-restore.yaml"
	appBlockRestorePath    = rbdExamplePath + "pod-block-restore.yaml"
	appEphemeralPath       = rbdExamplePath + "pod-ephemeral.yaml"
	snapshotPath           = rbdExamplePath + "snapshot.yaml"
	deployFSAppPath        = e2eTemplatesPath + "rbd-fs-deployment.yaml"
	deployBlockAppPath     = e2eTemplatesPath + "rbd-block-deployment.yaml"
	defaultCloneCount      = 3 // TODO: set to 10 once issues#2327 is fixed

	nbdMapOptions             = "nbd:debug-rbd=20"
	e2eDefaultCephLogStrategy = "preserve"

	// PV and PVC metadata keys used by external provisioner as part of
	// create requests as parameters, when `extra-create-metadata` is true.
	pvcNameKey      = "csi.storage.k8s.io/pvc/name"
	pvcNamespaceKey = "csi.storage.k8s.io/pvc/namespace"
	pvNameKey       = "csi.storage.k8s.io/pv/name"

	// snapshot metadata keys.
	volSnapNameKey        = "csi.storage.k8s.io/volumesnapshot/name"
	volSnapNamespaceKey   = "csi.storage.k8s.io/volumesnapshot/namespace"
	volSnapContentNameKey = "csi.storage.k8s.io/volumesnapshotcontent/name"
)

func deployRBDPlugin() {
	// delete objects deployed by rook
	data, err := replaceNamespaceInTemplate(rbdDirPath + rbdProvisionerRBAC)
	if err != nil {
		framework.Failf("failed to read content from %s: %v", rbdDirPath+rbdProvisionerRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		framework.Failf("failed to delete provisioner rbac %s: %v", rbdDirPath+rbdProvisionerRBAC, err)
	}

	data, err = replaceNamespaceInTemplate(rbdDirPath + rbdNodePluginRBAC)
	if err != nil {
		framework.Failf("failed to read content from %s: %v", rbdDirPath+rbdNodePluginRBAC, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		framework.Failf("failed to delete nodeplugin rbac %s: %v", rbdDirPath+rbdNodePluginRBAC, err)
	}

	createORDeleteRbdResources(kubectlCreate)
}

func deleteRBDPlugin() {
	createORDeleteRbdResources(kubectlDelete)
}

func createORDeleteRbdResources(action kubectlAction) {
	cephConfigFile := getConfigFile(cephConfconfigMap, deployPath, examplePath)
	resources := []ResourceDeployer{
		// shared resources
		&yamlResource{
			filename:     rbdDirPath + csiDriverObject,
			allowMissing: true,
		},
		&yamlResource{
			filename:     cephConfigFile,
			allowMissing: true,
		},
		// dependencies for provisioner
		&yamlResourceNamespaced{
			filename:  rbdDirPath + rbdProvisionerRBAC,
			namespace: cephCSINamespace,
		},
		// the provisioner itself
		&yamlResourceNamespaced{
			filename:       rbdDirPath + rbdProvisioner,
			namespace:      cephCSINamespace,
			oneReplica:     true,
			enableTopology: true,
		},
		// dependencies for the node-plugin
		&yamlResourceNamespaced{
			filename:  rbdDirPath + rbdNodePluginRBAC,
			namespace: cephCSINamespace,
		},
		// the node-plugin itself
		&yamlResourceNamespaced{
			filename:    rbdDirPath + rbdNodePlugin,
			namespace:   cephCSINamespace,
			domainLabel: nodeRegionLabel + "," + nodeZoneLabel,
		},
	}

	for _, r := range resources {
		err := r.Do(action)
		if err != nil {
			framework.Failf("failed to %s resource: %v", action, err)
		}
	}
}

func validateRBDImageCount(f *framework.Framework, count int, pool string) {
	imageList, err := listRBDImages(f, pool)
	if err != nil {
		framework.Failf("failed to list rbd images: %v", err)
	}
	if len(imageList) != count {
		framework.Failf(
			"backend images not matching kubernetes resource count,image count %d kubernetes resource count %d"+
				"\nbackend image Info:\n %v",
			len(imageList),
			count,
			imageList)
	}
}

func formatImageMetaGetCmd(pool, image, key string) string {
	return fmt.Sprintf("rbd image-meta get %s --image=%s %s", rbdOptions(pool), image, key)
}

// checkGetKeyError check for error conditions returned by get image-meta key,
// returns true if key exists.
func checkGetKeyError(err error, stdErr string) bool {
	if err == nil || !strings.Contains(err.Error(), "command terminated with exit code 2") ||
		!strings.Contains(stdErr, "failed to get metadata") {
		return true
	}

	return false
}

// checkClusternameInMetadata check for cluster name metadata on RBD image.
//
//nolint:nilerr // intentionally returning nil on error in the retry loop.
func checkClusternameInMetadata(f *framework.Framework, ns, pool, image string) {
	t := time.Duration(deployTimeout) * time.Minute
	var (
		coName  string
		stdErr  string
		execErr error
	)
	err := wait.PollUntilContextTimeout(context.TODO(), poll, t, true, func(_ context.Context) (bool, error) {
		coName, stdErr, execErr = execCommandInToolBoxPod(f,
			fmt.Sprintf("rbd image-meta get %s --image=%s %s", rbdOptions(pool), image, clusterNameKey),
			ns)
		if execErr != nil || stdErr != "" {
			framework.Logf("failed to get cluster name %s/%s %s: err=%v stdErr=%q",
				rbdOptions(pool), image, clusterNameKey, execErr, stdErr)

			return false, nil
		}

		return true, nil
	})
	if err != nil {
		framework.Failf("could not get cluster name %s/%s %s: %v", rbdOptions(pool), image, clusterNameKey, err)
	}
	coName = strings.TrimSuffix(coName, "\n")
	if coName != defaultClusterName {
		framework.Failf("expected coName %q got %q", defaultClusterName, coName)
	}
}

// ByFileAndBlockEncryption wraps ginkgo's By to run the test body using file and block encryption specific validators.
func ByFileAndBlockEncryption(
	text string,
	callback func(validator encryptionValidateFunc, pvcValidator validateFunc, encryptionType util.EncryptionType),
) {
	By(text+" (block)", func() {
		callback(validateEncryptedPVCAndAppBinding, isBlockEncryptedPVC, util.EncryptionTypeBlock)
	})
	By(text+" (file)", func() {
		if !testRBDFSCrypt {
			framework.Logf("skipping RBD fscrypt file encryption test")

			return
		}
		callback(validateEncryptedFilesystemAndAppBinding, isFileEncryptedPVC, util.EncryptionTypeFile)
	})
}

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework(rbdType)
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged
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
				framework.Failf("failed to create node label: %v", err)
			}
			err = createNodeLabel(f, nodeZoneLabel, zoneValue)
			if err != nil {
				framework.Failf("failed to create node label: %v", err)
			}
			if cephCSINamespace != defaultNs {
				err = createNamespace(c, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to create namespace: %v", err)
				}
			}
			deployRBDPlugin()
		}
		err := createConfigMap(rbdDirPath, f.ClientSet, f)
		if err != nil {
			framework.Failf("failed to create configmap: %v", err)
		}
		// Since helm deploys storageclass, skip storageclass creation if
		// ceph-csi is deployed via helm.
		if !helmTest {
			err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
			if err != nil {
				framework.Failf("failed to create storageclass: %v", err)
			}
		}
		// create rbd provisioner secret
		key, err := createCephUser(f, keyringRBDProvisionerUsername, rbdProvisionerCaps("", ""))
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringRBDProvisionerUsername, err)
		}
		err = createRBDSecret(f, rbdProvisionerSecretName, keyringRBDProvisionerUsername, key)
		if err != nil {
			framework.Failf("failed to create provisioner secret: %v", err)
		}
		// create rbd plugin secret
		key, err = createCephUser(f, keyringRBDNodePluginUsername, rbdNodePluginCaps("", ""))
		if err != nil {
			framework.Failf("failed to create user %s: %v", keyringRBDNodePluginUsername, err)
		}
		err = createRBDSecret(f, rbdNodePluginSecretName, keyringRBDNodePluginUsername, key)
		if err != nil {
			framework.Failf("failed to create node secret: %v", err)
		}
		deployVault(f.ClientSet, deployTimeout)

		// wait for provisioner deployment
		err = waitForDeploymentComplete(f.ClientSet, rbdDeploymentName, cephCSINamespace, deployTimeout)
		if err != nil {
			framework.Failf("timeout waiting for deployment %s: %v", rbdDeploymentName, err)
		}

		// wait for nodeplugin deamonset pods
		err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
		if err != nil {
			framework.Failf("timeout waiting for daemonset %s: %v", rbdDaemonsetName, err)
		}

		kernelRelease, err = getKernelVersionFromDaemonset(f, cephCSINamespace, rbdDaemonsetName, "csi-rbdplugin")
		if err != nil {
			framework.Failf("failed to get the kernel version: %v", err)
		}
		// default io-timeout=0, needs kernel >= 5.4
		if !util.CheckKernelSupport(kernelRelease, nbdZeroIOtimeoutSupport) {
			nbdMapOptions = "nbd:debug-rbd=20,io-timeout=330"
		}

		// wait for cluster name update in deployment
		containers := []string{"csi-rbdplugin", "csi-rbdplugin-controller"}
		err = waitForContainersArgsUpdate(c, cephCSINamespace, rbdDeploymentName,
			"clustername", defaultClusterName, containers, deployTimeout)
		if err != nil {
			framework.Failf("timeout waiting for deployment update %s/%s: %v", cephCSINamespace, rbdDeploymentName, err)
		}
	})

	AfterEach(func() {
		if !testRBD || upgradeTesting {
			Skip("Skipping RBD E2E")
		}
		if CurrentSpecReport().Failed() {
			// log pods created by helm chart
			logsCSIPods("app=ceph-csi-rbd", c)
			// log provisioner
			logsCSIPods("app=csi-rbdplugin-provisioner", c)
			// log node plugin
			logsCSIPods("app=csi-rbdplugin", c)

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), c, cephCSINamespace)
		}

		err := deleteConfigMap(rbdDirPath)
		if err != nil {
			framework.Failf("failed to delete configmap: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete provisioner secret: %v", err)
		}
		err = c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			framework.Failf("failed to delete node secret: %v", err)
		}
		err = deleteResource(rbdExamplePath + "storageclass.yaml")
		if err != nil {
			framework.Failf("failed to delete storageclass: %v", err)
		}
		// deleteResource(rbdExamplePath + "snapshotclass.yaml")
		deleteVault()
		if deployRBD {
			deleteRBDPlugin()
			if cephCSINamespace != defaultNs {
				err = deleteNamespace(c, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to delete namespace: %v", err)
				}
			}
		}
		err = deleteNodeLabel(c, nodeRegionLabel)
		if err != nil {
			framework.Failf("failed to delete node label: %v", err)
		}
		err = deleteNodeLabel(c, nodeZoneLabel)
		if err != nil {
			framework.Failf("failed to delete node label: %v", err)
		}
		// Remove the CSI labels that get added
		err = deleteNodeLabel(c, nodeCSIRegionLabel)
		if err != nil {
			framework.Failf("failed to delete node label: %v", err)
		}
		err = deleteNodeLabel(c, nodeCSIZoneLabel)
		if err != nil {
			framework.Failf("failed to delete node label: %v", err)
		}
	})

	Context("Test RBD CSI", func() {
		if !testRBD || upgradeTesting {
			return
		}

		It("Test RBD CSI", func() {
			// test only if ceph-csi is deployed via helm
			if helmTest {
				By("verify PVC and app binding on helm installation", func() {
					err := validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						framework.Failf("failed to validate RBD pvc and application binding: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
					//  Deleting the storageclass and secret created by helm
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = deleteResource(rbdExamplePath + "secret.yaml")
					if err != nil {
						framework.Failf("failed to delete secret: %v", err)
					}
					// Re-create the RBD storageclass
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				})
			}

			By("verify mountOptions support", func() {
				err := verifySeLinuxMountOption(f, pvcPath, appPath,
					rbdDaemonsetName, rbdContainerName, cephCSINamespace)
				if err != nil {
					framework.Failf("failed to verify mount options: %v", err)
				}
			})

			By("create a PVC and check PVC/PV metadata on RBD image", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				imageList, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					framework.Failf("failed to list rbd images: %v", err)
				}

				pvcName, stdErr, err := execCommandInToolBoxPod(f,
					formatImageMetaGetCmd(defaultRBDPool, imageList[0], pvcNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PVC name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey, err, stdErr)
				}
				pvcName = strings.TrimSuffix(pvcName, "\n")
				if pvcName != pvc.Name {
					framework.Failf("expected pvcName %q got %q", pvc.Name, pvcName)
				}

				pvcNamespace, stdErr, err := execCommandInToolBoxPod(f,
					formatImageMetaGetCmd(defaultRBDPool, imageList[0], pvcNamespaceKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PVC namespace %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNamespaceKey, err, stdErr)
				}
				pvcNamespace = strings.TrimSuffix(pvcNamespace, "\n")
				if pvcNamespace != pvc.Namespace {
					framework.Failf("expected pvcNamespace %q got %q", pvc.Namespace, pvcNamespace)
				}
				pvcObj, err := getPersistentVolumeClaim(c, pvc.Namespace, pvc.Name)
				if err != nil {
					framework.Logf("error getting pvc %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				if pvcObj.Spec.VolumeName == "" {
					framework.Logf("pv name is empty %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				pvName, stdErr, err := execCommandInToolBoxPod(f,
					formatImageMetaGetCmd(defaultRBDPool, imageList[0], pvNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PV name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvNameKey, err, stdErr)
				}
				pvName = strings.TrimSuffix(pvName, "\n")
				if pvName != pvcObj.Spec.VolumeName {
					framework.Failf("expected pvName %q got %q", pvcObj.Spec.VolumeName, pvName)
				}

				checkClusternameInMetadata(f, rookNamespace, defaultRBDPool, imageList[0])

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("reattach the old PV to a new PVC and check if PVC metadata is updated on RBD image", func() {
				reattachPVCNamespace := fmt.Sprintf("%s-2", f.Namespace.Name)
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				imageList, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					framework.Failf("failed to list rbd images: %v", err)
				}

				pvcName, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PVC name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey, err, stdErr)
				}
				pvcName = strings.TrimSuffix(pvcName, "\n")
				if pvcName != pvc.Name {
					framework.Failf("expected pvcName %q got %q", pvc.Name, pvcName)
				}

				pvcObj, err := c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(
					context.TODO(),
					pvc.Name,
					metav1.GetOptions{})
				if err != nil {
					framework.Logf("error getting pvc %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				if pvcObj.Spec.VolumeName == "" {
					framework.Logf("pv name is empty %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}

				// patch PV to Retain it after deleting the PVC.
				patchBytes := []byte(`{"spec":{"persistentVolumeReclaimPolicy": "Retain"}}`)
				_, err = c.CoreV1().PersistentVolumes().Patch(
					context.TODO(),
					pvcObj.Spec.VolumeName,
					types.StrategicMergePatchType,
					patchBytes,
					metav1.PatchOptions{})
				if err != nil {
					framework.Logf("error Patching PV %q for persistentVolumeReclaimPolicy: %v",
						pvcObj.Spec.VolumeName, err)
				}

				err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(
					context.TODO(),
					pvc.Name,
					metav1.DeleteOptions{})
				if err != nil {
					framework.Logf("failed to delete pvc: %w", err)
				}

				// Remove the claimRef to bind this PV to a new PVC.
				patchBytes = []byte(`{"spec":{"claimRef": null}}`)
				_, err = c.CoreV1().PersistentVolumes().Patch(
					context.TODO(),
					pvcObj.Spec.VolumeName,
					types.StrategicMergePatchType,
					patchBytes,
					metav1.PatchOptions{})
				if err != nil {
					framework.Logf("error Patching PV %q for claimRef: %v",
						pvcObj.Spec.VolumeName, err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				// create namespace for reattach PVC, deletion will be taken care by framework
				ns, err := f.CreateNamespace(context.TODO(), reattachPVCNamespace, nil)
				if err != nil {
					framework.Failf("failed to create namespace: %v", err)
				}

				pvcObj.Name = "rbd-pvc-new"
				pvcObj.Namespace = ns.Name

				// unset the resource version as should not be set on objects to be created
				pvcObj.ResourceVersion = ""
				err = createPVCAndvalidatePV(f.ClientSet, pvcObj, deployTimeout)
				if err != nil {
					framework.Failf("failed to create new PVC: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				pvcName, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PVC name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey, err, stdErr)
				}
				pvcName = strings.TrimSuffix(pvcName, "\n")
				if pvcName != pvcObj.Name {
					framework.Failf("expected pvcName %q got %q", pvcObj.Name, pvcName)
				}

				owner, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], pvcNamespaceKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get owner name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNamespaceKey, err, stdErr)
				}
				owner = strings.TrimSuffix(owner, "\n")
				if owner != pvcObj.Namespace {
					framework.Failf("expected pvcNamespace name %q got %q", pvcObj.Namespace, owner)
				}

				patchBytes = []byte(`{"spec":{"persistentVolumeReclaimPolicy": "Delete"}}`)
				_, err = c.CoreV1().PersistentVolumes().Patch(
					context.TODO(),
					pvcObj.Spec.VolumeName,
					types.StrategicMergePatchType,
					patchBytes,
					metav1.PatchOptions{})
				if err != nil {
					framework.Logf("error Patching PV %q for persistentVolumeReclaimPolicy: %v", pvcObj.Spec.VolumeName, err)
				}
				err = deletePVCAndValidatePV(f.ClientSet, pvcObj, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a snapshot and check metadata on RBD snapshot image", func() {
				err := createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				// delete pod as we should not create snapshot for in-use pvc
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

				imageList, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					framework.Failf("failed to list rbd images: %v", err)
				}

				volSnapName, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], volSnapNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get volume snapshot name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], volSnapNameKey, err, stdErr)
				}
				volSnapName = strings.TrimSuffix(volSnapName, "\n")
				if volSnapName != snap.Name {
					framework.Failf("expected volSnapName %q got %q", snap.Name, volSnapName)
				}

				volSnapNamespace, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], volSnapNamespaceKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get volume snapshot namespace %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], volSnapNamespaceKey, err, stdErr)
				}
				volSnapNamespace = strings.TrimSuffix(volSnapNamespace, "\n")
				if volSnapNamespace != snap.Namespace {
					framework.Failf("expected volSnapNamespace %q got %q", snap.Namespace, volSnapNamespace)
				}

				content, err := getVolumeSnapshotContent(snap.Namespace, snap.Name)
				if err != nil {
					framework.Failf("failed to get snapshotcontent for %s in namespace %s: %v",
						snap.Name, snap.Namespace, err)
				}
				volSnapContentName, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], volSnapContentNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get snapshotcontent name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], volSnapContentNameKey, err, stdErr)
				}
				volSnapContentName = strings.TrimSuffix(volSnapContentName, "\n")
				if volSnapContentName != content.Name {
					framework.Failf("expected volSnapContentName %q got %q", content.Name, volSnapContentName)
				}

				// make sure we had unset the PVC metadata on the rbd image created
				// for the snapshot
				pvcName, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PVC name found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey, pvcName, err, stdErr)
				}
				pvcNamespace, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], pvcNamespaceKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PVC namespace found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNamespaceKey, pvcNamespace, err, stdErr)
				}
				pvName, stdErr, err := execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageList[0], pvNameKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PV name found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvNameKey, pvName, err, stdErr)
				}
				checkClusternameInMetadata(f, rookNamespace, defaultRBDPool, imageList[0])

				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("verify generic ephemeral volume support", func() {
				// create application
				app, err := loadApp(appEphemeralPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
				}
			})

			By("validate RBD migration PVC", func() {
				err := setupMigrationCMSecretAndSC(f, "")
				if err != nil {
					framework.Failf("failed to setup migration prerequisites: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				// Block PVC resize
				err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					framework.Failf("failed to resize block PVC: %v", err)
				}

				// FileSystem PVC resize
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize filesystem PVC: %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = tearDownMigrationSetup(f)
				if err != nil {
					framework.Failf("failed to tear down migration setup: %v", err)
				}
			})

			By("validate RBD migration+static FileSystem", func() {
				err := setupMigrationCMSecretAndSC(f, "migrationsc")
				if err != nil {
					framework.Failf("failed to setup migration prerequisites: %v", err)
				}
				// validate filesystem pvc mount
				err = validateRBDStaticMigrationPVC(f, appPath, "migrationsc", false)
				if err != nil {
					framework.Failf("failed to validate rbd migrated static file mode pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = tearDownMigrationSetup(f)
				if err != nil {
					framework.Failf("failed to tear down migration setup: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and validate owner", func() {
				err := validateImageOwner(pvcPath, f)
				if err != nil {
					framework.Failf("failed to validate owner of pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a PVC and bind it to an app with normal user", func() {
				err := validateNormalUserPVCAccess(pvcPath, f)
				if err != nil {
					framework.Failf("failed to validate normal user pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a Block mode RWOP PVC and bind it to more than one app", func() {
				pvc, err := loadPVC(rawPVCRWOPPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(rawAppRWOPPath)
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
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
				err = validateRWOPPodCreation(f, pvc, app, baseAppName)
				if err != nil {
					framework.Failf("failed to validate RWOP pod creation: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a RWOP PVC and bind it to more than one app", func() {
				pvc, err := loadPVC(pvcRWOPPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

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
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				err = createApp(f.ClientSet, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}
				err = validateRWOPPodCreation(f, pvc, app, baseAppName)
				if err != nil {
					framework.Failf("failed to validate RWOP pod creation: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create an erasure coded PVC and bind it to an app", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"dataPool": erasureCodedPool,
						"pool":     defaultRBDPool,
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc and application binding: %v", err)
				}
				err = checkPVCDataPoolForImageInPool(f, pvc, defaultRBDPool, "ec-pool")
				if err != nil {
					framework.Failf("failed to check data pool for image: %v", err)
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete pvc and application : %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create an erasure coded PVC and validate snapshot restore", func() {
				validatePVCSnapshot(
					defaultCloneCount,
					pvcPath,
					appPath,
					snapshotPath,
					pvcClonePath,
					appClonePath,
					noKMS, noKMS,
					defaultSCName,
					erasureCodedPool,
					f,
					noPVCValidation)
			})

			By("create an erasure coded PVC and validate PVC-PVC clone", func() {
				validatePVCClone(
					defaultCloneCount,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					defaultSCName,
					erasureCodedPool,
					noKMS,
					noPVCValidation,
					f)
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with ext4 as the FS ", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"csi.storage.k8s.io/fstype": "ext4"},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app with ext4 as the FS and 1024 inodes ", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"csi.storage.k8s.io/fstype": "ext4",
						"mkfsOptions":               "-N1024", // 1024 inodes
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				err = validateInodeCount(pvcPath, f, 1024)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a PVC and bind it to an app using rbd-nbd mounter", func() {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Resize rbd-nbd PVC and check application directory size", func() {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}

				if util.CheckKernelSupport(kernelRelease, nbdResizeSupport) {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					// Storage class with rbd-nbd mounter
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"mounter":         "rbd-nbd",
							"mapOptions":      nbdMapOptions,
							"cephLogStrategy": e2eDefaultCephLogStrategy,
						},
						deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					// Block PVC resize
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						framework.Failf("failed to resize block PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

					// FileSystem PVC resize
					err = resizePVCAndValidateSize(pvcPath, appPath, f)
					if err != nil {
						framework.Failf("failed to resize filesystem PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				}
			})

			By("create PVC with layering,fast-diff image-features and bind it to an app",
				func() {
					if util.CheckKernelSupport(kernelRelease, fastDiffSupport) {
						err := deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(
							f.ClientSet,
							f,
							defaultSCName,
							nil,
							map[string]string{
								"imageFeatures": "layering,exclusive-lock,object-map,fast-diff",
							},
							deletePolicy)
						if err != nil {
							framework.Failf("failed to create storageclass: %v", err)
						}
						err = validatePVCAndAppBinding(pvcPath, appPath, f)
						if err != nil {
							framework.Failf("failed to validate RBD pvc and application binding: %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 0, defaultRBDPool)
						validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							framework.Failf("failed to create storageclass: %v", err)
						}
					}
				})

			By("create PVC with layering,deep-flatten image-features and bind it to an app",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures": "layering,deep-flatten",
						},
						deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					// set up PVC
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						framework.Failf("failed to load PVC: %v", err)
					}
					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

					if util.CheckKernelSupport(kernelRelease, deepFlattenSupport) {
						app, aErr := loadApp(appPath)
						if aErr != nil {
							framework.Failf("failed to load application: %v", aErr)
						}
						app.Namespace = f.UniqueName
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							framework.Failf("failed to create application: %v", err)
						}
						// delete pod as we should not create snapshot for in-use pvc
						err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
						if err != nil {
							framework.Failf("failed to delete application: %v", err)
						}

					}
					// clean up after ourselves
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						framework.Failf("failed to delete PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				})

			By("create PVC with layering,deep-flatten image-features and bind it to an app",
				func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures": "",
						},
						deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					// set up PVC
					pvc, err := loadPVC(pvcPath)
					if err != nil {
						framework.Failf("failed to load PVC: %v", err)
					}
					pvc.Namespace = f.UniqueName
					err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

					// checking the minimal kernel version for fast-diff as its
					// higher kernel version than other default image features.
					if util.CheckKernelSupport(kernelRelease, fastDiffSupport) {
						app, aErr := loadApp(appPath)
						if aErr != nil {
							framework.Failf("failed to load application: %v", aErr)
						}
						app.Namespace = f.UniqueName
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							framework.Failf("failed to create application: %v", err)
						}
						// delete pod as we should not create snapshot for in-use pvc
						err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
						if err != nil {
							framework.Failf("failed to delete application: %v", err)
						}

					}
					// clean up after ourselves
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						framework.Failf("failed to delete PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				})

			By("create PVC with journaling,fast-diff image-features and bind it to an app using rbd-nbd mounter",
				func() {
					if !testNBD {
						framework.Logf("skipping NBD test")

						return
					}

					if util.CheckKernelSupport(kernelRelease, fastDiffSupport) {
						err := deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
						// Storage class with rbd-nbd mounter
						err = createRBDStorageClass(
							f.ClientSet,
							f,
							defaultSCName,
							nil,
							map[string]string{
								"mounter":       "rbd-nbd",
								"imageFeatures": "layering,journaling,exclusive-lock,object-map,fast-diff",
							},
							deletePolicy)
						if err != nil {
							framework.Failf("failed to create storageclass: %v", err)
						}
						err = validatePVCAndAppBinding(pvcPath, appPath, f)
						if err != nil {
							framework.Failf("failed to validate RBD pvc and application binding: %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 0, defaultRBDPool)
						validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							framework.Failf("failed to create storageclass: %v", err)
						}
					}
				})

			// NOTE: RWX is restricted for FileSystem VolumeMode at ceph-csi,
			// see pull#261 for more details.
			By("Create RWX+Block Mode PVC and bind to multiple pods via deployment using rbd-nbd mounter", func() {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				pvc, err := loadPVC(rawPvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				pvc.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadWriteMany}

				app, err := loadAppDeployment(deployBlockAppPath)
				if err != nil {
					framework.Failf("failed to load application deployment: %v", err)
				}
				app.Namespace = f.UniqueName

				err = createPVCAndDeploymentApp(f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}

				err = waitForDeploymentComplete(f.ClientSet, app.Name, app.Namespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment to be in running state: %v", err)
				}

				devPath := app.Spec.Template.Spec.Containers[0].VolumeDevices[0].DevicePath
				cmd := fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10", devPath)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", app.Name),
				}
				podList, err := e2epod.PodClientNS(f, app.Namespace).List(context.TODO(), opt)
				if err != nil {
					framework.Failf("get pod list failed: %v", err)
				}
				if len(podList.Items) != int(*app.Spec.Replicas) {
					framework.Failf("podlist contains %d items, expected %d items", len(podList.Items), *app.Spec.Replicas)
				}
				for _, pod := range podList.Items {
					_, _, err = execCommandInPodWithName(f, cmd, pod.Name, pod.Spec.Containers[0].Name, app.Namespace)
					if err != nil {
						framework.Failf("command %q failed: %v", cmd, err)
					}
				}

				err = deletePVCAndDeploymentApp(f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Create ROX+FS Mode PVC and bind to multiple pods via deployment using rbd-nbd mounter", func() {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}

				// create clone PVC as ROX
				pvcClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				appClone, err := loadAppDeployment(deployFSAppPath)
				if err != nil {
					framework.Failf("failed to load application deployment: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndDeploymentApp(f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}

				err = waitForDeploymentComplete(f.ClientSet, appClone.Name, appClone.Namespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment to be in running state: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 3, defaultRBDPool)
				validateOmapCount(f, 2, rbdType, defaultRBDPool, volumesType)

				filePath := appClone.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				cmd := fmt.Sprintf("echo 'Hello World' > %s", filePath)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", appClone.Name),
				}
				podList, err := e2epod.PodClientNS(f, appClone.Namespace).List(context.TODO(), opt)
				if err != nil {
					framework.Failf("get pod list failed: %v", err)
				}
				if len(podList.Items) != int(*appClone.Spec.Replicas) {
					framework.Failf("podlist contains %d items, expected %d items", len(podList.Items), *appClone.Spec.Replicas)
				}
				for _, pod := range podList.Items {
					var stdErr string
					_, stdErr, err = execCommandInPodWithName(f, cmd, pod.Name, pod.Spec.Containers[0].Name, appClone.Namespace)
					if err != nil {
						framework.Logf("command %q failed: %v", cmd, err)
					}
					readOnlyErr := fmt.Sprintf("cannot create %s: Read-only file system", filePath)
					if !strings.Contains(stdErr, readOnlyErr) {
						framework.Failf(stdErr)
					}
				}

				err = deletePVCAndDeploymentApp(f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("Create ROX+Block Mode PVC and bind to multiple pods via deployment using rbd-nbd mounter", func() {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				// create PVC and bind it to an app
				pvc, err := loadPVC(rawPvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				app, err := loadApp(rawAppPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}

				// create clone PVC as ROX
				pvcClone, err := loadPVC(pvcBlockSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Spec.DataSource.Name = pvc.Name
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				volumeMode := v1.PersistentVolumeBlock
				pvcClone.Spec.VolumeMode = &volumeMode
				appClone, err := loadAppDeployment(deployBlockAppPath)
				if err != nil {
					framework.Failf("failed to load application deployment: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
				appClone.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
				err = createPVCAndDeploymentApp(f, pvcClone, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}

				err = waitForDeploymentComplete(f.ClientSet, appClone.Name, appClone.Namespace, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment to be in running state: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 3, defaultRBDPool)
				validateOmapCount(f, 2, rbdType, defaultRBDPool, volumesType)

				devPath := appClone.Spec.Template.Spec.Containers[0].VolumeDevices[0].DevicePath
				cmd := fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10", devPath)

				opt := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app=%s", appClone.Name),
				}
				podList, err := e2epod.PodClientNS(f, appClone.Namespace).List(context.TODO(), opt)
				if err != nil {
					framework.Failf("get pod list failed: %v", err)
				}
				if len(podList.Items) != int(*appClone.Spec.Replicas) {
					framework.Failf("podlist contains %d items, expected %d items", len(podList.Items), *appClone.Spec.Replicas)
				}
				for _, pod := range podList.Items {
					var stdErr string
					_, stdErr, err = execCommandInPodWithName(f, cmd, pod.Name, pod.Spec.Containers[0].Name, appClone.Namespace)
					if err != nil {
						framework.Logf("command %q failed: %v", cmd, err)
					}
					readOnlyErr := fmt.Sprintf("'%s': Operation not permitted", devPath)
					if !strings.Contains(stdErr, readOnlyErr) {
						framework.Failf(stdErr)
					}
				}
				err = deletePVCAndDeploymentApp(f, pvcClone, appClone)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("perform IO on rbd-nbd volume after nodeplugin restart", func() {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
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
					framework.Failf("failed to sync, err: %v, stdErr: %v ", err, stdErr)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDaemonsetName)
				if err != nil {
					framework.Failf("failed to get the labels: %v", err)
				}
				// delete rbd nodeplugin pods
				err = deletePodWithLabel(selector, cephCSINamespace, false)
				if err != nil {
					framework.Failf("fail to delete pod: %v", err)
				}

				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for daemonset pods: %v", err)
				}

				opt := metav1.ListOptions{
					LabelSelector: selector,
				}
				uname, stdErr, err := execCommandInContainer(f, "uname -a", cephCSINamespace, "csi-rbdplugin", &opt)
				if err != nil || stdErr != "" {
					framework.Failf("failed to run uname cmd : %v, stdErr: %v ", err, stdErr)
				}
				framework.Logf("uname -a: %v", uname)
				rpmv, stdErr, err := execCommandInContainer(
					f,
					"rpm -qa | grep rbd-nbd",
					cephCSINamespace,
					"csi-rbdplugin",
					&opt)
				if err != nil || stdErr != "" {
					framework.Failf("failed to run rpm -qa cmd : %v, stdErr: %v ", err, stdErr)
				}
				framework.Logf("rbd-nbd package version: %v", rpmv)

				timeout := time.Duration(deployTimeout) * time.Minute
				var reason string
				err = wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
					var runningAttachCmd string
					runningAttachCmd, stdErr, err = execCommandInContainer(
						f,
						"pstree --arguments | grep [r]bd-nbd",
						cephCSINamespace,
						"csi-rbdplugin",
						&opt)
					// if the rbd-nbd process is not running the 'grep' command
					// will return with exit code 1
					if err != nil {
						if strings.Contains(err.Error(), "command terminated with exit code 1") {
							reason = fmt.Sprintf("rbd-nbd process is not running yet: %v", err)
						} else if stdErr != "" {
							reason = fmt.Sprintf("failed to run ps cmd : %v, stdErr: %v", err, stdErr)
						}
						framework.Logf("%s", reason)

						return false, nil
					}
					framework.Logf("attach command running after restart, runningAttachCmd: %v", runningAttachCmd)

					return true, nil
				})

				if wait.Interrupted(err) {
					framework.Failf("timed out waiting for the rbd-nbd process: %s", reason)
				}
				if err != nil {
					framework.Failf("failed to poll: %v", err)
				}

				// Writes on kernel < 5.4 are failing due to a bug in NBD driver,
				// NBD zero cmd timeout handling is fixed with kernel >= 5.4
				// see https://github.com/ceph/ceph-csi/issues/2204#issuecomment-930941047
				if util.CheckKernelSupport(kernelRelease, nbdZeroIOtimeoutSupport) {
					filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
					_, stdErr, err = execCommandInPod(
						f,
						fmt.Sprintf("echo 'Hello World' > %s", filePath),
						app.Namespace,
						&appOpt)
					if err != nil || stdErr != "" {
						framework.Failf("failed to write IO, err: %v, stdErr: %v ", err, stdErr)
					}
				} else {
					framework.Logf("kernel %q does not meet recommendation, skipping IO test", kernelRelease)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create a PVC and bind it to an app using rbd-nbd mounter with encryption", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				if !testNBD {
					framework.Logf("skipping NBD test")

					return
				}
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				// Storage class with rbd-nbd mounter
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"mounter":         "rbd-nbd",
						"mapOptions":      nbdMapOptions,
						"cephLogStrategy": e2eDefaultCephLogStrategy,
						"encrypted":       "true",
						"encryptionType":  util.EncryptionTypeString(encType),
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validator(pvcPath, appPath, noKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create a PVC and bind it to an app with encrypted RBD volume", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true", "encryptionType": util.EncryptionTypeString(encType)},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validator(pvcPath, appPath, noKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("Resize Encrypted Block PVC and check Device size", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true", "encryptionType": util.EncryptionTypeString(encType)},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				// FileSystem PVC resize
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize filesystem PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				if encType != util.EncryptionTypeFile {
					// Block PVC resize
					err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
					if err != nil {
						framework.Failf("failed to resize block PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create a PVC and bind it to an app with encrypted RBD volume with VaultKMS", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validator(pvcPath, appPath, vaultKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create a PVC and bind it to an app with encrypted RBD volume with VaultTokensKMS", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tokens-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				// name(space) of the Tenant
				tenant := f.UniqueName

				// create the Secret with Vault Token in the Tenants namespace
				token, err := getSecret(vaultExamplePath + "tenant-token.yaml")
				if err != nil {
					framework.Failf("failed to load tenant token from secret: %v", err)
				}
				_, err = c.CoreV1().Secrets(tenant).Create(context.TODO(), &token, metav1.CreateOptions{})
				if err != nil {
					framework.Failf("failed to create Secret with tenant token: %v", err)
				}

				err = validator(pvcPath, appPath, vaultTokensKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				// delete the Secret of the Tenant
				err = c.CoreV1().Secrets(tenant).Delete(context.TODO(), token.Name, metav1.DeleteOptions{})
				if err != nil {
					framework.Failf("failed to delete Secret with tenant token: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create a PVC and bind it to an app with encrypted RBD volume with VaultTenantSA KMS", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tenant-sa-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
				if err != nil {
					framework.Failf("failed to create ServiceAccount: %v", err)
				}
				defer deleteTenantServiceAccount(f.UniqueName)

				err = validator(pvcPath, appPath, vaultTenantSAKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create a PVC and bind it to an app with encrypted RBD volume with SecretsMetadataKMS",
				func(validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType) {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "secrets-metadata-test",
						"encryptionType":  util.EncryptionTypeString(encType),
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					err = validator(pvcPath, appPath, noKMS, f)
					if err != nil {
						framework.Failf("failed to validate encrypted pvc: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				})

			ByFileAndBlockEncryption("test RBD volume encryption with user secrets based SecretsMetadataKMS", func(
				validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "user-ns-secrets-metadata-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				// user provided namespace where secret will be created
				namespace := cephCSINamespace

				// create user Secret
				err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
				if err != nil {
					framework.Failf("failed to create user Secret: %v", err)
				}

				err = validator(pvcPath, appPath, noKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				// delete user secret
				err = retryKubectlFile(namespace,
					kubectlDelete,
					vaultExamplePath+vaultUserSecret,
					deployTimeout,
					"--ignore-not-found=true")
				if err != nil {
					framework.Failf("failed to delete user Secret: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption(
				"test RBD volume encryption with user secrets based SecretsMetadataKMS with tenant namespace",
				func(validator encryptionValidateFunc, isEncryptedPVC validateFunc, encType util.EncryptionType) {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "user-secrets-metadata-test",
						"encryptionType":  util.EncryptionTypeString(encType),
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}

					// PVC creation namespace where secret will be created
					namespace := f.UniqueName

					// create user Secret
					err = retryKubectlFile(namespace, kubectlCreate, vaultExamplePath+vaultUserSecret, deployTimeout)
					if err != nil {
						framework.Failf("failed to create user Secret: %v", err)
					}

					err = validator(pvcPath, appPath, noKMS, f)
					if err != nil {
						framework.Failf("failed to validate encrypted pvc: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

					// delete user secret
					err = retryKubectlFile(
						namespace,
						kubectlDelete,
						vaultExamplePath+vaultUserSecret,
						deployTimeout,
						"--ignore-not-found=true")
					if err != nil {
						framework.Failf("failed to delete user Secret: %v", err)
					}

					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				})

			By(
				"create a PVC and Bind it to an app with journaling/exclusive-lock image-features and rbd-nbd mounter",
				func() {
					if !testNBD {
						framework.Logf("skipping NBD test")

						return
					}

					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(
						f.ClientSet,
						f,
						defaultSCName,
						nil,
						map[string]string{
							"imageFeatures":   "layering,journaling,exclusive-lock",
							"mounter":         "rbd-nbd",
							"mapOptions":      nbdMapOptions,
							"cephLogStrategy": e2eDefaultCephLogStrategy,
						},
						deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					err = validatePVCAndAppBinding(pvcPath, appPath, f)
					if err != nil {
						framework.Failf("failed to validate pvc and application binding: %v", err)
					}
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				},
			)

			By("create a PVC clone and bind it to an app", func() {
				validatePVCSnapshot(
					defaultCloneCount,
					pvcPath,
					appPath,
					snapshotPath,
					pvcClonePath,
					appClonePath,
					noKMS, noKMS,
					defaultSCName,
					noDataPool,
					f,
					noPVCValidation)
			})

			By("create a PVC-PVC clone and bind it to an app", func() {
				validatePVCClone(
					defaultCloneCount,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					defaultSCName,
					noDataPool,
					noKMS,
					noPVCValidation,
					f)
			})

			ByFileAndBlockEncryption("create an encrypted PVC snapshot and restore it for an app with VaultKMS", func(
				validator encryptionValidateFunc, isEncryptedPVC validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				validatePVCSnapshot(1,
					pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath,
					vaultKMS, vaultKMS,
					defaultSCName, noDataPool,
					f, isEncryptedPVC)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("Validate PVC restore from vaultKMS to vaultTenantSAKMS", func(
				validator encryptionValidateFunc, isEncryptedPVC validateFunc, encType util.EncryptionType,
			) {
				restoreSCName := "restore-sc"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				scOpts = map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tenant-sa-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, restoreSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
				if err != nil {
					framework.Failf("failed to create ServiceAccount: %v", err)
				}
				defer deleteTenantServiceAccount(f.UniqueName)

				validatePVCSnapshot(1,
					pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath,
					vaultKMS, vaultTenantSAKMS,
					restoreSCName, noDataPool, f,
					isEncryptedPVC)

				err = retryKubectlArgs(cephCSINamespace, kubectlDelete, deployTimeout, "storageclass", restoreSCName)
				if err != nil {
					framework.Failf("failed to delete storageclass %q: %v", restoreSCName, err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("Validate PVC-PVC clone with different SC from vaultKMS to vaultTenantSAKMS", func(
				validator encryptionValidateFunc, isValidPVC validateFunc, encType util.EncryptionType,
			) {
				restoreSCName := "restore-sc"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				scOpts = map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-tenant-sa-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, restoreSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				err = createTenantServiceAccount(f.ClientSet, f.UniqueName)
				if err != nil {
					framework.Failf("failed to create ServiceAccount: %v", err)
				}
				defer deleteTenantServiceAccount(f.UniqueName)

				validatePVCClone(1,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					restoreSCName,
					noDataPool,
					secretsMetadataKMS,
					isValidPVC,
					f)

				err = retryKubectlArgs(cephCSINamespace, kubectlDelete, deployTimeout, "storageclass", restoreSCName)
				if err != nil {
					framework.Failf("failed to delete storageclass %q: %v", restoreSCName, err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)

				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create an encrypted PVC-PVC clone and bind it to an app", func(
				validator encryptionValidateFunc, isValidPVC validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "secrets-metadata-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				validatePVCClone(1,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					defaultSCName,
					noDataPool,
					secretsMetadataKMS,
					isValidPVC,
					f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			ByFileAndBlockEncryption("create an encrypted PVC-PVC clone and bind it to an app with VaultKMS", func(
				validator encryptionValidateFunc, isValidPVC validateFunc, encType util.EncryptionType,
			) {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				scOpts := map[string]string{
					"encrypted":       "true",
					"encryptionKMSID": "vault-test",
					"encryptionType":  util.EncryptionTypeString(encType),
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				validatePVCClone(1,
					pvcPath,
					appPath,
					pvcSmartClonePath,
					appSmartClonePath,
					defaultSCName,
					noDataPool,
					vaultKMS,
					isValidPVC,
					f)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create a block type PVC and bind it to an app", func() {
				err := validatePVCAndAppBinding(rawPvcPath, rawAppPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
			})
			By("create a Block mode PVC-PVC clone and bind it to an app", func() {
				_, err := f.ClientSet.Discovery().ServerVersion()
				if err != nil {
					framework.Failf("failed to get server version: %v", err)
				}
				validatePVCClone(
					defaultCloneCount,
					rawPvcPath,
					rawAppPath,
					pvcBlockSmartClonePath,
					appBlockSmartClonePath,
					defaultSCName,
					noDataPool,
					noKMS,
					noPVCValidation,
					f)
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
					err := createPVCAndApp(name, f, pvc, app, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC and application: %v", err)
					}

				}
				// validate created backend rbd images
				validateRBDImageCount(f, totalCount, defaultRBDPool)
				validateOmapCount(f, totalCount, rbdType, defaultRBDPool, volumesType)
				// delete PVC and app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					err := deletePVCAndApp(name, f, pvc, app)
					if err != nil {
						framework.Failf("failed to delete PVC and application: %v", err)
					}

				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("check data persist after recreating pod", func() {
				err := checkDataPersist(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to check data persist: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("Resize Filesystem PVC and check application directory size", func() {
				err := resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize filesystem PVC %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"csi.storage.k8s.io/fstype": "xfs"},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize filesystem PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("Resize Block PVC and check Device size", func() {
				err := resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					framework.Failf("failed to resize block PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("Test unmount after nodeplugin restart", func() {
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to  load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				// delete rbd nodeplugin pods
				err = deletePodWithLabel("app=csi-rbdplugin", cephCSINamespace, false)
				if err != nil {
					framework.Failf("fail to delete pod: %v", err)
				}
				// wait for nodeplugin pods to come up
				err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for daemonset pods: %v", err)
				}

				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create PVC in storageClass with volumeNamePrefix", func() {
				volumeNamePrefix := "foo-bar-"
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"volumeNamePrefix": volumeNamePrefix},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				// list RBD images and check if one of them has the same prefix
				foundIt := false
				images, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					framework.Failf("failed to list rbd images: %v", err)
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
					framework.Failf("failed to  delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				if !foundIt {
					framework.Failf("could not find image with prefix %s", volumeNamePrefix)
				}
			})

			By("create storageClass with encrypted as false", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "false"},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				// set up PVC
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				// clean up after ourselves
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to  delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("validate RBD static FileSystem PVC", func() {
				err := validateRBDStaticPV(f, appPath, false, false)
				if err != nil {
					framework.Failf("failed to validate rbd static pv: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("validate RBD static Block PVC", func() {
				err := validateRBDStaticPV(f, rawAppPath, true, false)
				if err != nil {
					framework.Failf("failed to validate rbd block pv: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("validate failure of RBD static PVC without imageFeatures parameter", func() {
				err := validateRBDStaticPV(f, rawAppPath, true, true)
				if err != nil {
					framework.Failf("Validation of static PVC without imageFeatures parameter failed with err %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("validate mount options in app pod", func() {
				mountFlags := []string{"discard"}
				err := checkMountOptions(pvcPath, appPath, f, mountFlags)
				if err != nil {
					framework.Failf("failed to check mount options: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("creating an app with a PVC, using a topology constrained StorageClass", func() {
				By("checking node has required CSI topology labels set", func() {
					err := checkNodeHasLabel(f.ClientSet, nodeCSIRegionLabel, regionValue)
					if err != nil {
						framework.Failf("failed to check node label: %v", err)
					}
					err = checkNodeHasLabel(f.ClientSet, nodeCSIZoneLabel, zoneValue)
					if err != nil {
						framework.Failf("failed to check node label: %v", err)
					}
				})

				By("creating a StorageClass with delayed binding mode and CSI topology parameter")
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"domainSegments\":" +
					"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
					"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName,
					map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
					map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}

				By("creating an app using a PV from the delayed binding mode StorageClass")
				pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, 0)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}

				By("ensuring created PV has required node selector values populated")
				err = checkPVSelectorValuesForPVC(f, pvc)
				if err != nil {
					framework.Failf("failed to check pv selector values: %v", err)
				}
				By("ensuring created PV has its image in the topology specific pool")
				err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					framework.Failf("failed to check image in pool: %v", err)
				}

				By("ensuring created PV has its image journal in the topology specific pool")
				err = checkPVCImageJournalInPool(f, pvc, rbdTopologyPool)
				if err != nil {
					framework.Failf("failed to check image journal: %v", err)
				}

				By("ensuring created PV has its CSI journal in the CSI journal specific pool")
				err = checkPVCCSIJournalInPool(f, pvc, "replicapool")
				if err != nil {
					framework.Failf("failed to check csi journal in pool: %v", err)
				}

				err = deleteJournalInfoInPool(f, pvc, "replicapool")
				if err != nil {
					framework.Failf("failed to delete omap data: %v", err)
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				By("checking if data pool parameter is honored", func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					topologyConstraint := "[{\"poolName\":\"" + rbdTopologyPool + "\",\"dataPool\":\"" + rbdTopologyDataPool +
						"\",\"domainSegments\":" +
						"[{\"domainLabel\":\"region\",\"value\":\"" + regionValue + "\"}," +
						"{\"domainLabel\":\"zone\",\"value\":\"" + zoneValue + "\"}]}]"
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName,
						map[string]string{"volumeBindingMode": "WaitForFirstConsumer"},
						map[string]string{"topologyConstrainedPools": topologyConstraint}, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					By("creating an app using a PV from the delayed binding mode StorageClass with a data pool")
					pvc, app, err = createPVCAndAppBinding(pvcPath, appPath, f, 0)
					if err != nil {
						framework.Failf("failed to create PVC and application: %v", err)
					}

					By("ensuring created PV has its image in the topology specific pool")
					err = checkPVCImageInPool(f, pvc, rbdTopologyPool)
					if err != nil {
						framework.Failf("failed to check  pvc image in pool: %v", err)
					}

					By("ensuring created image has the right data pool parameter set")
					err = checkPVCDataPoolForImageInPool(f, pvc, rbdTopologyPool, rbdTopologyDataPool)
					if err != nil {
						framework.Failf("failed to check data pool for image: %v", err)
					}

					err = deleteJournalInfoInPool(f, pvc, "replicapool")
					if err != nil {
						framework.Failf("failed to delete omap data: %v", err)
					}
					// cleanup and undo changes made by the test
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						framework.Failf("failed to delete PVC and application: %v", err)
					}
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				})

				// cleanup and undo changes made by the test
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			// Mount pvc to pod with invalid mount option,expected that
			// mounting will fail
			By("Mount pvc to pod with invalid mount option", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					map[string]string{rbdMountOptions: "debug,invalidOption"},
					nil,
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to  load PVC: %v", err)
				}
				pvc.Namespace = f.UniqueName

				app, err := loadApp(appPath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				app.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				// create an app and wait for 1 min for it to go to running state
				err = createApp(f.ClientSet, app, 1)
				if err == nil {
					framework.Failf("application should not go to running state due to invalid mount option")
				}
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}

				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("create rbd clones in different pool", func() {
				clonePool := "clone-test"
				// create pool for clones
				err := createPool(f, clonePool)
				if err != nil {
					framework.Failf("failed to create pool %s: %v", clonePool, err)
				}
				err = createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create snapshotclass: %v", err)
				}
				cloneSC := "clone-storageclass"
				param := map[string]string{
					"pool": clonePool,
				}
				// create new storageclass with new pool
				err = createRBDStorageClass(f.ClientSet, f, cloneSC, nil, param, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validateCloneInDifferentPool(f, defaultRBDPool, cloneSC, clonePool)
				if err != nil {
					framework.Failf("failed to validate clones in different pool: %v", err)
				}

				err = retryKubectlArgs(
					cephCSINamespace,
					kubectlDelete,
					deployTimeout,
					"sc",
					cloneSC,
					"--ignore-not-found=true")
				if err != nil {
					framework.Failf("failed to delete storageclass %s: %v", cloneSC, err)
				}

				err = deleteResource(rbdExamplePath + "snapshotclass.yaml")
				if err != nil {
					framework.Failf("failed to delete snapshotclass: %v", err)
				}
				// validate images in trash
				err = waitToRemoveImagesFromTrash(f, clonePool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", clonePool, err)
				}
				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
				}

				err = deletePool(clonePool, false, f)
				if err != nil {
					framework.Failf("failed to delete pool %s: %v", clonePool, err)
				}
			})

			By("create ROX PVC clone and mount it to multiple pods", func() {
				err := createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				// delete pod as we should not create snapshot for in-use pvc
				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				// create clone PVC as ROX
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				// parent pvc+ snapshot + clone
				totalImages = 3
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 2, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
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
						framework.Failf("failed to create application: %v", err)
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
						framework.Failf(stdErr)
					}
				}

				// delete app
				for i := 0; i < totalCount; i++ {
					name := fmt.Sprintf("%s%d", f.UniqueName, i)
					appClone.Name = name
					err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						framework.Failf("failed to delete application: %v", err)
					}
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("validate PVC mounting if snapshot and parent PVC are deleted", func() {
				err := createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				// delete parent PVC
				err = deletePVCAndApp("", f, pvc, app)
				if err != nil {
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

				// create clone PVC
				pvcClone.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images = snapshot + clone
				totalImages = 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}

				// validate created backend rbd images = clone
				totalImages = 1
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)

				appClone, err := loadApp(appClonePath)
				if err != nil {
					framework.Failf("failed to load application: %v", err)
				}
				appClone.Namespace = f.UniqueName
				appClone.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name

				// create application
				err = createApp(f.ClientSet, appClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create application: %v", err)
				}

				err = deletePod(appClone.Name, appClone.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)
			})

			By(
				"validate PVC mounting if snapshot and parent PVC are deleted chained with depth 2",
				func() {
					snapChainDepth := 2

					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
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
						framework.Failf("failed to create storageclass: %v", err)
					}

					err = createRBDSnapshotClass(f)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}

					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
						err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
						if err != nil {
							framework.Failf("failed to create storageclass: %v", err)
						}
					}()

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
					err = createPVCAndApp("", f, pvc, app, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC and application: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 1, defaultRBDPool)
					validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
					for i := 0; i < snapChainDepth; i++ {
						var pvcClone *v1.PersistentVolumeClaim
						snap := getSnapshot(snapshotPath)
						snap.Name = fmt.Sprintf("%s-%d", snap.Name, i)
						snap.Namespace = f.UniqueName
						snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

						err = createSnapshot(&snap, deployTimeout)
						if err != nil {
							framework.Failf("failed to create snapshot: %v", err)
						}
						// validate created backend rbd images
						// parent PVC + snapshot
						totalImages := 2
						validateRBDImageCount(f, totalImages, defaultRBDPool)
						validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
						validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)
						pvcClone, err = loadPVC(pvcClonePath)
						if err != nil {
							framework.Failf("failed to load PVC: %v", err)
						}

						// delete parent PVC
						err = deletePVCAndApp("", f, pvc, app)
						if err != nil {
							framework.Failf("failed to delete PVC and application: %v", err)
						}
						// validate created backend rbd images
						validateRBDImageCount(f, 1, defaultRBDPool)
						validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
						validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

						// create clone PVC
						pvcClone.Name = fmt.Sprintf("%s-%d", pvcClone.Name, i)
						pvcClone.Namespace = f.UniqueName
						pvcClone.Spec.DataSource.Name = snap.Name
						err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
						if err != nil {
							framework.Failf("failed to create PVC: %v", err)
						}
						// validate created backend rbd images = snapshot + clone
						totalImages = 2
						validateRBDImageCount(f, totalImages, defaultRBDPool)
						validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
						validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

						// delete snapshot
						err = deleteSnapshot(&snap, deployTimeout)
						if err != nil {
							framework.Failf("failed to delete snapshot: %v", err)
						}

						// validate created backend rbd images = clone
						totalImages = 1
						validateRBDImageCount(f, totalImages, defaultRBDPool)
						validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
						validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)

						app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
						// create application
						err = createApp(f.ClientSet, app, deployTimeout)
						if err != nil {
							framework.Failf("failed to create application: %v", err)
						}

						pvc = pvcClone
					}

					err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
					if err != nil {
						framework.Failf("failed to delete application: %v", err)
					}
					// delete PVC clone
					err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
					if err != nil {
						framework.Failf("failed to delete PVC: %v", err)
					}
					// validate created backend rbd images
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)
				})

			By("validate PVC Clone chained with depth 2", func() {
				cloneChainDepth := 2

				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
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
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				}()

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
				err = createPVCAndApp("", f, pvc, app, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				for i := 0; i < cloneChainDepth; i++ {
					var pvcClone *v1.PersistentVolumeClaim
					pvcClone, err = loadPVC(pvcSmartClonePath)
					if err != nil {
						framework.Failf("failed to load PVC: %v", err)
					}

					// create clone PVC
					pvcClone.Name = fmt.Sprintf("%s-%d", pvcClone.Name, i)
					pvcClone.Namespace = f.UniqueName
					pvcClone.Spec.DataSource.Name = pvc.Name
					err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
					if err != nil {
						framework.Failf("failed to create PVC: %v", err)
					}

					// delete parent PVC
					err = deletePVCAndApp("", f, pvc, app)
					if err != nil {
						framework.Failf("failed to delete PVC and application: %v", err)
					}

					app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcClone.Name
					// create application
					err = createApp(f.ClientSet, app, deployTimeout)
					if err != nil {
						framework.Failf("failed to create application: %v", err)
					}

					pvc = pvcClone
				}

				err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete application: %v", err)
				}
				// delete PVC clone
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("ensuring all operations will work within a rados namespace", func() {
				updateConfigMap := func(radosNS string) {
					radosNamespace = radosNS
					err := deleteConfigMap(rbdDirPath)
					if err != nil {
						framework.Failf("failed to delete configmap:: %v", err)
					}
					err = createConfigMap(rbdDirPath, f.ClientSet, f)
					if err != nil {
						framework.Failf("failed to create configmap: %v", err)
					}
					err = createRadosNamespace(f)
					if err != nil {
						framework.Failf("failed to create rados namespace: %v", err)
					}
					// delete csi pods
					err = deletePodWithLabel("app in (ceph-csi-rbd, csi-rbdplugin, csi-rbdplugin-provisioner)",
						cephCSINamespace, false)
					if err != nil {
						framework.Failf("failed to delete pods with labels: %v", err)
					}
					// wait for csi pods to come up
					err = waitForDaemonSets(rbdDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
					if err != nil {
						framework.Failf("timeout waiting for daemonset pods: %v", err)
					}
					err = waitForDeploymentComplete(f.ClientSet, rbdDeploymentName, cephCSINamespace, deployTimeout)
					if err != nil {
						framework.Failf("timeout waiting for deployment to be in running state: %v", err)
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
					framework.Failf("failed to create user %s: %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceProvisionerSecretName, keyringRBDNamespaceProvisionerUsername, key)
				if err != nil {
					framework.Failf("failed to create provisioner secret: %v", err)
				}
				// create rbd plugin secret
				key, err = createCephUser(
					f,
					keyringRBDNamespaceNodePluginUsername,
					rbdNodePluginCaps(defaultRBDPool, radosNamespace))
				if err != nil {
					framework.Failf("failed to create user %s: %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = createRBDSecret(f, rbdNamespaceNodePluginSecretName, keyringRBDNamespaceNodePluginUsername, key)
				if err != nil {
					framework.Failf("failed to create node secret: %v", err)
				}

				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
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
					framework.Failf("failed to create storageclass: %v", err)
				}

				err = validateImageOwner(pvcPath, f)
				if err != nil {
					framework.Failf("failed to validate owner of pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				// Create a PVC and bind it to an app within the namespace
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}

				// Resize Block PVC and check Device size within the namespace
				err = resizePVCAndValidateSize(rawPvcPath, rawAppPath, f)
				if err != nil {
					framework.Failf("failed to resize block PVC: %v", err)
				}

				// Resize Filesystem PVC and check application directory size
				err = resizePVCAndValidateSize(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to resize filesystem PVC %v", err)
				}

				// Create a PVC clone and bind it to an app within the namespace
				err = createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				pvc, pvcErr := loadPVC(pvcPath)
				if pvcErr != nil {
					framework.Failf("failed to load PVC: %v", pvcErr)
				}

				pvc.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				validateRBDImageCount(f, 2, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

				err = validatePVCAndAppBinding(pvcClonePath, appClonePath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}
				// as snapshot is deleted the image count should be one
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)

				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", rbdOptions(defaultRBDPool), err)
				}

				// delete RBD provisioner secret
				err = deleteCephUser(f, keyringRBDNamespaceProvisionerUsername)
				if err != nil {
					framework.Failf("failed to delete user %s: %v", keyringRBDNamespaceProvisionerUsername, err)
				}
				err = c.CoreV1().
					Secrets(cephCSINamespace).
					Delete(context.TODO(), rbdNamespaceProvisionerSecretName, metav1.DeleteOptions{})
				if err != nil {
					framework.Failf("failed to delete provisioner secret: %v", err)
				}
				// delete RBD plugin secret
				err = deleteCephUser(f, keyringRBDNamespaceNodePluginUsername)
				if err != nil {
					framework.Failf("failed to delete user %s: %v", keyringRBDNamespaceNodePluginUsername, err)
				}
				err = c.CoreV1().
					Secrets(cephCSINamespace).
					Delete(context.TODO(), rbdNamespaceNodePluginSecretName, metav1.DeleteOptions{})
				if err != nil {
					framework.Failf("failed to delete node secret: %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				updateConfigMap("")
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
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)

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
					framework.Failf("failed to delete PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a PVC and Bind it to an app for mapped rbd image with options", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, map[string]string{
					"imageFeatures": "exclusive-lock",
					"mapOptions":    "lock_on_read,queue_depth=1024",
					"unmapOptions":  "force",
				}, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validatePVCAndAppBinding(pvcPath, appPath, f)
				if err != nil {
					framework.Failf("failed to validate pvc and application binding: %v", err)
				}
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			By("validate the functionality of controller", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass : %v", err)
				}
				scParams := map[string]string{
					"volumeNamePrefix": "test-",
				}
				err = validateController(f,
					pvcPath, appPath, rbdExamplePath+"storageclass.yaml",
					nil,
					scParams)
				if err != nil {
					framework.Failf("failed to validate controller : %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass : %v", err)
				}
			})

			By("validate image deletion when it is moved to trash", func() {
				// make sure pool is empty
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				err := createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load pvc: %v", err)
				}
				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}

				pvcSmartClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load pvcSmartClone: %v", err)
				}
				pvcSmartClone.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}

				smartCloneImageData, err := getImageInfoFromPVC(pvcSmartClone.Namespace, pvcSmartClone.Name, f)
				if err != nil {
					framework.Failf("failed to get ImageInfo from pvc: %v", err)
				}

				imageList, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					framework.Failf("failed to list rbd images: %v", err)
				}
				for _, imageName := range imageList {
					if imageName == smartCloneImageData.imageName {
						// do not move smartclone image to trash to test
						// temporary image clone cleanup.
						continue
					}
					_, _, err = execCommandInToolBoxPod(f,
						fmt.Sprintf("rbd snap purge %s %s", rbdOptions(defaultRBDPool), imageName), rookNamespace)
					if err != nil {
						framework.Failf(
							"failed to snap purge %s %s: %v",
							imageName,
							rbdOptions(defaultRBDPool),
							err)
					}
					_, _, err = execCommandInToolBoxPod(f,
						fmt.Sprintf("rbd trash move %s %s", rbdOptions(defaultRBDPool), imageName), rookNamespace)
					if err != nil {
						framework.Failf(
							"failed to move rbd image %s %s to trash: %v",
							imageName,
							rbdOptions(defaultRBDPool),
							err)
					}
				}

				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}

				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)

				err = waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in trash %s: %v", rbdOptions(defaultRBDPool), err)
				}
			})

			By("validate stale images in trash", func() {
				err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
				if err != nil {
					framework.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
				}
			})

			By("restore snapshot to a bigger size PVC", func() {
				By("restore snapshot to bigger size pvc", func() {
					err := deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					defer func() {
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
					}()
					err = createRBDSnapshotClass(f)
					if err != nil {
						framework.Failf("failed to create VolumeSnapshotClass: %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()
					// validate filesystem mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						pvcPath,
						appPath,
						snapshotPath,
						pvcClonePath,
						appClonePath)
					if err != nil {
						framework.Failf("failed to validate restore bigger size clone: %v", err)
					}
					// validate block mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						rawPvcPath,
						rawAppPath,
						snapshotPath,
						pvcBlockRestorePath,
						appBlockRestorePath)
					if err != nil {
						framework.Failf("failed to validate restore bigger size clone: %v", err)
					}
				})

				ByFileAndBlockEncryption("restore snapshot to bigger size encrypted PVC with VaultKMS", func(
					_ encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
				) {
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionKMSID": "vault-test",
						"encryptionType":  util.EncryptionTypeString(encType),
					}
					err := createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					defer func() {
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
					}()
					err = createRBDSnapshotClass(f)
					if err != nil {
						framework.Failf("failed to create VolumeSnapshotClass: %v", err)
					}
					defer func() {
						err = deleteRBDSnapshotClass()
						if err != nil {
							framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
						}
					}()
					// validate filesystem mode PVC
					err = validateBiggerPVCFromSnapshot(f,
						pvcPath,
						appPath,
						snapshotPath,
						pvcClonePath,
						appClonePath)
					if err != nil {
						framework.Failf("failed to validate restore bigger size clone: %v", err)
					}
					if encType != util.EncryptionTypeFile {
						// validate block mode PVC
						err = validateBiggerPVCFromSnapshot(f,
							rawPvcPath,
							rawAppPath,
							snapshotPath,
							pvcBlockRestorePath,
							appBlockRestorePath)
						if err != nil {
							framework.Failf("failed to validate restore bigger size clone: %v", err)
						}
					}
				})

				By("validate image deletion", func() {
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
					err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
					if err != nil {
						framework.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
					}
				})
			})

			By("clone PVC to a bigger size PVC", func() {
				ByFileAndBlockEncryption("clone PVC to bigger size encrypted PVC with VaultKMS", func(
					validator encryptionValidateFunc, _ validateFunc, encType util.EncryptionType,
				) {
					scOpts := map[string]string{
						"encrypted":       "true",
						"encryptionType":  util.EncryptionTypeString(encType),
						"encryptionKMSID": "vault-test",
					}
					err := createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, scOpts, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					defer func() {
						err = deleteResource(rbdExamplePath + "storageclass.yaml")
						if err != nil {
							framework.Failf("failed to delete storageclass: %v", err)
						}
					}()

					// validate filesystem mode PVC
					err = validateBiggerCloneFromPVC(f,
						pvcPath,
						appPath,
						pvcSmartClonePath,
						appSmartClonePath)
					if err != nil {
						framework.Failf("failed to validate bigger size clone: %v", err)
					}
					if encType != util.EncryptionTypeFile {
						// validate block mode PVC
						err = validateBiggerCloneFromPVC(f,
							rawPvcPath,
							rawAppPath,
							pvcBlockSmartClonePath,
							appBlockSmartClonePath)
						if err != nil {
							framework.Failf("failed to validate bigger size clone: %v", err)
						}
					}
				})

				By("clone PVC to bigger size pvc", func() {
					err := createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
					// validate filesystem mode PVC
					err = validateBiggerCloneFromPVC(f,
						pvcPath,
						appPath,
						pvcSmartClonePath,
						appSmartClonePath)
					if err != nil {
						framework.Failf("failed to validate bigger size clone: %v", err)
					}
					// validate block mode PVC
					err = validateBiggerCloneFromPVC(f,
						rawPvcPath,
						rawAppPath,
						pvcBlockSmartClonePath,
						appBlockSmartClonePath)
					if err != nil {
						framework.Failf("failed to validate bigger size clone: %v", err)
					}
				})

				By("validate image deletion", func() {
					validateRBDImageCount(f, 0, defaultRBDPool)
					validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
					err := waitToRemoveImagesFromTrash(f, defaultRBDPool, deployTimeout)
					if err != nil {
						framework.Failf("failed to validate rbd images in pool %s trash: %v", defaultRBDPool, err)
					}
				})
			})

			By("validate rbd image stripe", func() {
				stripeUnit := 4096
				stripeCount := 8
				objectSize := 131072
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}

				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{
						"stripeUnit":  fmt.Sprintf("%d", stripeUnit),
						"stripeCount": fmt.Sprintf("%d", stripeCount),
						"objectSize":  fmt.Sprintf("%d", objectSize),
					},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteResource(rbdExamplePath + "storageclass.yaml")
					if err != nil {
						framework.Failf("failed to delete storageclass: %v", err)
					}
					err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
					if err != nil {
						framework.Failf("failed to create storageclass: %v", err)
					}
				}()

				err = createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
					if err != nil {
						framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
					}
				}()

				// create PVC and bind it to an app
				pvc, err := loadPVC(pvcPath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				pvc.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC and application: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 1, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				err = validateStripe(f, pvc, stripeUnit, stripeCount, objectSize)
				if err != nil {
					framework.Failf("failed to validate stripe: %v", err)
				}

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				// parent PVC + snapshot
				totalImages := 2
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}

				// create clone PVC as ROX
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				// validate created backend rbd images
				// parent pvc + snapshot + clone
				totalImages = 3
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 2, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)
				err = validateStripe(f, pvcClone, stripeUnit, stripeCount, objectSize)
				if err != nil {
					framework.Failf("failed to validate stripe for clone: %v", err)
				}
				// delete snapshot
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}
				// delete clone pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}

				pvcSmartClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load pvcSmartClone: %v", err)
				}
				pvcSmartClone.Namespace = f.UniqueName

				err = createPVCAndvalidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create pvc: %v", err)
				}
				// validate created backend rbd images
				// parent pvc + temp clone + clone
				totalImages = 3
				validateRBDImageCount(f, totalImages, defaultRBDPool)
				validateOmapCount(f, 2, rbdType, defaultRBDPool, volumesType)
				err = validateStripe(f, pvcSmartClone, stripeUnit, stripeCount, objectSize)
				if err != nil {
					framework.Failf("failed to validate stripe for clone: %v", err)
				}
				// delete parent pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}

				// delete clone pvc
				err = deletePVCAndValidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete PVC: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
			})

			By("create a PVC and check PVC/PV metadata on RBD image after setmetadata is set to false", func() {
				err := createRBDSnapshotClass(f)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				defer func() {
					err = deleteRBDSnapshotClass()
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
				imageList, err := listRBDImages(f, defaultRBDPool)
				if err != nil {
					framework.Failf("failed to list rbd images: %v", err)
				}

				pvcName, stdErr, err := execCommandInToolBoxPod(f,
					formatImageMetaGetCmd(defaultRBDPool, imageList[0], pvcNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PVC name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNameKey, err, stdErr)
				}
				pvcName = strings.TrimSuffix(pvcName, "\n")
				if pvcName != pvc.Name {
					framework.Failf("expected pvcName %q got %q", pvc.Name, pvcName)
				}

				pvcNamespace, stdErr, err := execCommandInToolBoxPod(f,
					formatImageMetaGetCmd(defaultRBDPool, imageList[0], pvcNamespaceKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PVC namespace %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvcNamespaceKey, err, stdErr)
				}
				pvcNamespace = strings.TrimSuffix(pvcNamespace, "\n")
				if pvcNamespace != pvc.Namespace {
					framework.Failf("expected pvcNamespace %q got %q", pvc.Namespace, pvcNamespace)
				}
				pvcObj, err := getPersistentVolumeClaim(c, pvc.Namespace, pvc.Name)
				if err != nil {
					framework.Logf("error getting pvc %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				if pvcObj.Spec.VolumeName == "" {
					framework.Logf("pv name is empty %q in namespace %q: %v", pvc.Name, pvc.Namespace, err)
				}
				pvName, stdErr, err := execCommandInToolBoxPod(f,
					formatImageMetaGetCmd(defaultRBDPool, imageList[0], pvNameKey),
					rookNamespace)
				if err != nil || stdErr != "" {
					framework.Failf("failed to get PV name %s/%s %s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageList[0], pvNameKey, err, stdErr)
				}
				pvName = strings.TrimSuffix(pvName, "\n")
				if pvName != pvcObj.Spec.VolumeName {
					framework.Failf("expected pvName %q got %q", pvcObj.Spec.VolumeName, pvName)
				}

				checkClusternameInMetadata(f, rookNamespace, defaultRBDPool, imageList[0])

				snap := getSnapshot(snapshotPath)
				snap.Namespace = f.UniqueName
				snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

				err = createSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to create snapshot: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 2, defaultRBDPool)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 1, rbdType, defaultRBDPool, snapsType)

				// wait for cluster name update in deployment
				containers := []string{"csi-rbdplugin", "csi-rbdplugin-controller"}
				err = waitForContainersArgsUpdate(c, cephCSINamespace, rbdDeploymentName,
					"setmetadata", "false", containers, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment update %s/%s: %v", cephCSINamespace, rbdDeploymentName, err)
				}
				pvcSmartClone, err := loadPVC(pvcSmartClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcSmartClone.Spec.DataSource.Name = pvc.Name
				pvcSmartClone.Namespace = f.UniqueName
				err = createPVCAndvalidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				_, smartPV, err := getPVCAndPV(f.ClientSet, pvcSmartClone.Name, pvcSmartClone.Namespace)
				imageName := smartPV.Spec.CSI.VolumeAttributes["imageName"]
				// make sure we had unset the PVC metadata on the rbd image created
				// for the snapshot
				pvcName, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageName, pvcNameKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PVC name found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageName, pvcNameKey, pvcName, err, stdErr)
				}
				pvcNamespace, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageName, pvcNamespaceKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PVC namespace found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageName, pvcNamespaceKey, pvcNamespace, err, stdErr)
				}
				pvName, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageName, pvNameKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PV name found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageName, pvNameKey, pvName, err, stdErr)
				}
				err = deletePVCAndValidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}

				// Test Restore snapshot
				pvcClone, err := loadPVC(pvcClonePath)
				if err != nil {
					framework.Failf("failed to load PVC: %v", err)
				}
				pvcClone.Namespace = f.UniqueName
				pvcClone.Spec.DataSource.Name = snap.Name
				err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to create PVC: %v", err)
				}
				_, restorePV, err := getPVCAndPV(f.ClientSet, pvcClone.Name, pvcClone.Namespace)
				imageName = restorePV.Spec.CSI.VolumeAttributes["imageName"]
				// make sure we had unset the PVC metadata on the rbd image created
				// for the snapshot
				pvcName, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageName, pvcNameKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PVC name found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageName, pvcNameKey, pvcName, err, stdErr)
				}
				pvcNamespace, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageName, pvcNamespaceKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PVC namespace found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageName, pvcNamespaceKey, pvcNamespace, err, stdErr)
				}
				pvName, stdErr, err = execCommandInToolBoxPod(f,
					fmt.Sprintf("rbd image-meta get %s --image=%s %s",
						rbdOptions(defaultRBDPool), imageName, pvNameKey),
					rookNamespace)
				if checkGetKeyError(err, stdErr) {
					framework.Failf("PV name found on %s/%s %s=%s: err=%v stdErr=%q",
						rbdOptions(defaultRBDPool), imageName, pvNameKey, pvName, err, stdErr)
				}
				err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete pvc: %v", err)
				}
				err = deleteSnapshot(&snap, deployTimeout)
				if err != nil {
					framework.Failf("failed to delete snapshot: %v", err)
				}
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)
				// wait for cluster name update in deployment
				err = waitForContainersArgsUpdate(c, cephCSINamespace, rbdDeploymentName,
					"setmetadata", "true", containers, deployTimeout)
				if err != nil {
					framework.Failf("timeout waiting for deployment update %s/%s: %v", cephCSINamespace, rbdDeploymentName, err)
				}
			})

			By("create a PVC and bind it to an app with encrypted RBD volume (default type setting)", func() {
				err := deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(
					f.ClientSet,
					f,
					defaultSCName,
					nil,
					map[string]string{"encrypted": "true"},
					deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
				err = validateEncryptedPVCAndAppBinding(pvcPath, appPath, noKMS, f)
				if err != nil {
					framework.Failf("failed to validate encrypted pvc: %v", err)
				}
				// validate created backend rbd images
				validateRBDImageCount(f, 0, defaultRBDPool)
				validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
				err = deleteResource(rbdExamplePath + "storageclass.yaml")
				if err != nil {
					framework.Failf("failed to delete storageclass: %v", err)
				}
				err = createRBDStorageClass(f.ClientSet, f, defaultSCName, nil, nil, deletePolicy)
				if err != nil {
					framework.Failf("failed to create storageclass: %v", err)
				}
			})

			// delete RBD provisioner secret
			err := deleteCephUser(f, keyringRBDProvisionerUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringRBDProvisionerUsername, err)
			}
			// delete RBD plugin secret
			err = deleteCephUser(f, keyringRBDNodePluginUsername)
			if err != nil {
				framework.Failf("failed to delete user %s: %v", keyringRBDNodePluginUsername, err)
			}

			// Make sure this should be last testcase in this file, because
			// it deletes pool
			By("Create a PVC and delete PVC when backend pool deleted", func() {
				err := pvcDeleteWhenPoolNotFound(pvcPath, false, f)
				if err != nil {
					framework.Failf("failed to delete PVC when pool not found: %v", err)
				}
			})
		})
	})
})
