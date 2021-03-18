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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// kmsProviderKey is the name of the KMS provider that is registered at
	// the kmsManager. This is used in the ConfigMap configuration options.
	kmsProviderKey = "KMS_PROVIDER"
)

// getKMSConfig returns the (.Data) contents of the ConfigMap.
//
// FIXME: Ceph-CSI should not talk to Kubernetes directly.
func getKMSConfig(ns, configmap string) (map[string]string, error) {
	c := NewK8sClient()
	cm, err := c.CoreV1().ConfigMaps(ns).Get(context.Background(),
		configmap, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return cm.Data, nil
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
	Id, Tenant string
	Config     map[string]interface{}
	Secrets    map[string]string
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

func (kf *kmsProviderList) buildKMS(providerName, kmsID, tenant string, config map[string]interface{}, secrets map[string]string) (EncryptionKMS, error) {
	provider, ok := kf.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("could not find KMS provider %q",
			providerName)
	}

	return provider.Initializer(KMSInitializerArgs{
		Id:      kmsID,
		Tenant:  tenant,
		Config:  config,
		Secrets: secrets,
	})
}
