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
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"
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

	f := framework.NewDefaultFramework("nvmeof")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged

	// is set during BeforeEach(), f.UniqueName is empty here
	var nvmeofStorageClass string

	ginkgo.BeforeEach(func() {
		if !deployNVMeoF {
			return
		}

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
	})

	ginkgo.AfterEach(func() {
		if !deployNVMeoF {
			return
		}

		deleteNVMeoFPlugin()
		deleteGateway(f)
		deleteNVMeofStorageClass(f, nvmeofStorageClass)
	})

	ginkgo.Context("Test NVMe CSI", func() {

		pvcPath := nvmeofExamplePath + "pvc.yaml"

		ginkgo.It("create a PVC and delete it", func() {
			ginkgo.By("prepare PVC")
			pvc, err := loadPVC(pvcPath)
			Expect(err).ShouldNot(HaveOccurred())

			pvc.Namespace = f.UniqueName
			pvc.Spec.StorageClassName = &nvmeofStorageClass

			ginkgo.By("create the PVC")
			err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())

			validateRBDImageCount(f, 1, nvmeofPool)
			validateOmapCount(f, 1, rbdType, nvmeofPool, volumesType)

			ginkgo.By("delete the PVC again")
			err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())

			// validate created backend rbd images
			validateRBDImageCount(f, 0, nvmeofPool)
			validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
		})
	})
})
