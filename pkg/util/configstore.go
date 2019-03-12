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
	"fmt"
	"k8s.io/klog"
	"path"
	"strings"
)

// StoreReader interface enables plugging different stores, that contain the
// keys and data. (e.g k8s secrets or local files)
type StoreReader interface {
	DataForKey(clusterID string, key string) (string, error)
}

/* ConfigKeys contents and format,
- csMonitors: MON list, comma separated
- csAdminID: adminID, used for provisioning
- csUserID: userID, used for publishing
- csAdminKey: key, for userID in csProvisionerUser
- csUserKey: key, for userID in csPublisherUser
- csPools: Pool list, comma separated
*/

// Constants for various ConfigKeys
const (
	csMonitors = "monitors"
	csAdminID  = "adminid"
	csUserID   = "userid"
	csAdminKey = "adminkey"
	csUserKey  = "userkey"
	csPools    = "pools"
)

// ConfigStore provides various gettors for ConfigKeys
type ConfigStore struct {
	StoreReader
}

// dataForKey returns data from the config store for the provided key
func (dc *ConfigStore) dataForKey(clusterID string, key string) (string, error) {
	if dc.StoreReader != nil {
		return dc.StoreReader.DataForKey(clusterID, key)
	}

	err := errors.New("config store location uninitialized")
	return "", err
}

// Mons returns a comma separated MON list from the cluster config represented by clusterID
func (dc *ConfigStore) Mons(clusterID string) (string, error) {
	return dc.dataForKey(clusterID, csMonitors)
}

// Pools returns a list of pool names from the cluster config represented by clusterID
func (dc *ConfigStore) Pools(clusterID string) ([]string, error) {
	content, err := dc.dataForKey(clusterID, csPools)
	if err != nil {
		return nil, err
	}

	return strings.Split(content, ","), nil
}

// AdminID returns the admin ID from the cluster config represented by clusterID
func (dc *ConfigStore) AdminID(clusterID string) (string, error) {
	return dc.dataForKey(clusterID, csAdminID)
}

// UserID returns the user ID from the cluster config represented by clusterID
func (dc *ConfigStore) UserID(clusterID string) (string, error) {
	return dc.dataForKey(clusterID, csUserID)
}

// KeyForUser returns the key for the requested user ID from the cluster config
// represented by clusterID
func (dc *ConfigStore) KeyForUser(clusterID, userID string) (data string, err error) {
	var fetchKey string
	user, err := dc.AdminID(clusterID)
	if err != nil {
		return
	}

	if user == userID {
		fetchKey = csAdminKey
	} else {
		user, err = dc.UserID(clusterID)
		if err != nil {
			return
		}

		if user != userID {
			err = fmt.Errorf("requested user (%s) not found in cluster configuration of (%s)", userID, clusterID)
			return
		}

		fetchKey = csUserKey
	}

	return dc.dataForKey(clusterID, fetchKey)
}

// NewConfigStore returns a config store based on value of configRoot. If
// configRoot is not "k8s_objects" then it is assumed to be a path to a
// directory, under which the configuration files can be found
func NewConfigStore(configRoot string) (*ConfigStore, error) {
	if configRoot != "k8s_objects" {
		klog.Infof("cache-store: using files in path (%s) as config store", configRoot)
		fc := &FileConfig{}
		fc.BasePath = path.Clean(configRoot)
		dc := &ConfigStore{fc}
		return dc, nil
	}

	klog.Infof("cache-store: using k8s objects as config store")
	kc := &K8sConfig{}
	kc.Client = NewK8sClient()
	kc.Namespace = GetK8sNamespace()
	dc := &ConfigStore{kc}
	return dc, nil
}
