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

package rbd

import (
	"encoding/json"
	"os"
	"path"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type volMeta interface {
	persistVolInfo(image string, storagePath string, volInfo *rbdVolume) error
	loadVolInfo(image string, storagePath string, volInfo *rbdVolume) error
	deleteVolInfo(image string, storagePath string) error
}

type snapMeta interface {
	persistSnapInfo(snapshot string, storagePath string, snapInfo *rbdSnapshot) error
	loadSnapInfo(snapshot string, storagePath string, snapInfo *rbdSnapshot) error
	deleteSnapInfo(snapshot string, storagePath string) error
}

const (
	cmName = "csi-rbd-metadata"
	defaultNamespace = "default"
)

var (
	Client *k8s.Clientset
	namespace = getNamespace()
)

func newVolMeta(persistMetadata bool) (volMeta, error) {
	if persistMetadata {
		return &cmVolMeta{}, nil
	} else {
		return &hostVolMeta{}, nil
	}
	return nil, errors.New("rbd: failed to load metadata persist method")
}

func newSnapMeta(persistMetadata bool) (snapMeta, error) {
	if persistMetadata {
		return &cmSnapMeta{}, nil
	} else {
		return &hostSnapMeta{}, nil
	}
	return nil, errors.New("rbd: failed to load metadata persist method")
}

type hostVolMeta struct {}
type hostSnapMeta struct {}

func (v *hostVolMeta) persistVolInfo(image string, persistentStoragePath string, volInfo *rbdVolume) error {
	file := path.Join(persistentStoragePath, image+".json")
	fp, err := os.Create(file)
	if err != nil {
		glog.Errorf("rbd: failed to create persistent storage file %s with error: %v\n", file, err)
		return errors.Wrapf(err, "rbd: create error for %s", file)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(volInfo); err != nil {
		glog.Errorf("rbd: failed to encode volInfo: %+v for file: %s with error: %v\n", volInfo, file, err)
		return errors.Wrap(err, "rbd: encode error")
	}
	glog.Infof("rbd: successfully saved volInfo: %+v into file: %s\n", volInfo, file)
	return nil
}

func (v *hostVolMeta) loadVolInfo(image string, persistentStoragePath string, volInfo *rbdVolume) error {
	file := path.Join(persistentStoragePath, image+".json")
	fp, err := os.Open(file)
	if err != nil {
		return errors.Wrapf(err, "rbd: open error for %s", file)
	}
	defer fp.Close()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(volInfo); err != nil {
		return errors.Wrap(err, "rbd: decode error")
	}

	return nil
}

func (v *hostVolMeta) deleteVolInfo(image string, persistentStoragePath string) error {
	file := path.Join(persistentStoragePath, image+".json")
	glog.Infof("rbd: Deleting file for Volume: %s at: %s resulting path: %+v\n", image, persistentStoragePath, file)
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return errors.Wrapf(err, "rbd: error removing file %s", file)
		}
	}
	return nil
}

func (s *hostSnapMeta) persistSnapInfo(snapshot string, persistentStoragePath string, snapInfo *rbdSnapshot) error {
	file := path.Join(persistentStoragePath, snapshot+".json")
	fp, err := os.Create(file)
	if err != nil {
		glog.Errorf("rbd: failed to create persistent storage file %s with error: %v\n", file, err)
		return errors.Wrapf(err, "rbd: create error for %s", file)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(snapInfo); err != nil {
		glog.Errorf("rbd: failed to encode snapInfo: %+v for file: %s with error: %v\n", snapInfo, file, err)
		return errors.Wrap(err, "rbd: encode error")
	}
	glog.Infof("rbd: successfully saved snapInfo: %+v into file: %s\n", snapInfo, file)
	return nil
}

func (s *hostSnapMeta) loadSnapInfo(snapshot string, persistentStoragePath string, snapInfo *rbdSnapshot) error {
	file := path.Join(persistentStoragePath, snapshot+".json")
	fp, err := os.Open(file)
	if err != nil {
		return errors.Wrapf(err, "rbd: open error for %s", file)
	}
	defer fp.Close()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(snapInfo); err != nil {
		return errors.Wrap(err, "rbd: decode error")
	}
	return nil
}

func (s *hostSnapMeta) deleteSnapInfo(snapshot string, persistentStoragePath string) error {
	file := path.Join(persistentStoragePath, snapshot+".json")
	glog.Infof("rbd: Deleting file for Snapshot: %s at: %s resulting path: %+v\n", snapshot, persistentStoragePath, file)
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return errors.Wrapf(err, "rbd: error removing file %s", file)
		}
	}
	return nil
}

type cmVolMeta struct {}
type cmSnapMeta struct {}

func NewK8sClient() (*k8s.Clientset) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		glog.Errorf("Failed to get cluster config with error: %v\n", err)
		os.Exit(1)
	}
	client, err := k8s.NewForConfig(cfg)
	if err != nil {
		glog.Errorf("Failed to create client with error: %v\n", err)
		os.Exit(1)
	}
	return client
}

func CreateMetadataCM() error {
	cm, err := getMetadataCM()
	if err != nil {
		glog.Infof("rbd: an error occured getting configmap %s with error: %v", cmName, err)
		glog.Infof("rbd: creating ConfigMap...")
		cm = &v1.ConfigMap {
			ObjectMeta: metav1.ObjectMeta {
				Name: cmName,
				Namespace: namespace,
			},
			Data: map[string]string{},
		}
		_, err := Client.CoreV1().ConfigMaps(namespace).Create(cm)
		if err != nil {
			glog.Errorf("rbd: couldn't create configmap %s with error: %v\n", cmName, err)
			return errors.Wrap(err, "rbd: create configmap error")
		}
	}
	if err == nil && cm != nil {
		glog.Infof("rbd: configmap %s already exists, skipping creation\n", cmName)
		return nil
	}
	glog.Infof("rbd: configmap %s successfully created\n", cmName)
	return nil
}

