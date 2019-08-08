package e2e

import (
	"flag"
	"fmt"
	"log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	RookVersion    string
	rookRequired   bool
	cephFsRequired bool
	rbdRequired    bool
	csiRequired    bool
	deployTimeout  int
)

func init() {
	log.SetOutput(GinkgoWriter)
	flag.StringVar(&RookVersion, "rook-version", "master", "rook version to pull yaml files")

	flag.BoolVar(&rookRequired, "deploy-rook", true, "deploy rook on kubernetes")
	flag.BoolVar(&csiRequired, "ceph-csi", true, "deploy ceph-csi on rook")
	flag.BoolVar(&cephFsRequired, "cephfs-csi", false, "deploy cephfs-csi on rook")
	flag.BoolVar(&rbdRequired, "rbd-csi", false, "deploy rbd-csi rook")
	flag.IntVar(&deployTimeout, "deploy-timeout", 10, "timeout to wait for created kubernetes resources")
	// Register framework flags, then handle flags
	framework.HandleFlags()
	framework.AfterReadingAllFlags(&framework.TestContext)

	formRookURL(RookVersion)
	fmt.Println("timeout for deploytimeout ", deployTimeout)
}

// BeforeSuite deploys the rook-operator and ceph cluster
var _ = BeforeSuite(func() {
	if rookRequired {
		deployRook()
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
