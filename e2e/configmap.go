package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/v3/internal/util"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func deleteConfigMap(pluginPath string) error {
	path := pluginPath + configMap
	_, err := framework.RunKubectl(cephCSINamespace, "delete", "-f", path, ns)
	if err != nil {
		return err
	}
	return nil
}

func createConfigMap(pluginPath string, c kubernetes.Interface, f *framework.Framework) error {
	path := pluginPath + configMap
	cm := v1.ConfigMap{}
	err := unmarshal(path, &cm)
	if err != nil {
		return err
	}

	fsID, stdErr, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error getting fsid %v", stdErr)
	}
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	// get mon list
	mons, err := getMons(rookNamespace, c)
	if err != nil {
		return err
	}
	conmap := []util.ClusterInfo{{
		ClusterID:      fsID,
		Monitors:       mons,
		RadosNamespace: radosNamespace,
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
			return updateErr
		}
	}
	if apierrs.IsNotFound(err) {
		_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Create(context.TODO(), &cm, metav1.CreateOptions{})
	}

	return err
}

// createCustomConfigMap provides multiple clusters information.
func createCustomConfigMap(c kubernetes.Interface, pluginPath string, subvolgrpInfo map[string]string) error {
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
	for key := range subvolgrpInfo {
		clusterID = append(clusterID, key)
	}
	conmap := []util.ClusterInfo{
		{
			ClusterID: clusterID[0],
			Monitors:  mons,
		},
		{
			ClusterID: clusterID[1],
			Monitors:  mons,
		}}
	for i := 0; i < len(subvolgrpInfo); i++ {
		conmap[i].CephFS.SubvolumeGroup = subvolgrpInfo[clusterID[i]]
	}
	data, err := json.Marshal(conmap)
	if err != nil {
		return err
	}
	cm.Data["config.json"] = string(data)
	cm.Namespace = cephCSINamespace
	// since a configmap is already created, update the existing configmap
	_, err = c.CoreV1().ConfigMaps(cephCSINamespace).Update(context.TODO(), &cm, metav1.UpdateOptions{})
	return err
}