func getMetadataCM() (*v1.ConfigMap, error) {
	cm, err := NewK8sClient().CoreV1().ConfigMaps(namespace).Get(cmName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func loadExDataFromCM(cm *v1.ConfigMap) {
	rbdVol := rbdVolume{}
	rbdSnap := rbdSnapshot{}
	for id, data := range cm.Data {
		if strings.Contains(data, "snapName") {
			err := json.Unmarshal([]byte(data), &rbdSnap)
			if err != nil {
				return
			}
			rbdSnapshots[id] = &rbdSnap
		} else {
			err := json.Unmarshal([]byte(data), &rbdVol)
			if err != nil {
				return
			}
			rbdVolumes[id] = &rbdVol
		}
	}
	glog.Infof("rbd: loaded %d volumes and %d snapshots from ConfigMap %s", len(rbdVolumes), len(rbdSnapshots), cmName)
}

func (v *cmVolMeta) persistVolInfo(image, _ string, volInfo *rbdVolume) error {
	cm, err := getMetadataCM()
	if err != nil {
		return err
	}
	volInfoJson, err := json.Marshal(volInfo)
	if err != nil {
		return errors.Wrap(err, "rbd: marshal error")
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[image] = string(volInfoJson)

	updatedCM := &v1.ConfigMap {
		ObjectMeta: metav1.ObjectMeta {
			Name: cmName,
			Namespace: namespace,
		},
		Data: cm.Data,
	}
	_, err = Client.CoreV1().ConfigMaps(namespace).Update(updatedCM)
	if err != nil {
		glog.Errorf("rbd: couldn't persist volume %s metadata in configmap %s with error: %v", image, cmName, err)
		return errors.Wrap(err, "rbd: update configmap error")
	}
	glog.Infof("rbd: successfully persisted volume %s metadata in configmap %s", image, cmName)
	return nil
}

func (v *cmVolMeta) loadVolInfo(image, _ string, volInfo *rbdVolume) error {
	cm, err := getMetadataCM()
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(cm.Data[image]), volInfo)
	if err != nil {
		return errors.Wrap(err, "rbd: unmarshal error")
	}
	return nil
}

func (v *cmVolMeta) deleteVolInfo(image, _ string) error {
	cm, err := getMetadataCM()
	if err != nil {
		return err
	}
	delete(cm.Data, image)
	updatedCM := &v1.ConfigMap {
		ObjectMeta: metav1.ObjectMeta {
			Name: cmName,
			Namespace: namespace,
		},
		Data: cm.Data,
	}
	_, err = Client.CoreV1().ConfigMaps(namespace).Update(updatedCM)
	if err != nil {
		glog.Infof("rbd: couldn't delete volume %s metadata from configmap %s with error: %v", image, cmName, err)
		return errors.Wrap(err, "rbd: update configmap error")
	}
	glog.Infof("rbd: successfully deleted volume %s metadata from configmap %s", image, cmName)
	return nil
}

func (s *cmSnapMeta) persistSnapInfo(snapshot, _ string, snapInfo *rbdSnapshot) error {
	cm, err := getMetadataCM()
	if err != nil {
		return err
	}
	snapInfoJson, err := json.Marshal(snapInfo)
	if err != nil {
		return errors.Wrap(err, "rbd: marshal error")
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[snapshot] = string(snapInfoJson)

	updatedCM := &v1.ConfigMap {
		ObjectMeta: metav1.ObjectMeta {
			Name: cmName,
			Namespace: namespace,
		},
		Data: cm.Data,
	}
	_, err = Client.CoreV1().ConfigMaps(namespace).Update(updatedCM)
	if err != nil {
		glog.Errorf("rbd: couldn't persist snapshot %s metadata in configmap %s with error: %v", snapshot, cmName, err)
		return errors.Wrap(err, "rbd: update configmap error")
	}
	glog.Infof("rbd: successfully persisted snapshot %s metadata in configmap %s", snapshot, cmName)
	return nil
}

func (s *cmSnapMeta) loadSnapInfo(snapshot, _ string, snapInfo *rbdSnapshot) error {
	cm, err := getMetadataCM()
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(cm.Data[snapshot]), snapInfo)
	if err != nil {
		return errors.Wrap(err, "rbd: unmarshal error")
	}
	return nil
}

func (s *cmSnapMeta) deleteSnapInfo(snapshot, _ string) error {
	cm, err := getMetadataCM()
	if err != nil {
		return err
	}
	delete(cm.Data, snapshot)
	updatedCM := &v1.ConfigMap {
		ObjectMeta: metav1.ObjectMeta {
			Name: cmName,
			Namespace: namespace,
		},
		Data: cm.Data,
	}
	_, err = Client.CoreV1().ConfigMaps(namespace).Update(updatedCM)
	if err != nil {
		glog.Infof("rbd: couldn't delete snapshot %s metadata from configmap %s with error: %v", snapshot, cmName, err)
		return errors.Wrap(err, "rbd: update configmap error")
	}
	glog.Infof("rbd: successfully deleted snapshot %s metadata from configmap %s", snapshot, cmName)
	return nil
}
