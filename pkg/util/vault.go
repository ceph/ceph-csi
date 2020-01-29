/*
Copyright 2019 The Ceph-CSI Authors.

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

package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	// path to service account token that will be used to authenticate with Vault
	// #nosec
	serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	// vault configuration defaults
	vaultDefaultAuthPath       = "/v1/auth/kubernetes/login"
	vaultDefaultRole           = "csi-kubernetes"
	vaultDefaultNamespace      = ""
	vaultDefaultPassphraseRoot = "/v1/secret"
	vaultDefaultPassphrasePath = ""

	// vault request headers
	vaultTokenHeader     = "X-Vault-Token" // nolint: gosec, #nosec
	vaultNamespaceHeader = "X-Vault-Namespace"
)

/*
kmsKMS represents a Hashicorp Vault KMS configuration

Example JSON structure in the KMS config is,
[
	{
		"encryptionKMSID": "local_vault_unique_identifier",
		"vaultAddress": "https://127.0.0.1:8500",
		"vaultAuthPath": "/v1/auth/kubernetes/login",
		"vaultRole": "csi-kubernetes",
		"vaultNamespace": "",
		"vaultPassphraseRoot": "/v1/secret",
		"vaultPassphrasePath": "",
		"vaultCAVerify": true,
		"vaultCAFromSecret": "vault-ca"
	},
	...
]
*/
type VaultKMS struct {
	EncryptionKMSID     string `json:"encryptionKMSID"`
	VaultAddress        string `json:"vaultAddress"`
	VaultAuthPath       string `json:"vaultAuthPath"`
	VaultRole           string `json:"vaultRole"`
	VaultNamespace      string `json:"vaultNamespace"`
	VaultPassphraseRoot string `json:"vaultPassphraseRoot"`
	VaultPassphrasePath string `json:"vaultPassphrasePath"`
	VaultCAVerify       bool   `json:"vaultCAVerify"`
	VaultCAFromSecret   string `json:"vaultCAFromSecret"`
	vaultCA             *x509.CertPool
}

// InitVaultKMS returns an interface to HashiCorp Vault KMS
func InitVaultKMS(opts, secrets map[string]string) (EncryptionKMS, error) {
	var config []VaultKMS

	vaultID, ok := opts["encryptionKMSID"]
	if !ok {
		return nil, fmt.Errorf("missing encryptionKMSID for vault as encryption KMS")
	}

	// #nosec
	content, err := ioutil.ReadFile(kmsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error fetching vault configuration for vault ID (%s): (%s)",
			vaultID, err)
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %v. raw buffer response: %s",
			err, string(content))
	}

	for i := range config {
		vault := &config[i]
		if vault.EncryptionKMSID != vaultID {
			continue
		}
		if vault.VaultAddress == "" {
			return nil, fmt.Errorf("missing vaultAddress for vault as encryption KMS")
		}
		if vault.VaultAuthPath == "" {
			vault.VaultAuthPath = vaultDefaultAuthPath
		}
		if vault.VaultRole == "" {
			vault.VaultRole = vaultDefaultRole
		}
		if vault.VaultNamespace == "" {
			vault.VaultNamespace = vaultDefaultNamespace
		}
		if vault.VaultPassphraseRoot == "" {
			vault.VaultPassphraseRoot = vaultDefaultPassphraseRoot
		}
		if vault.VaultPassphrasePath == "" {
			vault.VaultPassphrasePath = vaultDefaultPassphrasePath
		}
		if vault.VaultCAFromSecret != "" {
			caPEM, ok := secrets[vault.VaultCAFromSecret]
			if !ok {
				return nil, fmt.Errorf("missing vault CA in secret %s", vault.VaultCAFromSecret)
			}
			roots := x509.NewCertPool()
			ok = roots.AppendCertsFromPEM([]byte(caPEM))
			if !ok {
				return nil, fmt.Errorf("failed loading CA bundle for vault from secret %s",
					vault.VaultCAFromSecret)
			}
			vault.vaultCA = roots
		}
		return vault, nil
	}

	return nil, fmt.Errorf("missing configuration for vault ID (%s)", vaultID)
}

// KmsConfig returns KMS configuration: "<kms-type>|<kms-id>"
func (kms *VaultKMS) KmsConfig() string {
	return fmt.Sprintf("vault|%s", kms.EncryptionKMSID)
}

// GetPassphrase returns passphrase from Vault
func (kms *VaultKMS) GetPassphrase(key string) (string, error) {
	var passphrase string
	resp, err := kms.request("GET", kms.getKeyDataURI(key), nil)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve passphrase for %s from vault: %s",
			key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", MissingPassphrase{fmt.Errorf("passphrase for %s not found", key)}
	}
	err = kms.processError(resp, fmt.Sprintf("get passphrase for %s", key))
	if err != nil {
		return "", err
	}

	// parse resp as JSON and retrieve vault token
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed parsing passphrase for %s from response: %s",
			key, err)
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed parsing data for get passphrase request for %s", key)
	}
	data, ok = data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed parsing data.data for get passphrase request for %s", key)
	}
	passphrase, ok = data["passphrase"].(string)
	if !ok {
		return "", fmt.Errorf("failed parsing passphrase for get passphrase request for %s", key)
	}

	return passphrase, nil
}

