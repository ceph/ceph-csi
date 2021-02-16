package e2e

import (
	"context"
	"strings"

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
	// hack to make helm E2E pass as helm charts creates this configmap as part
	// of cephcsi deployment
	_, err := framework.RunKubectl(cephCSINamespace, "delete", "cm", "ceph-csi-encryption-kms-config", "--namespace", cephCSINamespace, "--ignore-not-found=true")
	Expect(err).Should(BeNil())

	createORDeleteVault("create")
	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}

	pods, err := c.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
	Expect(err).Should(BeNil())
	Expect(len(pods.Items)).Should(Equal(1))
	name := pods.Items[0].Name
	err = waitForPodInRunningState(name, cephCSINamespace, c, deployTimeout)
	Expect(err).Should(BeNil())
}

func deleteVault() {
	createORDeleteVault("delete")
}

func createORDeleteVault(action string) {
	data, err := replaceNamespaceInTemplate(vaultExamplePath + vaultServicePath)
	if err != nil {
		e2elog.Failf("failed to read content from %s %v", vaultExamplePath+vaultServicePath, err)
	}

	data = strings.ReplaceAll(data, "vault.default", "vault."+cephCSINamespace)

	data = strings.ReplaceAll(data, "value: default", "value: "+cephCSINamespace)
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s vault statefulset %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(vaultExamplePath + vaultRBACPath)
	if err != nil {
		e2elog.Failf("failed to read content from %s %v", vaultExamplePath+vaultRBACPath, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s vault statefulset %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(vaultExamplePath + vaultConfigPath)
	if err != nil {
		e2elog.Failf("failed to read content from %s %v", vaultExamplePath+vaultConfigPath, err)
	}
	data = strings.ReplaceAll(data, "default", cephCSINamespace)
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s vault configmap %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(vaultExamplePath + vaultPSPPath)
	if err != nil {
		e2elog.Failf("failed to read content from %s %v", vaultExamplePath+vaultPSPPath, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, action, ns, "-f", "-")
	if err != nil {
		e2elog.Failf("failed to %s vault psp %v", action, err)
	}
}
