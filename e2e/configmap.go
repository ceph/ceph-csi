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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func deleteConfigMap(pluginPath string) error {
	path := pluginPath + configMap

	return retryKubectlFile(cephCSINamespace, kubectlDelete, path, deployTimeout)
}

func createConfigMap(pluginPath string, c kubernetes.Interface, f *framework.Framework) error {
	path := pluginPath + configMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	if err != nil {
		return err
	}

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}

	// get mon list
	mons, err := getMons(rookNamespace, c)
	if err != nil {
		return err
	}
	conmap := []util.ClusterInfo{{
		ClusterID: fsID,
		Monitors:  mons,
		RBD: struct {
			NetNamespaceFilePath string `json:"netNamespaceFilePath"`
			RadosNamespace       string `json:"radosNamespace"`
		}{
			RadosNamespace: radosNamespace,
		},
	}}
	if upgradeTesting {
		subvolumegroup = "csi"
	}
	conmap[0].CephFS.SubvolumeGroup = subvolumegroup
	data, err := json.Marshal(conmap)
	if err != nil {
		return err
	}
	cm.Data["config.json"] = string(data)
	cm.Namespace = cephCSINamespace
	// if the configmap is present update it,during cephcsi helm charts
	// deployment empty configmap gets created we need to override it
	_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Get(context.TODO(), cm.Name, metav1.GetOptions{})

	if err == nil {
		_, updateErr := c.CoreV1().ConfigMaps(cephCSINamespace).Update(context.TODO(), &cm, metav1.UpdateOptions{})
		if updateErr != nil {
			return fmt.Errorf("failed to update configmap: %w", updateErr)
		}
	}
	if apierrs.IsNotFound(err) {
		_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Create(context.TODO(), &cm, metav1.CreateOptions{})
	}

	return err
}

// createCustomConfigMap provides multiple clusters information.
func createCustomConfigMap(
	c kubernetes.Interface,
	pluginPath string,
	clusterInfo map[string]map[string]string,
) error {
	path := pluginPath + configMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	if err != nil {
		return err
	}
	// get mon list
	mons, err := getMons(rookNamespace, c)
	if err != nil {
		return err
	}
	// get clusterIDs
	var clusterID []string
	for key := range clusterInfo {
		clusterID = append(clusterID, key)
	}
	conmap := make([]util.ClusterInfo, len(clusterID))

	for i, j := range clusterID {
		conmap[i].ClusterID = j
		conmap[i].Monitors = mons
	}

	// fill radosNamespace and subvolgroups
	for cluster, confItems := range clusterInfo {
		for i, j := range confItems {
			switch i {
			case "subvolumeGroup":
				for c := range conmap {
					if conmap[c].ClusterID == cluster {
						conmap[c].CephFS.SubvolumeGroup = j
					}
				}
			case "radosNamespace":
				for c := range conmap {
					if conmap[c].ClusterID == cluster {
						conmap[c].RBD.RadosNamespace = j
					}
				}
			}
		}
	}

	data, err := json.Marshal(conmap)
	if err != nil {
		return err
	}
	cm.Data["config.json"] = string(data)
	cm.Namespace = cephCSINamespace
	// since a configmap is already created, update the existing configmap
	_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Update(context.TODO(), &cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}

	return nil
}
