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
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/gomega" //nolint:golint // e2e uses Expect() and other Gomega functions
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var (
	vaultExamplePath     = "../examples/kms/vault/"
	vaultServicePath     = "vault.yaml"
	vaultRBACPath        = "csi-vaulttokenreview-rbac.yaml"
	vaultConfigPath      = "kms-config.yaml"
	vaultTenantPath      = "tenant-sa.yaml"
	vaultTenantAdminPath = "tenant-sa-admin.yaml"
	vaultUserSecret      = "user-secret.yaml"
)

func deployVault(c kubernetes.Interface, deployTimeout int) {
	// hack to make helm E2E pass as helm charts creates this configmap as part
	// of cephcsi deployment
	err := retryKubectlArgs(
		cephCSINamespace,
		kubectlDelete,
		deployTimeout,
		"cm",
		"ceph-csi-encryption-kms-config",
		"--ignore-not-found=true")
	Expect(err).ShouldNot(HaveOccurred())

	createORDeleteVault(kubectlCreate)
	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}

	pods, err := c.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(pods.Items).Should(HaveLen(1))
	name := pods.Items[0].Name
	err = waitForPodInRunningState(name, cephCSINamespace, c, deployTimeout, noError)
	Expect(err).ShouldNot(HaveOccurred())
}

func deleteVault() {
	createORDeleteVault(kubectlDelete)
}

func createORDeleteVault(action kubectlAction) {
	data, err := replaceNamespaceInTemplate(vaultExamplePath + vaultServicePath)
	if err != nil {
		framework.Failf("failed to read content from %s %v", vaultExamplePath+vaultServicePath, err)
	}

	data = strings.ReplaceAll(data, "vault.default", "vault."+cephCSINamespace)

	data = strings.ReplaceAll(data, "value: default", "value: "+cephCSINamespace)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		framework.Failf("failed to %s vault statefulset %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(vaultExamplePath + vaultRBACPath)
	if err != nil {
		framework.Failf("failed to read content from %s %v", vaultExamplePath+vaultRBACPath, err)
	}
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		framework.Failf("failed to %s vault statefulset %v", action, err)
	}

	data, err = replaceNamespaceInTemplate(vaultExamplePath + vaultConfigPath)
	if err != nil {
		framework.Failf("failed to read content from %s %v", vaultExamplePath+vaultConfigPath, err)
	}
	data = strings.ReplaceAll(data, "default", cephCSINamespace)
	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		framework.Failf("failed to %s vault configmap %v", action, err)
	}
}

// createTenantServiceAccount uses the tenant-sa.yaml example file to create
// the ServiceAccount for the tenant and configured Hashicorp Vault with a
// kv-store that the ServiceAccount has access to.
func createTenantServiceAccount(c kubernetes.Interface, ns string) error {
	err := createORDeleteTenantServiceAccount(kubectlCreate, ns)
	if err != nil {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}

	// wait for the Job to finish
	const jobName = "vault-tenant-sa"
	err = waitForJobCompletion(c, cephCSINamespace, jobName, deployTimeout)
	if err != nil {
		return fmt.Errorf("job %s/%s did not succeed: %w", cephCSINamespace, jobName, err)
	}

	return nil
}

// deleteTenantServiceAccount removed the ServiceAccount and other objects that
// were created with createTenantServiceAccount.
func deleteTenantServiceAccount(ns string) {
	err := createORDeleteTenantServiceAccount(kubectlDelete, ns)
	Expect(err).ShouldNot(HaveOccurred())
}

// createORDeleteTenantServiceAccount is a helper that reads the tenant-sa.yaml
// example file and replaces the default namespaces with the current deployment
// configuration.
func createORDeleteTenantServiceAccount(action kubectlAction, ns string) error {
	err := retryKubectlFile(ns, action, vaultExamplePath+vaultTenantPath, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to %s tenant ServiceAccount: %w", action, err)
	}

	// the ServiceAccount needs permissions in Vault, the admin job sets that up
	data, err := replaceNamespaceInTemplate(vaultExamplePath + vaultTenantAdminPath)
	if err != nil {
		return fmt.Errorf("failed to read content from %q: %w", vaultExamplePath+vaultTenantAdminPath, err)
	}

	// replace the value for TENANT_NAMESPACE
	data = strings.ReplaceAll(data, "value: tenant", "value: "+ns)

	// replace "default" in the URL to the Vault service
	data = strings.ReplaceAll(data, "vault.default", "vault."+cephCSINamespace)

	err = retryKubectlInput(cephCSINamespace, action, data, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to %s ServiceAccount: %w", action, err)
	}

	return nil
}
