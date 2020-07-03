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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// K8sCMCache to store metadata
type K8sCMCache struct {
	Client    *k8s.Clientset
	Namespace string
}

const (
	defaultNamespace = "default"

	cmLabel   = "csi-metadata"
	cmDataKey = "content"

	csiMetadataLabelAttr = "com.ceph.ceph-csi/metadata"
)

// GetK8sNamespace returns pod namespace. if pod namespace is empty
// it returns default namespace
func GetK8sNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

// NewK8sClient create kubernetes client
func NewK8sClient() *k8s.Clientset {
	var cfg *rest.Config
	var err error
	cPath := os.Getenv("KUBERNETES_CONFIG_PATH")
	if cPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", cPath)
		if err != nil {
			klog.Errorf("Failed to get cluster config with error: %v\n", err)
			os.Exit(1)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			klog.Errorf("Failed to get cluster config with error: %v\n", err)
			os.Exit(1)
		}
	}
	client, err := k8s.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("Failed to create client with error: %v\n", err)
		os.Exit(1)
	}
	return client
}

func (k8scm *K8sCMCache) getMetadataCM(resourceID string) (*v1.ConfigMap, error) {
	cm, err := k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Get(context.TODO(), resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// ForAll list the metadata in configmaps and filters outs based on the pattern
func (k8scm *K8sCMCache) ForAll(pattern string, destObj interface{}, f ForAllFunc) error {
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", csiMetadataLabelAttr, cmLabel)}
	cms, err := k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).List(context.TODO(), listOpts)
	if err != nil {
		return fmt.Errorf("k8s-cm-cache: failed to list metadata configmaps: %w", err)
	}

	for i := range cms.Items {
		data := cms.Items[i].Data[cmDataKey]
		match, err := regexp.MatchString(pattern, cms.Items[i].ObjectMeta.Name)
		if err != nil {
			continue
		}
		if !match {
			continue
		}
		if err = json.Unmarshal([]byte(data), destObj); err != nil {
			return fmt.Errorf("k8s-cm-cache: JSON unmarshaling failed for configmap %s: %w", cms.Items[i].ObjectMeta.Name, err)
		}
		if err = f(cms.Items[i].ObjectMeta.Name); err != nil {
			return err
		}
	}
	return nil
}

// Create stores the metadata in configmaps with identifier name
func (k8scm *K8sCMCache) Create(identifier string, data interface{}) error {
	cm, err := k8scm.getMetadataCM(identifier)
	if cm != nil && err == nil {
		klog.V(4).Infof("k8s-cm-cache: configmap %s already exists, skipping configmap creation", identifier)
		return nil
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("k8s-cm-cache: JSON marshaling failed for configmap %s: %w", identifier, err)
	}
	cm = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      identifier,
			Namespace: k8scm.Namespace,
			Labels: map[string]string{
				csiMetadataLabelAttr: cmLabel,
			},
		},
		Data: map[string]string{},
	}
	cm.Data[cmDataKey] = string(dataJSON)

	_, err = k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Create(context.TODO(), cm, metav1.CreateOptions{})
	if err != nil {
		if apierrs.IsAlreadyExists(err) {
			klog.V(4).Infof("k8s-cm-cache: configmap %s already exists", identifier)
			return nil
		}
		return fmt.Errorf("k8s-cm-cache: couldn't persist %s metadata as configmap: %w", identifier, err)
	}

	klog.V(4).Infof("k8s-cm-cache: configmap %s successfully created", identifier)
	return nil
}

// Get retrieves the metadata in configmaps with identifier name
func (k8scm *K8sCMCache) Get(identifier string, data interface{}) error {
	cm, err := k8scm.getMetadataCM(identifier)
	if err != nil {
		if apierrs.IsNotFound(err) {
			return &CacheEntryNotFound{err}
		}

		return err
	}
	err = json.Unmarshal([]byte(cm.Data[cmDataKey]), data)
	if err != nil {
		return fmt.Errorf("k8s-cm-cache: JSON unmarshaling failed for configmap %s: %w", identifier, err)
	}
	return nil
}

// Delete deletes the metadata in configmaps with identifier name
func (k8scm *K8sCMCache) Delete(identifier string) error {
	err := k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Delete(context.TODO(), identifier, metav1.DeleteOptions{})
	if err != nil {
		if apierrs.IsNotFound(err) {
			klog.V(4).Infof("k8s-cm-cache: cannot delete missing metadata configmap %s, assuming it's already deleted", identifier)
			return nil
		}

		return fmt.Errorf("k8s-cm-cache: couldn't delete metadata configmap %s: %w", identifier, err)
	}
	klog.V(4).Infof("k8s-cm-cache: successfully deleted metadata configmap %s", identifier)
	return nil
}
