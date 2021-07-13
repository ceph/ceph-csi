package e2e

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	// defaultVaultBackendPath is the default VAULT_BACKEND_PATH for secrets
	defaultVaultBackendPath = "secret/"
)

// kmsConfig is an interface that should be used when passing a configuration
// for a KMS to validation functions. This allows the validation functions to
// work independently from the actual KMS.
type kmsConfig interface {
	canGetPassphrase() bool
	getPassphrase(f *framework.Framework, key string) (string, string)
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
	}
	vaultTokensKMS = &vaultConfig{
		simpleKMS: &simpleKMS{
			provider: "vaulttokens",
		},
		backendPath: defaultVaultBackendPath,
	}
	vaultTenantSAKMS = &vaultConfig{
		simpleKMS: &simpleKMS{
			provider: "vaulttenantsa",
		},
		backendPath: "tenant/",
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
//  * authenticate with vault and ignore any stdout (we do not need output)
//  * issue get request for particular key
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
