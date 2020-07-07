/*
Copyright 2018 The Ceph-CSI Authors.

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

// ForAllFunc is a unary predicate for visiting all cache entries
// matching the `pattern' in CachePersister's ForAll function.
type ForAllFunc func(identifier string) error

// CacheEntryNotFound is an error type for "Not Found" cache errors.
type CacheEntryNotFound struct {
	error
}

// CachePersister interface implemented for store.
type CachePersister interface {
	Create(identifier string, data interface{}) error
	Get(identifier string, data interface{}) error
	ForAll(pattern string, destObj interface{}, f ForAllFunc) error
	Delete(identifier string) error
}

// NewCachePersister returns CachePersister based on store.
func NewCachePersister(metadataStore, pluginPath string) (CachePersister, error) {
	if metadataStore == "k8s_configmap" {
		klog.V(4).Infof("cache-perister: using kubernetes configmap as metadata cache persister") // nolint:gomnd // number specifies log level
		k8scm := &K8sCMCache{}
		k8scm.Client = NewK8sClient()
		k8scm.Namespace = GetK8sNamespace()
		return k8scm, nil
	} else if metadataStore == "node" {
		klog.V(4).Infof("cache-persister: using node as metadata cache persister") // nolint:gomnd // number specifies log level
		nc := &NodeCache{}
		nc.BasePath = pluginPath
		nc.CacheDir = "controller"
		return nc, nil
	}
	return nil, errors.New("cache-persister: couldn't parse metadatastorage flag")
}
