package util

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type K8sCMCache struct {
	Client    *k8s.Clientset
	Namespace string
}

const (
	defaultNamespace = "default"

	cmLabel   = "csi-metadata"
	cmDataKey = "content"

	csiMetadataLabelAttr = "com.ceph.ceph-csi/persister"
)

func GetK8sNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

func NewK8sClient() *k8s.Clientset {
	var cfg *rest.Config
	var err error
	cPath := os.Getenv("KUBERNETES_CONFIG_PATH")
	if cPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", cPath)
		if err != nil {
			glog.Errorf("Failed to get cluster config with error: %v\n", err)
			os.Exit(1)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			glog.Errorf("Failed to get cluster config with error: %v\n", err)
			os.Exit(1)
		}
	}
	client, err := k8s.NewForConfig(cfg)
	if err != nil {
		glog.Errorf("Failed to create client with error: %v\n", err)
		os.Exit(1)
	}
	return client
}

func (k8scm *K8sCMCache) createMetadataCM(resourceID string) (*v1.ConfigMap, error) {
	cm := &v1.ConfigMap{}
	cm, err := k8scm.getMetadataCM(resourceID)
	if err != nil {
		glog.V(4).Infof("k8s-cm-cache: couldn't get configmap %s with error: %v", resourceID, err)
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceID,
				Namespace: k8scm.Namespace,
				Labels: map[string]string{
					csiMetadataLabelAttr: cmLabel,
				},
			},
			Data: map[string]string{},
		}
		cm, err = k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Create(cm)
		if err != nil {
			return nil, errors.Wrapf(err, "k8s-cm-cache: couldn't create configmap %s\n", resourceID)
		}
	}
	glog.V(4).Infof("k8s-cm-cache: configmap %s successfully created\n", resourceID)
	return cm, nil
}

func (k8scm *K8sCMCache) getMetadataCM(resourceID string) (*v1.ConfigMap, error) {
	cm, err := k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Get(resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func (k8scm *K8sCMCache) ForAll(pattern string, destObj interface{}, f ForAllFunc) error {
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", csiMetadataLabelAttr, cmLabel)}
	cms, err := k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).List(listOpts)
	if err != nil {
		return errors.Wrap(err, "k8s-cm-cache: failed to list metadata configmaps")
	}

	for _, cm := range cms.Items {
		data := cm.Data[cmDataKey]
		match, err := regexp.MatchString(pattern, cm.ObjectMeta.Name)
		if err != nil {
			continue
		}
		if !match {
			continue
		}
		if err := json.Unmarshal([]byte(data), destObj); err != nil {
			return errors.Wrap(err, "k8s-cm-cache: unmarshal error")
		}
		if err = f(cm.ObjectMeta.Name); err != nil {
			return err
		}
	}
	return nil
}

func (k8scm *K8sCMCache) Create(identifier string, data interface{}) error {
	cm, err := k8scm.getMetadataCM(identifier)
	if err != nil {
		cm, err = k8scm.createMetadataCM(identifier)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				return errors.Wrap(err, "k8s-cm-cache: skipping configmap creation")
			}
			return err
		}
	}

	dataJson, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "k8s-cm-cache: marshal error")
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[cmDataKey] = string(dataJson)

	updatedCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      identifier,
			Namespace: k8scm.Namespace,
			Labels: map[string]string{
				csiMetadataLabelAttr: cmLabel,
			},
		},
		Data: cm.Data,
	}
	_, err = k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Update(updatedCM)
	if err != nil {
		return errors.Wrapf(err, "k8s-cm-cache: couldn't persist %s metadata as configmap", identifier)
	}
	return nil
}

func (k8scm *K8sCMCache) Get(identifier string, data interface{}) error {
	cm, err := k8scm.getMetadataCM(identifier)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(cm.Data[cmDataKey]), data)
	if err != nil {
		return errors.Wrap(err, "k8s-cm-cache: unmarshal error")
	}
	return nil
}

func (k8scm *K8sCMCache) Delete(identifier string) error {
	err := k8scm.Client.CoreV1().ConfigMaps(k8scm.Namespace).Delete(identifier, nil)
	if err != nil {
		return errors.Wrapf(err, "k8s-cm-cache: couldn't delete metadata configmap %s", identifier)
	}
	glog.Infof("k8s-cm-cache: successfully deleted metadata configmap %s", identifier)
	return nil
}
