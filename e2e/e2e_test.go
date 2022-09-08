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
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
)

func init() {
	log.SetOutput(GinkgoWriter)

	flag.IntVar(&deployTimeout, "deploy-timeout", 10, "timeout to wait for created kubernetes resources")
	flag.BoolVar(&deployCephFS, "deploy-cephfs", true, "deploy cephFS csi driver")
	flag.BoolVar(&deployRBD, "deploy-rbd", true, "deploy rbd csi driver")
	flag.BoolVar(&deployNFS, "deploy-nfs", false, "deploy nfs csi driver")
	flag.BoolVar(&testCephFS, "test-cephfs", true, "test cephFS csi driver")
	flag.BoolVar(&testCephFSFscrypt, "test-cephfs-fscrypt", false, "test CephFS csi driver fscrypt support")
	flag.BoolVar(&testRBD, "test-rbd", true, "test rbd csi driver")
	flag.BoolVar(&testRBDFSCrypt, "test-rbd-fscrypt", false, "test rbd csi driver fscrypt support")
	flag.BoolVar(&testNBD, "test-nbd", false, "test rbd csi driver with rbd-nbd mounter")
	flag.BoolVar(&testNFS, "test-nfs", false, "test nfs csi driver")
	flag.BoolVar(&helmTest, "helm-test", false, "tests running on deployment via helm")
	flag.BoolVar(&upgradeTesting, "upgrade-testing", false, "perform upgrade testing")
	flag.StringVar(&upgradeVersion, "upgrade-version", "v3.5.1", "target version for upgrade testing")
	flag.StringVar(&cephCSINamespace, "cephcsi-namespace", defaultNs, "namespace in which cephcsi deployed")
	flag.StringVar(&rookNamespace, "rook-namespace", "rook-ceph", "namespace in which rook is deployed")
	flag.BoolVar(&isOpenShift, "is-openshift", false, "disables certain checks on OpenShift")
	flag.StringVar(&fileSystemName, "filesystem", "myfs", "CephFS filesystem to use")
	flag.StringVar(&clusterID, "clusterid", "", "Ceph cluster ID to use (defaults to `ceph fsid` detection)")
	flag.StringVar(&nfsDriverName, "nfs-driver", "nfs.csi.ceph.com", "name of the driver for NFS-volumes")
	setDefaultKubeconfig()

	// Register framework flags, then handle flags
	handleFlags()
	framework.AfterReadingAllFlags(&framework.TestContext)

	fmt.Println("timeout for deploytimeout ", deployTimeout)
}

func setDefaultKubeconfig() {
	_, exists := os.LookupEnv("KUBECONFIG")
	if !exists {
		defaultKubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv("KUBECONFIG", defaultKubeconfig)
	}
}

func TestE2E(t *testing.T) {
	t.Parallel()
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

func handleFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	testing.Init()
	flag.Parse()

	// testNFS will automatically be enabled when testCephFS is enabled,
	// this makes sure the NFS tests run in the CI where there are
	// different jobs for CephFS and RBD. With a dedicated testNFS
	// variable, it is still possible to only run the NFS tests, when both
	// CephFS and RBD are disabled.
	if testCephFS {
		testNFS = testCephFS
		deployNFS = deployCephFS
	}
}
