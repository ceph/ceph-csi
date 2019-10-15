/*
Copyright 2018 The Ceph-CSI Authors.

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
	"flag"
	"fmt"
	"log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	RookVersion   string
	rookRequired  bool
	deployTimeout int
)

func init() {
	log.SetOutput(GinkgoWriter)
	flag.StringVar(&RookVersion, "rook-version", "master", "rook version to pull yaml files")

	flag.BoolVar(&rookRequired, "deploy-rook", true, "deploy rook on kubernetes")
	flag.IntVar(&deployTimeout, "deploy-timeout", 10, "timeout to wait for created kubernetes resources")
	// Register framework flags, then handle flags
	framework.HandleFlags()
	framework.AfterReadingAllFlags(&framework.TestContext)

	formRookURL(RookVersion)
	fmt.Println("timeout for deploytimeout ", deployTimeout)
}

// removeCephCSIResource is a temporary fix for CI to remove the ceph-csi resources deployed by rook
func removeCephCSIResource() {
	// cleanup rbd and cephfs deamonset deployed by rook
	_, err := framework.RunKubectl("delete", "-nrook-ceph", "daemonset", "csi-cephfsplugin")
	if err != nil {
		e2elog.Logf("failed to delete rbd daemonset %v", err)
	}
	_, err = framework.RunKubectl("delete", "-nrook-ceph", "daemonset", "csi-rbdplugin")
	if err != nil {
		e2elog.Logf("failed to delete cephfs daemonset %v", err)
	}

	// cleanup rbd and cephfs statefulset deployed by rook
	_, err = framework.RunKubectl("delete", "-nrook-ceph", "statefulset", "csi-rbdplugin-provisioner")
	if err != nil {
		e2elog.Logf("failed to delete rbd statefulset %v", err)
	}
	_, err = framework.RunKubectl("delete", "-nrook-ceph", "statefulset", "csi-cephfsplugin-provisioner")
	if err != nil {
		e2elog.Logf("failed to delete cephfs statefulset %v", err)
	}

	// cleanup rbd cluster roles deployed by rook
	rbdPath := fmt.Sprintf("%s/%s/", rbdDirPath, "v1.13")
	_, err = framework.RunKubectl("delete", "--ignore-not-found", "-f", rbdPath+rbdProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to delete provisioner rbac %v", err)
	}
	_, err = framework.RunKubectl("delete", "--ignore-not-found", "-f", rbdPath+rbdNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin rbac %v", err)
	}

	// cleanup cephfs cluster roles deployed by rook
	cephfsPath := fmt.Sprintf("%s/%s/", cephfsDirPath, "v1.13")
	_, err = framework.RunKubectl("delete", "--ignore-not-found", "-f", cephfsPath+cephfsProvisionerRBAC)
	if err != nil {
		e2elog.Logf("failed to delete provisioner rbac %v", err)
	}
	_, err = framework.RunKubectl("delete", "--ignore-not-found", "-f", cephfsPath+cephfsNodePluginRBAC)
	if err != nil {
		e2elog.Logf("failed to delete nodeplugin rbac %v", err)
	}
}

// BeforeSuite deploys the rook-operator and ceph cluster
var _ = BeforeSuite(func() {
	if rookRequired {
		deployRook()
		removeCephCSIResource()
	}
})

// AfterSuite removes the rook-operator and ceph cluster
var _ = AfterSuite(func() {
	if rookRequired {
		tearDownRook()
	}
})

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
