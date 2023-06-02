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

package kms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// kmsProviderKey is the name of the KMS provider that is registered at
	// the kmsManager. This is used in the ConfigMap configuration options.
	kmsProviderKey = "KMS_PROVIDER"
	// kmsTypeKey is the name of the KMS provider that is registered at
	// the kmsManager. This is used in the configfile configuration
	// options.
	kmsTypeKey = "encryptionKMSType"

	// podNamespaceEnv ENV should be set in the cephcsi container.
	podNamespaceEnv = "POD_NAMESPACE"

	// kmsConfigMapEnv env to read a ConfigMap by name.
	kmsConfigMapEnv = "KMS_CONFIGMAP_NAME"

	// defaultKMSConfigMapName default ConfigMap name to fetch kms
	// connection details.
	defaultKMSConfigMapName = "csi-kms-connection-details"

	// kmsConfigPath is the location of the vault config file.
	kmsConfigPath = "/etc/ceph-csi-encryption-kms-config/config.json"

	// Default KMS type.
	DefaultKMSType = "default"
)

var (
	ErrGetSecretUnsupported = errors.New("KMS does not support access to user provided secret")
	ErrGetSecretIntegrated  = errors.New("integrated DEK stores do not allow GetSecret")
)

// GetKMS returns an instance of Key Management System.
//
//   - tenant is the owner of the Volume, used to fetch the Vault Token from the
//     Kubernetes Namespace where the PVC lives
//   - kmsID is the service name of the KMS configuration
//   - secrets contain additional details, like TLS certificates to connect to
//     the KMS
func GetKMS(tenant, kmsID string, secrets map[string]string) (EncryptionKMS, error) {
	if kmsID == "" || kmsID == DefaultKMSType {
		return GetDefaultKMS(secrets)
	}

	config, err := getKMSConfiguration()
	if err != nil {
		return nil, err
	}

	// config contains a list of KMS connections, indexed by kmsID
	section, ok := config[kmsID]
	if !ok {
		return nil, fmt.Errorf("could not get KMS configuration "+
			"for %q (have %v)", kmsID, getKeys(config))
	}

	// kmsConfig can have additional sub-sections
	kmsConfig, ok := section.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert KMS configuration "+
			"section: %s", kmsID)
	}

	return kmsManager.buildKMS(tenant, kmsConfig, secrets)
}

// getKMSConfiguration reads the configuration file from the filesystem, or if
// that fails the ConfigMap directly. The returned map contains all the KMS
// configuration sections, each keyed by its own kmsID.
func getKMSConfiguration() (map[string]interface{}, error) {
	var config map[string]interface{}
	// #nosec
	content, err := os.ReadFile(kmsConfigPath)
	if err == nil {
		// kmsConfigPath exists and was successfully read
		err = json.Unmarshal(content, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse KMS "+
				"configuration: %w", err)
		}
	} else {
		// an error occurred while reading kmsConfigPath
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read KMS "+
				"configuration from %s: %w", kmsConfigPath,
				err)
		}

		// If the configmap is not mounted to the CSI pods read the
		// configmap the kubernetes.
		config, err = getKMSConfigMap()
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

// getPodNamespace reads the `podNamespaceEnv` from the environment and returns
// its value. In case the namespace can not be detected, an error is returned.
func getPodNamespace() (string, error) {
	ns := os.Getenv(podNamespaceEnv)
	if ns == "" {
		return "", fmt.Errorf("%q is not set in the environment",
			podNamespaceEnv)
	}

	return ns, nil
}

// getKMSConfigMapName reads the `kmsConfigMapEnv` from the environment, or
// returns the value of `defaultKMSConfigMapName` if it was not set.
func getKMSConfigMapName() string {
	cmName := os.Getenv(kmsConfigMapEnv)
	if cmName == "" {
		cmName = defaultKMSConfigMapName
	}

	return cmName
}

// getKMSConfigMap returns the contents of the ConfigMap.
//
// FIXME: Ceph-CSI should not talk to Kubernetes directly.
func getKMSConfigMap() (map[string]interface{}, error) {
	ns, err := getPodNamespace()
	if err != nil {
		return nil, err
	}
	cmName := getKMSConfigMapName()

	c, err := k8s.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("can not get ConfigMap %q, failed to "+
			"connect to Kubernetes: %w", cmName, err)
	}

	cm, err := c.CoreV1().ConfigMaps(ns).Get(context.Background(),
		cmName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// convert cm.Data from map[string]interface{}
	kmsConfig := make(map[string]interface{})
	for kmsID, data := range cm.Data {
		section := make(map[string]interface{})
		err = json.Unmarshal([]byte(data), &section)
		if err != nil {
			return nil, fmt.Errorf("could not convert contents "+
				"of %q to s config section", kmsID)
		}
		kmsConfig[kmsID] = section
	}

	return kmsConfig, nil
}

// getProvider inspects the configuration and tries to identify what
// Provider is expected to be used with it. This returns the
// Provider.UniqueID.
func getProvider(config map[string]interface{}) (string, error) {
	var name string

	providerName, ok := config[kmsTypeKey]
	if ok {
		name, ok = providerName.(string)
		if !ok {
			return "", fmt.Errorf("could not convert KMS provider"+
				"type (%v) to string", providerName)
		}

		return name, nil
	}

	providerName, ok = config[kmsProviderKey]
	if ok {
		name, ok = providerName.(string)
		if !ok {
			return "", fmt.Errorf("could not convert KMS provider"+
				"type (%v) to string", providerName)
		}

		return name, nil
	}

	return "", fmt.Errorf("failed to get KMS provider, missing"+
		"configuration option %q or %q", kmsTypeKey, kmsProviderKey)
}

