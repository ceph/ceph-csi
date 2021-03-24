package e2e

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
)

func init() {
	log.SetOutput(GinkgoWriter)

	flag.IntVar(&deployTimeout, "deploy-timeout", 10, "timeout to wait for created kubernetes resources")
	flag.BoolVar(&deployCephFS, "deploy-cephfs", true, "deploy cephfs csi driver")
	flag.BoolVar(&deployRBD, "deploy-rbd", true, "deploy rbd csi driver")
	flag.BoolVar(&testCephFS, "test-cephfs", true, "test cephfs csi driver")
	flag.BoolVar(&testRBD, "test-rbd", true, "test rbd csi driver")
	flag.BoolVar(&upgradeTesting, "upgrade-testing", false, "perform upgrade testing")
	flag.StringVar(&upgradeVersion, "upgrade-version", "v2.1.2", "target version for upgrade testing")
	flag.StringVar(&cephCSINamespace, "cephcsi-namespace", defaultNs, "namespace in which cephcsi deployed")
	flag.StringVar(&rookNamespace, "rook-namespace", "rook-ceph", "namespace in which rook is deployed")
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

var _ = BeforeSuite(func() {

})

var _ = AfterSuite(func() {

})

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

func handleFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	testing.Init()
	flag.Parse()
	initResouces()
}
