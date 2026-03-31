/*
Copyright 2025 The Ceph-CSI Authors.

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

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edebug "k8s.io/kubernetes/test/e2e/framework/debug"
	"k8s.io/pod-security-admission/api"
)

const (
	nvmeofPool = "nvmeofpool"
)

var _ = ginkgo.Describe("nvmeof", func() {
	if !testNVMeoF {
		framework.Logf("Skipping NVMe-oF E2E")

		return
	}

	// only support deployment through YAML files (for now)
	if helmTest || operatorDeployment {
		framework.Logf("Skipping NVMe-oF E2E (simple deployment only)")

		return
	}

	f := framework.NewDefaultFramework("nvmeof")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged

	// is set during BeforeEach(), f.UniqueName is empty here
	var nvmeofStorageClass string

	ginkgo.BeforeEach(func() {
		if !deployNVMeoF {
			return
		}

		version, err := getCephVersion(f)
		if err != nil {
			logAndFail("failed to get Ceph cluster version: %v", err)
		}
		if !version.GreaterEquals(CephVersionTentacle) {
			deployNVMeoF = false
			ginkgo.Skip("Skipping NVMe-oF E2E, requires Ceph v20 (Tentacle): " + version.String())
		}

		framework.Logf("NVMe-oF testing supported, Ceph version: %s", version)

		// FIXME: gateway should get deployed by Rook
		deployGateway(f, deployTimeout)

		// No need to create the namespace if ceph-csi is deployed via helm or operator.
		if cephCSINamespace != defaultNs && !(helmTest || operatorDeployment) {
			err := createNamespace(f.ClientSet, cephCSINamespace)
			if err != nil {
				logAndFail("failed to create namespace: %v", err)
			}
		}

		// Ceph credentials referenced in the StorageClass
		createNVMeoFCredentials(f)

		// FIXME: use ceph-csi-operator
		deployNVMeoFPlugin(f, deployTimeout)

		// create the StorageClass
		options := map[string]string{}
		params := map[string]string{
			"pool": nvmeofPool,
		}
		policy := v1.PersistentVolumeReclaimDelete

		nvmeofStorageClass = "e2e-" + f.UniqueName + "-sc"
		createNVMeoFStorageClass(f, nvmeofStorageClass, options, params, policy)
	}, ginkgo.OncePerOrdered)

	ginkgo.AfterEach(func() {
		if !deployNVMeoF {
			return
		}

		if ginkgo.CurrentSpecReport().Failed() {
			// log pods created by helm chart
			//logsCSIPods("app="+helmNFSPodsLabel, c)
			// log provisioner
			logsCSIPods("app="+nvmeofDeploymentName, f.ClientSet)
			// log node plugin
			logsCSIPods("app="+nvmeofDaemonsetName, f.ClientSet)

			// Gateway logs - need to search in rook-ceph namespace
			opt := metav1.ListOptions{LabelSelector: "app=ceph-nvmeof-gateway"}
			podList, _ := f.ClientSet.CoreV1().Pods(rookNamespace).List(context.TODO(), opt)
			for i := range podList.Items {
				kubectlLogPod(f.ClientSet, &podList.Items[i])
			}

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), f.ClientSet, cephCSINamespace)
		}

		deleteNVMeoFPlugin()
		deleteGateway(f)
		deleteNVMeofStorageClass(f, nvmeofStorageClass)
	}, ginkgo.OncePerOrdered)

	ginkgo.Context("Test NVMe CSI", ginkgo.Ordered, func() {

		pvcPath := nvmeofExamplePath + "pvc.yaml"
		appPath := nvmeofExamplePath + "pod.yaml"
		rawPvcPath := nvmeofExamplePath + "raw-block-pvc.yaml"
		rawAppPath := nvmeofExamplePath + "raw-block-pod.yaml"

		ginkgo.It("create a PVC and delete it", func() {
			pvc, err := loadPVC(pvcPath)
			Expect(err).ShouldNot(HaveOccurred())

			pvc.Namespace = f.UniqueName
			pvc.Spec.StorageClassName = &nvmeofStorageClass

			err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())

			validateRBDImageCount(f, 1, nvmeofPool)
			validateOmapCount(f, 1, rbdType, nvmeofPool, volumesType)

			err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())

			ginkgo.By("test service account based volume access restriction")
			err = validateServiceAccountVolumeRestriction(
				pvcPath, appPath,
				".rbd.csi.ceph.com/serviceaccount", nvmeofPool,
				&nvmeofStorageClass, f)
			Expect(err).ShouldNot(HaveOccurred())

			// validate created backend rbd images
			validateRBDImageCount(f, 0, nvmeofPool)
			validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
		})

		ginkgo.It("Resize Filesystem PVC and check application directory size", func() {
			pvc, err := loadPVC(pvcPath)
			Expect(err).ShouldNot(HaveOccurred())

			pvc.Namespace = f.UniqueName
			pvc.Spec.StorageClassName = &nvmeofStorageClass

			err = resizePVCAndValidateSize(pvc, appPath, f)
			Expect(err).ShouldNot(HaveOccurred())

			// validate created backend rbd images
			validateRBDImageCount(f, 0, nvmeofPool)
			validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
		})

		ginkgo.It("Resize Block PVC and check Device size", func() {
			pvc, err := loadPVC(rawPvcPath)
			Expect(err).ShouldNot(HaveOccurred())

			pvc.Namespace = f.UniqueName
			pvc.Spec.StorageClassName = &nvmeofStorageClass

			err = resizePVCAndValidateSize(pvc, rawAppPath, f)
			Expect(err).ShouldNot(HaveOccurred())

			// validate created backend rbd images
			validateRBDImageCount(f, 0, nvmeofPool)
			validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
		})

		ginkgo.It("Test GroupLock: Concurrent Create/Delete Pods Only", func() {
			// This test validates the GroupLock implementation in the NVMeoF NodeServer
			// by creating and deleting multiple Pods (not PVCs) concurrently.
			//
			// Test flow:
			// 1. Create 3 PVCs sequentially and validate they're Bound
			// 2. Create 3 Pods concurrently using those PVCs (triggers NodeStage -> Group A lock)
			// 3. Wait for all Pods to be Running
			// 4. Delete all 3 Pods concurrently (triggers NodeUnstage -> Group B lock)
			// 5. Delete all 3 PVCs sequentially
			// 6. Verify no timeouts/deadlocks and all operations succeed
			//
			// This tests GroupLock in the NodeServer without involving ControllerServer operations.
			totalCount := 3

			ginkgo.By("Creating PVCs sequentially")
			pvc, err := loadPVC(pvcPath)
			Expect(err).ShouldNot(HaveOccurred())
			pvc.Namespace = f.UniqueName
			pvc.Spec.StorageClassName = &nvmeofStorageClass

			pvcBaseName := uuid.NewString()
			for i := range totalCount {
				pvcName := fmt.Sprintf("%s-%d", pvcBaseName, i)
				pvcCopy := pvc.DeepCopy()
				pvcCopy.Name = pvcName

				framework.Logf("Creating PVC %d/%d: %s", i+1, totalCount, pvcName)
				err = createPVCAndvalidatePV(f.ClientSet, pvcCopy, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}

			ginkgo.By("Validating backend RBD images were created")
			validateRBDImageCount(f, totalCount, nvmeofPool)
			validateOmapCount(f, totalCount, rbdType, nvmeofPool, volumesType)

			ginkgo.By("Creating Pods concurrently using those PVCs")
			createResult := createConcurrentPods(totalCount, pvcBaseName, 0, appPath, f)

			// Log any errors
			if createResult.HasErrors() {
				createResult.LogErrors()
			}

			// Verify all creations succeeded
			Expect(createResult.failed).To(Equal(0),
				"Expected all %d Pod create operations to succeed, but %d failed",
				totalCount, createResult.failed)

			ginkgo.By("Waiting for all Pods to be Running")
			for i := range totalCount {
				podName := fmt.Sprintf("%s-%d", createResult.uniqueName, i)
				err = waitForPodInRunningState(podName, f.UniqueName, f.ClientSet, deployTimeout, noError)
				Expect(err).ShouldNot(HaveOccurred())
			}

			ginkgo.By("Deleting Pods concurrently")
			deleteResult := deleteConcurrentPods(createResult, f)

			// Log any errors
			if deleteResult.HasErrors() {
				deleteResult.LogErrors()
			}

			// Verify all deletions succeeded
			Expect(deleteResult.failed).To(Equal(0),
				"Expected all %d Pod delete operations to succeed, but %d failed",
				totalCount, deleteResult.failed)

			ginkgo.By("Deleting PVCs sequentially")
			for i := range totalCount {
				pvcName := fmt.Sprintf("%s-%d", pvcBaseName, i)
				pvcCopy := pvc.DeepCopy()
				pvcCopy.Name = pvcName

				framework.Logf("Deleting PVC %d/%d: %s", i+1, totalCount, pvcName)
				err = deletePVCAndValidatePV(f.ClientSet, pvcCopy, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}

			ginkgo.By("Validating all backend RBD images were deleted")
			validateRBDImageCount(f, 0, nvmeofPool)
			validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)

			framework.Logf("GroupLock test passed: %d concurrent Pod creates and %d concurrent Pod deletes completed successfully",
				totalCount, totalCount)
		})

		ginkgo.It("Test GroupLock: Mixed Create/Delete Pods with Rapid Switching", func() {
			// This test validates the GroupLock implementation under rapid switching
			// between Group A (NodeStage) and Group B (NodeUnstage) operations.
			//
			// Test flow:
			// 1. Create 15 PVCs sequentially
			// 2. Create 5 Pods using PVCs 0-4 (Group A)
			// 3. Concurrently: Create 5 Pods using PVCs 5-9 (Group A) + Delete previous 5 Pods (Group B)
			// 4. Concurrently: Create 5 Pods using PVCs 10-14 (Group A) + Delete previous 5 Pods (Group B)
			// 5. Delete final 5 Pods
			// 6. Delete all 15 PVCs sequentially
			//
			// This tests rapid GroupLock switching between Group A and B in the NodeServer only,
			// without involving ControllerServer operations.
			totalCount := 15
			batchSize := 5

			ginkgo.By(fmt.Sprintf("Running Pods-only mixed test: %d total PVCs, batches of %d Pods",
				totalCount, batchSize))

			err := mixedCreateDeletePodsOnly(totalCount, batchSize, pvcPath, appPath, nvmeofStorageClass, f)
			Expect(err).ShouldNot(HaveOccurred(),
				"Mixed Pods-only operations should complete without errors")

			ginkgo.By("Validating all backend RBD images were cleaned up")
			validateRBDImageCount(f, 0, nvmeofPool)
			validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)

			framework.Logf("GroupLock Pods-only test passed: %d Pods created and deleted with rapid Group A/B switching",
				totalCount)
		})
	})
})