// ProviderInitArgs get passed to ProviderInitFunc when a new instance of a
// Provider is initialized.
type ProviderInitArgs struct {
	Tenant  string
	Config  map[string]interface{}
	Secrets map[string]string
	// Namespace contains the Kubernetes Namespace where the Ceph-CSI Pods
	// are running. This is an optional option, and might be unset when the
	// Provider.Initializer is called.
	Namespace string
}

// ProviderInitFunc gets called when the Provider needs to be
// instantiated.
type ProviderInitFunc func(args ProviderInitArgs) (EncryptionKMS, error)

type Provider struct {
	UniqueID    string
	Initializer ProviderInitFunc
}

type kmsProviderList struct {
	providers map[string]Provider
}

// kmsManager is used to create instances for a KMS provider.
var kmsManager = kmsProviderList{providers: map[string]Provider{}}

// RegisterProvider uses kmsManager to register the given Provider. The
// Provider.Initializer function will get called when a new instance of the
// KMS is required.
func RegisterProvider(provider Provider) bool {
	// validate uniqueness of the UniqueID
	if provider.UniqueID == "" {
		panic("a provider MUST set a UniqueID")
	}
	_, ok := kmsManager.providers[provider.UniqueID]
	if ok {
		panic("duplicate registration of Provider.UniqueID: " + provider.UniqueID)
	}

	// validate the Initializer
	if provider.Initializer == nil {
		panic("a provider MUST have an Initializer")
	}

	kmsManager.providers[provider.UniqueID] = provider

	return true
}

// buildKMS creates a new Provider instance, based on the configuration that
// was passed. This uses getProvider() internally to identify the
// Provider to instantiate.
func (kf *kmsProviderList) buildKMS(
	tenant string,
	config map[string]interface{},
	secrets map[string]string,
) (EncryptionKMS, error) {
	providerName, err := getProvider(config)
	if err != nil {
		return nil, err
	}

	provider, ok := kf.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("could not find KMS provider %q",
			providerName)
	}

	kmsInitArgs := ProviderInitArgs{
		Tenant:  tenant,
		Config:  config,
		Secrets: secrets,
	}

	// Namespace is an optional parameter, it may not be set and is not
	// required for all Providers
	ns, err := getPodNamespace()
	if err == nil {
		kmsInitArgs.Namespace = ns
	}

	return provider.Initializer(kmsInitArgs)
}

func GetDefaultKMS(secrets map[string]string) (EncryptionKMS, error) {
	provider, ok := kmsManager.providers[DefaultKMSType]
	if !ok {
		return nil, fmt.Errorf("could not find KMS provider %q", DefaultKMSType)
	}

	kmsInitArgs := ProviderInitArgs{
		Secrets: secrets,
	}

	return provider.Initializer(kmsInitArgs)
}

// EncryptionKMS provides external Key Management System for encryption
// passphrases storage.
type EncryptionKMS interface {
	Destroy()

	// RequiresDEKStore returns the DEKStoreType that is needed to be
	// configure for the KMS. Nothing needs to be done when this function
	// returns DEKStoreIntegrated, otherwise you will need to configure an
	// alternative storage for the DEKs.
	RequiresDEKStore() DEKStoreType

	// EncryptDEK provides a way for a KMS to encrypt a DEK. In case the
	// encryption is done transparently inside the KMS service, the
	// function can return an unencrypted value.
	EncryptDEK(volumeID, plainDEK string) (string, error)

	// DecryptDEK provides a way for a KMS to decrypt a DEK. In case the
	// encryption is done transparently inside the KMS service, the
	// function does not need to do anything except return the encyptedDEK
	// as it was received.
	DecryptDEK(volumeID, encyptedDEK string) (string, error)

	// GetSecret allows external key management systems to
	// retrieve keys used in EncryptDEK / DecryptDEK to use them
	// directly. Example: fscrypt uses this to unlock raw protectors
	GetSecret(volumeID string) (string, error)
}

// DEKStoreType describes what DEKStore needs to be configured when using a
// particular KMS. A KMS might support different DEKStores depending on its
// configuration.
type DEKStoreType string

const (
	// DEKStoreIntegrated indicates that the KMS itself supports storing
	// DEKs.
	DEKStoreIntegrated = DEKStoreType("")
	// DEKStoreMetadata indicates that the KMS should be configured to
	// store the DEK in the metadata of the volume.
	DEKStoreMetadata = DEKStoreType("metadata")
)

// DEKStore allows KMS instances to implement a modular backend for DEK
// storage. This can be used to store the DEK in a different location, in case
// the KMS can not store passphrases for volumes.
type DEKStore interface {
	// StoreDEK saves the DEK in the configured store.
	StoreDEK(volumeID string, dek string) error
	// FetchDEK reads the DEK from the configured store and returns it.
	FetchDEK(volumeID string) (string, error)
	// RemoveDEK deletes the DEK from the configured store.
	RemoveDEK(volumeID string) error
}

// integratedDEK is a DEKStore that can not be configured. Either the KMS does
// not use a DEK, or the DEK is stored in the KMS without additional
// configuration options.
type integratedDEK struct{}

func (i integratedDEK) RequiresDEKStore() DEKStoreType {
	return DEKStoreIntegrated
}

func (i integratedDEK) EncryptDEK(volumeID, plainDEK string) (string, error) {
	return plainDEK, nil
}

func (i integratedDEK) DecryptDEK(volumeID, encyptedDEK string) (string, error) {
	return encyptedDEK, nil
}

func (i integratedDEK) GetSecret(volumeID string) (string, error) {
	return "", ErrGetSecretIntegrated
}

// getKeys takes a map that uses strings for keys and returns a slice with the
// keys.
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, len(m))

	i := 0
	for k := range m {
		keys[i] = k
		i++
	}

	return keys
}