// SavePassphrase saves new passphrase in Vault
func (kms *VaultKMS) SavePassphrase(key, value string) error {
	data, err := json.Marshal(map[string]map[string]string{
		"data": {
			"passphrase": value,
		},
	})
	if err != nil {
		return fmt.Errorf("passphrase request data is broken: %s", err)
	}

	resp, err := kms.request("POST", kms.getKeyDataURI(key), data)
	if err != nil {
		return fmt.Errorf("failed to POST passphrase for %s to vault: %s", key, err)
	}
	defer resp.Body.Close()
	err = kms.processError(resp, "save passphrase")
	if err != nil {
		return err
	}

	return nil
}

// DeletePassphrase deletes passphrase from Vault
func (kms *VaultKMS) DeletePassphrase(key string) error {
	vaultToken, err := kms.getAccessToken()
	if err != nil {
		return fmt.Errorf("could not retrieve vault token to delete the passphrase at %s: %s",
			key, err)
	}

	resp, err := kms.send("DELETE", kms.getKeyMetadataURI(key), &vaultToken, nil)
	if err != nil {
		return fmt.Errorf("delete passphrase at %s request to vault failed: %s", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		err = kms.processError(resp, "delete passphrase")
		if err != nil {
			return err
		}
	}
	return nil
}

func (kms *VaultKMS) getKeyDataURI(key string) string {
	return kms.VaultPassphraseRoot + "/data/" + kms.VaultPassphrasePath + key
}

func (kms *VaultKMS) getKeyMetadataURI(key string) string {
	return kms.VaultPassphraseRoot + "/metadata/" + kms.VaultPassphrasePath + key
}

/*
getVaultAccessToken retrieves vault token using kubernetes authentication:
 1. read jwt service account token from well known location
 2. request token from vault using service account jwt token
Vault will verify service account jwt token with Kubernetes and return token
if the requester is allowed
*/
func (kms *VaultKMS) getAccessToken() (string, error) {
	saToken, err := ioutil.ReadFile(serviceAccountTokenPath)
	if err != nil {
		return "", fmt.Errorf("service account token could not be read: %s", err)
	}
	data, err := json.Marshal(map[string]string{
		"role": kms.VaultRole,
		"jwt":  string(saToken),
	})
	if err != nil {
		return "", fmt.Errorf("vault token request data is broken: %s", err)
	}
	resp, err := kms.send("POST", kms.VaultAuthPath, nil, data)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve vault token: %s", err)
	}
	defer resp.Body.Close()

	err = kms.processError(resp, "retrieve vault token")
	if err != nil {
		return "", err
	}
	// parse resp as JSON and retrieve vault token
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed parsing vaultToken from response: %s", err)
	}

	auth, ok := result["auth"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed parsing vault token auth data")
	}
	vaultToken, ok := auth["client_token"].(string)
	if !ok {
		return "", fmt.Errorf("failed parsing vault client_token")
	}

	return vaultToken, nil
}

func (kms *VaultKMS) processError(resp *http.Response, action string) error {
	if resp.StatusCode >= 200 || resp.StatusCode < 300 {
		return nil
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to %s (%v), error body parsing failed: %s",
			action, resp.StatusCode, err)
	}
	return fmt.Errorf("failed to %s (%v): %s", action, resp.StatusCode, body)
}

func (kms *VaultKMS) request(method, path string, data []byte) (*http.Response, error) {
	vaultToken, err := kms.getAccessToken()
	if err != nil {
		return nil, err
	}

	return kms.send(method, path, &vaultToken, data)
}

func (kms *VaultKMS) send(method, path string, token *string, data []byte) (*http.Response, error) {
	tlsConfig := &tls.Config{}
	if !kms.VaultCAVerify {
		tlsConfig.InsecureSkipVerify = true
	}
	if kms.vaultCA != nil {
		tlsConfig.RootCAs = kms.vaultCA
	}
	netTransport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: netTransport}

	var dataToSend io.Reader
	if data != nil {
		dataToSend = strings.NewReader(string(data))
	}

	req, err := http.NewRequest(method, kms.VaultAddress+path, dataToSend)
	if err != nil {
		return nil, fmt.Errorf("could not create a Vault request: %s", err)
	}

	if kms.VaultNamespace != "" {
		req.Header.Set(vaultNamespaceHeader, kms.VaultNamespace)
	}
	if token != nil {
		req.Header.Set(vaultTokenHeader, *token)
	}

	return client.Do(req)
}
