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
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	// defaultVaultBackendPath is the default VAULT_BACKEND_PATH for secrets.
	defaultVaultBackendPath = "secret/"
)

// kmsConfig is an interface that should be used when passing a configuration
// for a KMS to validation functions. This allows the validation functions to
// work independently of the actual KMS.
type kmsConfig interface {
	canGetPassphrase() bool
	getPassphrase(f *framework.Framework, key string) (string, string)
	canVerifyKeyDestroyed() bool
	verifyKeyDestroyed(f *framework.Framework, key string) (bool, string)
}

// simpleKMS is to be used for KMS configurations that do not offer options to
// validate the passphrase stored in the KMS.
type simpleKMS struct {
	provider string
}

// vaultConfig describes the configuration of the Hashicorp Vault service that
// is used to store the encryption passphrase in.
type vaultConfig struct {
	*simpleKMS
	backendPath string
	// destroyKeys indicates that a Vault config needs to destroy the
	// metadata of deleted keys in addition to the data
	destroyKeys bool
}

// The following variables describe different KMS services as they are defined
// in the kms-config.yaml file. These variables can be passed on to validation
// functions when a StorageClass has a KMS enabled.
var (
	noKMS kmsConfig

	secretsMetadataKMS = &simpleKMS{
		provider: "secrets-metadata",
	}

	vaultKMS = &vaultConfig{
		simpleKMS: &simpleKMS{
			provider: "vault",
		},
		backendPath: defaultVaultBackendPath + "ceph-csi/",
		destroyKeys: true,
	}
	vaultTokensKMS = &vaultConfig{
		simpleKMS: &simpleKMS{
			provider: "vaulttokens",
		},
		backendPath: defaultVaultBackendPath,
		destroyKeys: true,
	}
	vaultTenantSAKMS = &vaultConfig{
		simpleKMS: &simpleKMS{
			provider: "vaulttenantsa",
		},
		backendPath: "tenant/",
		destroyKeys: false,
	}
)

func (sk *simpleKMS) String() string {
	return sk.provider
}

// canGetPassphrase returns false for the basic KMS configuration as there is
// currently no way to fetch the passphrase.
func (sk *simpleKMS) canGetPassphrase() bool {
	return false
}

func (sk *simpleKMS) getPassphrase(f *framework.Framework, key string) (string, string) {
	return "", ""
}

func (sk *simpleKMS) canVerifyKeyDestroyed() bool {
	return false
}

func (sk *simpleKMS) verifyKeyDestroyed(f *framework.Framework, key string) (bool, string) {
	return false, ""
}

func (vc *vaultConfig) String() string {
	return fmt.Sprintf("%s (backend path %q)", vc.simpleKMS, vc.backendPath)
}

// canGetPassphrase returns true for the Hashicorp Vault KMS configurations as
// the Vault CLI can be used to retrieve the passphrase.
func (vc *vaultConfig) canGetPassphrase() bool {
	return true
}

// getPassphrase method will execute few commands to try read the secret for
// specified key from inside the vault container:
//   - authenticate with vault and ignore any stdout (we do not need output)
//   - issue get request for particular key
//
// resulting in stdOut (first entry in tuple) - output that contains the key
// or stdErr (second entry in tuple) - error getting the key.
func (vc *vaultConfig) getPassphrase(f *framework.Framework, key string) (string, string) {
	vaultAddr := fmt.Sprintf("http://vault.%s.svc.cluster.local:8200", cephCSINamespace)
	loginCmd := fmt.Sprintf("vault login -address=%s sample_root_token_id > /dev/null", vaultAddr)
	readSecret := fmt.Sprintf("vault kv get -address=%s -field=data %s%s",
		vaultAddr, vc.backendPath, key)
	cmd := fmt.Sprintf("%s && %s", loginCmd, readSecret)
	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}
	stdOut, stdErr := execCommandInPodAndAllowFail(f, cmd, cephCSINamespace, &opt)

	return strings.TrimSpace(stdOut), strings.TrimSpace(stdErr)
}

// canVerifyKeyDestroyed returns true in case the Vault configuration for the
// KMS setup destroys the keys in addition to (soft) deleting the contents.
func (vc *vaultConfig) canVerifyKeyDestroyed() bool {
	return vc.destroyKeys
}

// verifyKeyDestroyed checks for the metadata of a deleted key. If the
// deletion_time from the metadata can be read, the key has not been destroyed
// but only (soft) deleted.
func (vc *vaultConfig) verifyKeyDestroyed(f *framework.Framework, key string) (bool, string) {
	vaultAddr := fmt.Sprintf("http://vault.%s.svc.cluster.local:8200", cephCSINamespace)
	loginCmd := fmt.Sprintf("vault login -address=%s sample_root_token_id > /dev/null", vaultAddr)
	readDeletionTime := fmt.Sprintf("vault kv metadata get -address=%s -field=deletion_time %s%s",
		vaultAddr, vc.backendPath, key)
	cmd := fmt.Sprintf("%s && %s", loginCmd, readDeletionTime)
	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}
	stdOut, stdErr := execCommandInPodAndAllowFail(f, cmd, cephCSINamespace, &opt)

	// in case stdOut contains something, it will be the deletion_time
	// when the deletion_time is set, the metadata is still available and not destroyed
	if strings.TrimSpace(stdOut) != "" {
		return false, stdOut
	}

	// when stdOut is empty, assume the key is completely destroyed
	return true, stdErr
}
