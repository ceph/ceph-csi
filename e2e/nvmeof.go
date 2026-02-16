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

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
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
		if version.GetMajor() < CephMajorTentacle {
			deployNVMeoF = false
			ginkgo.Skip("Skipping NVMe-oF E2E, requires Ceph 20 (Tentacle):" + version.String())
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
	})
})
