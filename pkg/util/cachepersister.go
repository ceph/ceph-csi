package util

import (
	"errors"

	"github.com/golang/glog"
)

const (
	PluginFolder = "/var/lib/kubelet/plugins"
)

type ForAllFunc func(identifier string) error

type CachePersister interface {
	Create(identifier string, data interface{}) error
	Get(identifier string, data interface{}) error
	ForAll(pattern string, destObj interface{}, f ForAllFunc) error
	Delete(identifier string) error
}

func NewCachePersister(metadataStore string, driverName string) (CachePersister, error) {
	if metadataStore == "k8s_configmap" {
		glog.Infof("cache-perister: using kubernetes configmap as metadata cache persister")
		k8scm := &K8sCMCache{}
		k8scm.Client = NewK8sClient()
		k8scm.Namespace = GetK8sNamespace()
		return k8scm, nil
	} else if metadataStore == "node" {
		glog.Infof("cache-persister: using node as metadata cache persister")
		nc := &NodeCache{}
		nc.BasePath = PluginFolder + "/" + driverName
		return nc, nil
	}
	return nil, errors.New("cache-persister: couldn't parse metadatastorage flag")
}
