package e2e

import (
	"fmt"

	. "github.com/onsi/gomega" // nolint
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

var (
	vaultExamplePath = "../examples/kms/vault/"
	vaultServicePath = "vault.yaml"
	vaultPSPPath     = "vault-psp.yaml"
	vaultRBACPath    = "csi-vaulttokenreview-rbac.yaml"
	vaultConfigPath  = "kms-config.yaml"
)

func deployVault(c kubernetes.Interface, deployTimeout int) {
	framework.RunKubectlOrDie("create", "-f", vaultExamplePath+vaultServicePath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	framework.RunKubectlOrDie("create", "-f", vaultExamplePath+vaultPSPPath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	framework.RunKubectlOrDie("create", "-f", vaultExamplePath+vaultRBACPath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	framework.RunKubectlOrDie("create", "-f", vaultExamplePath+vaultConfigPath, fmt.Sprintf("--namespace=%s", cephCSINamespace))

	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}

	pods, err := c.CoreV1().Pods(cephCSINamespace).List(opt)
	Expect(err).Should(BeNil())
	Expect(len(pods.Items)).Should(Equal(1))
	name := pods.Items[0].Name
	err = waitForPodInRunningState(name, cephCSINamespace, c, deployTimeout)
	Expect(err).Should(BeNil())
}

func deleteVault() {
	_, err := framework.RunKubectl("delete", "-f", vaultExamplePath+vaultServicePath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	if err != nil {
		e2elog.Logf("failed to delete vault statefull set %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", vaultExamplePath+vaultRBACPath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	if err != nil {
		e2elog.Logf("failed to delete vault statefull set %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", vaultExamplePath+vaultConfigPath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	if err != nil {
		e2elog.Logf("failed to delete vault config map %v", err)
	}
	_, err = framework.RunKubectl("delete", "-f", vaultExamplePath+vaultPSPPath, fmt.Sprintf("--namespace=%s", cephCSINamespace))
	if err != nil {
		e2elog.Logf("failed to delete vault psp %v", err)
	}
}
