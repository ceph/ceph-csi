package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/command/agent/auth"
	"github.com/hashicorp/vault/command/agent/auth/kubernetes"
)

const (
	vaultAddressPrefix  = "http"

	// AuthMethodKubernetes is a named auth method.
	AuthMethodKubernetes = "kubernetes"
	// AuthMethod is a vault authentication method used.
	// https://www.vaultproject.io/docs/auth#auth-methods
	AuthMethod = "VAULT_AUTH_METHOD"
	// AuthMountPath defines a custom auth mount path.
	AuthMountPath = "VAULT_AUTH_MOUNT_PATH"
	// AuthKubernetesRole is the role to authenticate against on Vault
	AuthKubernetesRole = "VAULT_AUTH_KUBERNETES_ROLE"
	// AuthKubernetesTokenPath is the file path to a custom JWT token to use for authentication.
	// If omitted, the default service account token path is used.
	AuthKubernetesTokenPath = "VAULT_AUTH_KUBERNETES_TOKEN_PATH"
	// AuthKubernetesMountPath
	AuthKubernetesMountPath = "kubernetes"
)

var (
	ErrVaultAuthParamsNotSet = errors.New("VAULT_TOKEN or VAULT_AUTH_METHOD not set")
	ErrVaultAddressNotSet    = errors.New("VAULT_ADDR not set")
	ErrInvalidVaultToken     = errors.New("VAULT_TOKEN is invalid")
	ErrInvalidSkipVerify     = errors.New("VAULT_SKIP_VERIFY is invalid")
	ErrInvalidVaultAddress   = errors.New("VAULT_ADDRESS is invalid. " +
		"Should be of the form http(s)://<ip>:<port>")

	ErrAuthMethodUnknown = errors.New("unknown auth method")
	ErrKubernetesRole    = errors.New(AuthKubernetesRole + " not set")
)

// IsValidAddr checks address has the correct format.
func IsValidAddr(address string) error {
	// Vault fails if address is not in correct format
	if !strings.HasPrefix(address, vaultAddressPrefix) {
		return ErrInvalidVaultAddress
	}
	return nil
}

// GetVaultParam retrieves a named parameter from the config or tried to get it from the environment variables.
func GetVaultParam(secretConfig map[string]interface{}, name string) string {
	if tokenIntf, exists := secretConfig[name]; exists {
		tokenStr, ok := tokenIntf.(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(tokenStr)
	} else {
		return strings.TrimSpace(os.Getenv(name))
	}
}

// ConfigureTLS adds tls parameters to the vault configuration.
func ConfigureTLS(config *api.Config, secretConfig map[string]interface{}) error {
	tlsConfig := api.TLSConfig{}
	skipVerify := GetVaultParam(secretConfig, api.EnvVaultInsecure)
	if skipVerify != "" {
		insecure, err := strconv.ParseBool(skipVerify)
		if err != nil {
			return ErrInvalidSkipVerify
		}
		tlsConfig.Insecure = insecure
	}

	cacert := GetVaultParam(secretConfig, api.EnvVaultCACert)
	tlsConfig.CACert = cacert

	capath := GetVaultParam(secretConfig, api.EnvVaultCAPath)
	tlsConfig.CAPath = capath

	clientcert := GetVaultParam(secretConfig, api.EnvVaultClientCert)
	tlsConfig.ClientCert = clientcert

	clientkey := GetVaultParam(secretConfig, api.EnvVaultClientKey)
	tlsConfig.ClientKey = clientkey

	tlsserverName := GetVaultParam(secretConfig, api.EnvVaultTLSServerName)
	tlsConfig.TLSServerName = tlsserverName

	return config.ConfigureTLS(&tlsConfig)
}

// CloseIdleConnections ensures that the vault idle connections are closed.
func CloseIdleConnections(cfg *api.Config) {
	if cfg == nil || cfg.HttpClient == nil {
		return
	}
	// close connection in case of error (a fix for go version < 1.12)
	if tp, ok := cfg.HttpClient.Transport.(*http.Transport); ok {
		tp.CloseIdleConnections()
	}
}

// Authenticate gets vault authentication parameters for the provided configuration.
func Authenticate(client *api.Client, config map[string]interface{}) (token string, autoAuth bool, err error) {
	// use VAULT_TOKEN if it's provided
	if token = GetVaultParam(config, api.EnvVaultToken); token != "" {
		return token, false, nil
	}

	// or try to use the kubernetes auth method
	if GetVaultParam(config, AuthMethod) != "" {
		token, err = GetAuthToken(client, config)
		return token, true, err
	}

	return "", false, ErrVaultAuthParamsNotSet
} 

// GetAuthToken tries to get the vault token for the provided authentication method.
func GetAuthToken(client *api.Client, config map[string]interface{}) (string, error) {
	path, _, data, err := authenticate(client, config)
	if err != nil {
		return "", err
	}

	secret, err := client.Logical().Write(path, data)
	if err != nil {
		return "", err
	}
	if secret == nil || secret.Auth == nil {
		return "", errors.New("authentication returned nil auth info")
	}
	if secret.Auth.ClientToken == "" {
		return "", errors.New("authentication returned empty client token")
	}

	return secret.Auth.ClientToken, err
}

func authenticate(client *api.Client, config map[string]interface{}) (string, http.Header, map[string]interface{}, error) {
	method := GetVaultParam(config, AuthMethod)
	switch method {
	case AuthMethodKubernetes:
		return authKubernetes(client, config)
	}
	return "", nil, nil, fmt.Errorf("%s method: %s", method, ErrAuthMethodUnknown)
}

func authKubernetes(client *api.Client, config map[string]interface{}) (string, http.Header, map[string]interface{}, error) {
	authConfig, err := buildAuthConfig(config)
	if err != nil {
		return "", nil, nil, err
	}
	method, err := kubernetes.NewKubernetesAuthMethod(authConfig)
	if err != nil {
		return "", nil, nil, err
	}

	return method.Authenticate(context.TODO(), client)
}

func buildAuthConfig(config map[string]interface{}) (*auth.AuthConfig, error) {
	role := GetVaultParam(config, AuthKubernetesRole)
	if role == "" {
		return nil, ErrKubernetesRole
	}
	mountPath := GetVaultParam(config, AuthMountPath)
	if mountPath == "" {
		mountPath = AuthKubernetesMountPath
	}
	tokenPath := GetVaultParam(config, AuthKubernetesTokenPath)

	authMountPath := path.Join("auth", mountPath)
	return &auth.AuthConfig{
		Logger:    hclog.NewNullLogger(),
		MountPath: authMountPath,
		Config: map[string]interface{}{
			"role":       role,
			"token_path": tokenPath,
		},
	}, nil
}
