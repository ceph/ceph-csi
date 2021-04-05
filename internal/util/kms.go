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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

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

	// podNamespaceEnv ENV should be set in the cephcsi container
	podNamespaceEnv = "POD_NAMESPACE"

	// kmsConfigMapEnv env to read a ConfigMap by name
	kmsConfigMapEnv = "KMS_CONFIGMAP_NAME"

	// defaultKMSConfigMapName default ConfigMap name to fetch kms
	// connection details
	defaultKMSConfigMapName = "csi-kms-connection-details"
)

// GetKMS returns an instance of Key Management System.
//
// - tenant is the owner of the Volume, used to fetch the Vault Token from the
//   Kubernetes Namespace where the PVC lives
// - kmsID is the service name of the KMS configuration
// - secrets contain additional details, like TLS certificates to connect to
//   the KMS
func GetKMS(tenant, kmsID string, secrets map[string]string) (EncryptionKMS, error) {
	if kmsID == "" || kmsID == defaultKMSType {
		return initSecretsKMS(secrets)
	}

	config, err := getKMSConfiguration()
	if err != nil {
		return nil, err
	}

	// config contains a list of KMS connections, indexed by kmsID
	section, ok := config[kmsID]
	if !ok {
		return nil, fmt.Errorf("could not get KMS configuration "+
			"for %q", kmsID)
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
	content, err := ioutil.ReadFile(kmsConfigPath)
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

	c := NewK8sClient()
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

// getKMSProvider inspects the configuration and tries to identify what
// KMSProvider is expected to be used with it. This returns the
// KMSProvider.UniqueID.
func getKMSProvider(config map[string]interface{}) (string, error) {
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

// KMSInitializerArgs get passed to KMSInitializerFunc when a new instance of a
// KMSProvider is initialized.
type KMSInitializerArgs struct {
	Tenant  string
	Config  map[string]interface{}
	Secrets map[string]string
	// Namespace contains the Kubernetes Namespace where the Ceph-CSI Pods
	// are running. This is an optional option, and might be unset when the
	// KMSProvider.Initializer is called.
	Namespace string
}

// KMSInitializerFunc gets called when the KMSProvider needs to be
// instantiated.
type KMSInitializerFunc func(args KMSInitializerArgs) (EncryptionKMS, error)

type KMSProvider struct {
	UniqueID    string
	Initializer KMSInitializerFunc
}

type kmsProviderList struct {
	providers map[string]KMSProvider
}

// kmsManager is used to create instances for a KMS provider.
var kmsManager = kmsProviderList{providers: map[string]KMSProvider{}}

// RegisterKMSProvider uses kmsManager to register the given KMSProvider. The
// KMSProvider.Initializer function will get called when a new instance of the
// KMS is required.
func RegisterKMSProvider(provider KMSProvider) bool {
	// validate uniqueness of the UniqueID
	if provider.UniqueID == "" {
		panic("a provider MUST set a UniqueID")
	}
	_, ok := kmsManager.providers[provider.UniqueID]
	if ok {
		panic("duplicate tegistration of KMSProvider.UniqueID: " + provider.UniqueID)
	}

	// validate the Initializer
	if provider.Initializer == nil {
		panic("a provider MUST have an Initializer")
	}

	kmsManager.providers[provider.UniqueID] = provider

	return true
}

// buildKMS creates a new KMSProvider instance, based on the configuration that
// was passed. This uses getKMSProvider() internally to identify the
// KMSProvider to instantiate.
func (kf *kmsProviderList) buildKMS(tenant string, config map[string]interface{}, secrets map[string]string) (EncryptionKMS, error) {
	providerName, err := getKMSProvider(config)
	if err != nil {
		return nil, err
	}

	provider, ok := kf.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("could not find KMS provider %q",
			providerName)
	}

	kmsInitArgs := KMSInitializerArgs{
		Tenant:  tenant,
		Config:  config,
		Secrets: secrets,
	}

	// Namespace is an optional parameter, it may not be set and is not
	// required for all KMSProviders
	ns, err := getPodNamespace()
	if err == nil {
		kmsInitArgs.Namespace = ns
	}

	return provider.Initializer(kmsInitArgs)
}
