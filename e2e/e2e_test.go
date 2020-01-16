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
)

var (
	deployTimeout int
)

func init() {
	log.SetOutput(GinkgoWriter)

	flag.IntVar(&deployTimeout, "deploy-timeout", 10, "timeout to wait for created kubernetes resources")

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
