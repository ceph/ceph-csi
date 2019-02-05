/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"

	"k8s.io/klog"
)

const (
	//PluginFolder defines location of plugins
	PluginFolder = "/var/lib/kubelet/plugins"
)

// ForAllFunc stores metadata with identifier
type ForAllFunc func(identifier string) error

// CachePersister interface implemented for store
type CachePersister interface {
	Create(identifier string, data interface{}) error
	Get(identifier string, data interface{}) error
	ForAll(pattern string, destObj interface{}, f ForAllFunc) error
	Delete(identifier string) error
}

// NewCachePersister returns CachePersister based on store
func NewCachePersister(metadataStore, driverName string) (CachePersister, error) {
	if metadataStore == "k8s_configmap" {
		klog.Infof("cache-perister: using kubernetes configmap as metadata cache persister")
		k8scm := &K8sCMCache{}
		k8scm.Client = NewK8sClient()
		k8scm.Namespace = GetK8sNamespace()
		return k8scm, nil
	} else if metadataStore == "node" {
		klog.Infof("cache-persister: using node as metadata cache persister")
		nc := &NodeCache{}
		nc.BasePath = PluginFolder + "/" + driverName
		return nc, nil
	}
	return nil, errors.New("cache-persister: couldn't parse metadatastorage flag")
}
