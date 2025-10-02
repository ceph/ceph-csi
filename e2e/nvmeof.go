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
	. "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/pod-security-admission/api"
)

const (
	nvmeofPool = "nvmeofpool"
)

var _ = Describe("nvmeof", func() {
	f := framework.NewDefaultFramework("nvmeof")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged

	BeforeEach(func() {
		if !testNVMeoF {
			Skip("Skipping NVMe-oF E2E")
		}

		// Ceph credentials referenced in the StorageClass
		createNVMeoFCredentials(f)

		if deployNVMeoF {
			deployGateway(f, deployTimeout)
			deployNVMeoFPlugin(f, deployTimeout)
		}
	})

	AfterEach(func() {
		if !testNVMeoF {
			Skip("Skipping NVMe E2E")
		}

		if deployNVMeoF {
			deleteNVMeoFPlugin()
			deleteGateway(f)
		}
	})

	Context("Test NVMe CSI", func() {
		if !testNVMeoF {
			return
		}

		It("Test NVMe-oF CSI", func() {
			options := map[string]string{}
			params := map[string]string{}
			policy := v1.PersistentVolumeReclaimDelete

			createNVMeoFStorageClass(f, "e2e-nvmeof", options, params, policy)

			Skip("no NVMe-oF test cases yet")
		})
	})
})
